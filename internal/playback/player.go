package playback

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	log "github.com/charmbracelet/log/v2"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"

	"plexmusic-tui/internal/domain"
)

// Player manages audio playback
type Player struct {
	state        domain.PlaybackState
	currentTrack *domain.Track
	// streamer is used by speaker.Play; seekStreamer is non-nil for seekable
	// streams (files) to support seeking, length, and position.
	streamer              beep.Streamer
	seekStreamer          beep.StreamSeekCloser
	ctrl                  *beep.Ctrl
	volume                *effects.Volume
	desiredVolume         float64 // Volume to apply when volume effect is created
	speakerInit           bool
	sampleRate            beep.SampleRate
	initializedSampleRate beep.SampleRate
	position              int // Current position in samples (0 if unknown for live streams)
	length                int // Total length in samples (0 for live streams)
	onCompletion          func()
}

// NewPlayer creates a new audio player
func NewPlayer() *Player {
	return &Player{
		state:         domain.PlaybackStopped,
		desiredVolume: 0.0, // Default to 0 (100% display, logarithmic scale with Base:2)
	}
}

// State returns the current playback state
func (p *Player) State() domain.PlaybackState {
	return p.state
}

// CurrentTrack returns the currently playing track
func (p *Player) CurrentTrack() *domain.Track {
	return p.currentTrack
}

// Position returns the current playback position in samples
func (p *Player) Position() int {
	return p.position
}

// AudiblePosition returns the estimated audible position (stream position minus buffer size)
func (p *Player) AudiblePosition() int {
	if p.sampleRate == 0 {
		return 0
	}
	// Buffer size is 100ms (hardcoded in Initialize)
	bufferSamples := int(p.sampleRate) / 10
	pos := p.position - bufferSamples
	if pos < 0 {
		return 0
	}
	return pos
}

// Length returns the total track length in samples
func (p *Player) Length() int {
	return p.length
}

// IsInitialized returns whether the speaker is initialized
func (p *Player) IsInitialized() bool {
	return p.speakerInit
}

// LoadStream loads an audio stream and prepares it for playback
// contentType is used to determine the audio format
func (p *Player) LoadStream(body io.ReadCloser, contentType string) error {
	// Close previous streamer if it exists to free resources
	if p.seekStreamer != nil {
		_ = p.seekStreamer.Close()
		p.seekStreamer = nil
	}
	p.streamer = nil

	// If body implements ReadSeeker we can attempt buffered decode and
	// multiple decoders; otherwise this is a streaming body and we will
	// attempt a single decode based on Content-Type and not buffer the
	// entire stream to avoid unbounded memory use.
	var format beep.Format
	var seekableStream beep.StreamSeekCloser
	var stream beep.Streamer
	var rs io.ReadSeeker
	var errDecode error
	tryDirect := func(decode func(io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error)) bool {
		s, f, e := decode(body)
		if e == nil {
			seekableStream = s
			stream = s
			format = f
			return true
		}
		errDecode = e
		return false
	}

	// For non-seekable bodies we only attempt a single decoder based on
	// content type and will not fallback to buffering (can't re-read the body).
	decoded := false
	if strings.Contains(contentType, "mp3") || strings.Contains(contentType, "mpeg") {
		if tryDirect(mp3.Decode) {
			decoded = true
		}
	} else if strings.Contains(contentType, "flac") || strings.Contains(contentType, "x-flac") {
		if tryDirect(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return flac.Decode(rc) }) {
			decoded = true
		}
	} else if strings.Contains(contentType, "ogg") {
		if tryDirect(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return vorbis.Decode(rc) }) {
			decoded = true
		}
	} else if strings.Contains(contentType, "wav") {
		if tryDirect(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return wav.Decode(rc) }) {
			decoded = true
		}
	}

	if decoded {
		// Attach whichever stream we obtained (seekable or not)
		if seekableStream != nil {
			p.attachStreamer(seekableStream, format)
			return nil
		}
		// If stream implements StreamSeekCloser, cast; else attach via helper
		if ssc, ok := stream.(beep.StreamSeekCloser); ok {
			p.attachStreamer(ssc, format)
			return nil
		}
		// No seekable interface; attach streaming-only streamer
		p.attachStreamerStream(stream, format)
		return nil
	}

	// If we are here, and if the original body is seekable, we can fall
	// back to buffering and trying more decoders. Otherwise, we failed.
	if rsVal, ok := body.(io.ReadSeeker); ok {
		rs = rsVal
		// Rewind seeker so we can read whole content
		_, _ = rs.Seek(0, io.SeekStart)
		// Buffer and attempt decodes using the memory-backed reader, similar to legacy behavior
	} else {
		// Non-seekable body; we can't buffer and retry because bytes have
		// been consumed by the failed decode attempt. Return the decode err.
		if errDecode != nil {
			return fmt.Errorf("stream decode failed: %w", errDecode)
		}
		return fmt.Errorf("stream decode failed: unable to decode stream")
	}
	// If still here, we have a seekable body; read it into memory and try decoders
	buf, err := io.ReadAll(rs)
	_ = body.Close() // Always close the network body
	if err != nil {
		return fmt.Errorf("failed to buffer stream: %w", err)
	}
	if len(buf) == 0 {
		return fmt.Errorf("stream is empty")
	}
	var streamerSeek beep.StreamSeekCloser
	// Helper to try a decoder on the buffer
	tryBuf := func(decode func(io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error)) bool {
		r := nopCloserSeeker{bytes.NewReader(buf)}
		s, f, e := decode(r)
		if e == nil {
			streamerSeek = s
			format = f
			return true
		}
		return false
	}
	if strings.Contains(contentType, "mp3") || strings.Contains(contentType, "mpeg") {
		if tryBuf(mp3.Decode) {
			p.attachStreamer(streamerSeek, format)
			return nil
		}
	} else if strings.Contains(contentType, "flac") || strings.Contains(contentType, "x-flac") {
		if tryBuf(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return flac.Decode(rc) }) {
			p.attachStreamer(streamerSeek, format)
			return nil
		}
	} else if strings.Contains(contentType, "ogg") {
		if tryBuf(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return vorbis.Decode(rc) }) {
			p.attachStreamer(streamerSeek, format)
			return nil
		}
	} else if strings.Contains(contentType, "wav") {
		if tryBuf(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return wav.Decode(rc) }) {
			p.attachStreamer(streamerSeek, format)
			return nil
		}
	}
	// Final fallback: try all
	if tryBuf(mp3.Decode) ||
		tryBuf(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return flac.Decode(rc) }) ||
		tryBuf(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return vorbis.Decode(rc) }) ||
		tryBuf(func(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) { return wav.Decode(rc) }) {
		p.attachStreamer(streamerSeek, format)
		return nil
	}

	return fmt.Errorf("failed to decode audio: format not recognized or decode failed")
}

// attachStreamer attaches a streamer to the player and sets up playback chain
func (p *Player) attachStreamer(streamer beep.StreamSeekCloser, format beep.Format) {
	p.streamer = streamer
	p.seekStreamer = streamer
	p.sampleRate = format.SampleRate
	p.length = streamer.Len()

	// Wrap the streamer to detect completion
	// We use beep.Seq to append a callback that runs when the streamer is exhausted
	wrappedStreamer := beep.Seq(streamer, beep.Callback(func() {
		// Use a local copy of the callback to avoid race conditions if it's changed
		cb := p.onCompletion
		if cb != nil {
			// Run in a separate goroutine to avoid blocking the speaker
			go func() {
				// Recover from panics in the callback to prevent crashing the audio thread
				defer func() {
					if r := recover(); r != nil {
						// We can't easily log here without importing log package, but we prevent the crash
						// fmt.Printf("Panic in playback completion callback: %v\n", r)
						_ = r
					}
				}()
				cb()
			}()
		}
	}))

	// Create the control node
	p.ctrl = &beep.Ctrl{Streamer: wrappedStreamer}

	// Create the volume effect
	p.volume = &effects.Volume{Streamer: p.ctrl, Base: 2}

	// Apply the desired volume to the newly created effect
	p.volume.Volume = p.desiredVolume
}

// attachStreamerStream attaches a non-seekable streamer (e.g., live stream)
// and sets up the playback chain. Length will be zero for live streams.
func (p *Player) attachStreamerStream(streamer beep.Streamer, format beep.Format) {
	p.streamer = streamer
	p.seekStreamer = nil
	p.sampleRate = format.SampleRate
	p.length = 0

	// Wrap the streamer to detect completion similarly
	wrappedStreamer := beep.Seq(streamer, beep.Callback(func() {
		cb := p.onCompletion
		if cb != nil {
			go func() {
				defer func() { _ = recover() }()
				cb()
			}()
		}
	}))
	p.ctrl = &beep.Ctrl{Streamer: wrappedStreamer}
	p.volume = &effects.Volume{Streamer: p.ctrl, Base: 2}
	p.volume.Volume = p.desiredVolume
}

// Initialize initializes the speaker for playback
// Must be called before Play() is used
func (p *Player) Initialize() error {
	if p.speakerInit {
		return nil // Already initialized
	}

	if p.sampleRate == 0 {
		return fmt.Errorf("no audio stream loaded")
	}

	err := speaker.Init(p.sampleRate, p.sampleRate.N(time.Second/10))
	if err != nil {
		return fmt.Errorf("failed to initialize speaker: %w", err)
	}

	p.speakerInit = true
	p.initializedSampleRate = p.sampleRate
	return nil
}

// Play starts playback of the current track
func (p *Player) Play(track *domain.Track) error {
	if !p.speakerInit {
		return fmt.Errorf("speaker not initialized")
	}

	if p.volume == nil {
		return fmt.Errorf("no audio stream loaded")
	}

	// Check if we need to re-initialize speaker for new sample rate
	if p.sampleRate != p.initializedSampleRate {
		log.Info("Re-initializing speaker for new sample rate", "old", p.initializedSampleRate, "new", p.sampleRate)
		err := speaker.Init(p.sampleRate, p.sampleRate.N(time.Second/10))
		if err != nil {
			return fmt.Errorf("failed to re-initialize speaker: %w", err)
		}
		p.initializedSampleRate = p.sampleRate
	}

	p.currentTrack = track
	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()
	speaker.Clear()
	speaker.Play(p.volume)
	p.state = domain.PlaybackPlaying

	return nil
}

// Pause pauses the current playback
func (p *Player) Pause() error {
	if p.state == domain.PlaybackStopped {
		return fmt.Errorf("cannot pause when stopped")
	}

	speaker.Lock()
	if p.ctrl == nil {
		speaker.Unlock()
		p.state = domain.PlaybackPaused
		log.Debug("Player.Pause: ctrl is nil, pausing state only")
		return nil
	}
	p.ctrl.Paused = true
	log.Debug("Player.Pause: ctrl.Paused set to true")
	speaker.Unlock()
	p.state = domain.PlaybackPaused

	return nil
}

// Resume resumes paused playback
func (p *Player) Resume() error {
	if p.state != domain.PlaybackPaused {
		return fmt.Errorf("cannot resume when not paused")
	}

	speaker.Lock()
	if p.ctrl == nil {
		speaker.Unlock()
		p.state = domain.PlaybackPlaying
		log.Debug("Player.Resume: ctrl is nil, setting state to Playing")
		return nil
	}
	p.ctrl.Paused = false
	log.Debug("Player.Resume: ctrl.Paused set to false")
	speaker.Unlock()
	p.state = domain.PlaybackPlaying

	return nil
}

// Stop stops the current playback
func (p *Player) Stop() error {
	if p.state == domain.PlaybackStopped {
		return nil // Already stopped
	}

	speaker.Clear()
	p.state = domain.PlaybackStopped
	p.position = 0

	return nil
}

// Seek seeks to a specific position in samples
func (p *Player) Seek(pos int) error {
	if p.streamer == nil {
		return fmt.Errorf("no audio stream loaded")
	}

	// Guard against negative position
	if pos < 0 {
		pos = 0
	}
	// Guard against seeking past end
	if p.length > 0 && pos > p.length {
		pos = p.length
	}

	speaker.Lock()
	defer speaker.Unlock()

	// Recover from potential panics in underlying library
	defer func() {
		if r := recover(); r != nil {
			// Log or handle panic if possible, though we can't easily return error from defer
			// But at least we unlock the speaker
			// Intentionally ignored to prevent crash
			_ = r
		}
	}()

	if p.seekStreamer == nil {
		return fmt.Errorf("stream is not seekable")
	}
	err := p.seekStreamer.Seek(pos)
	if err != nil {
		// If we hit EOF while seeking (e.g. to the very end), treat it as success
		if err == io.EOF {
			p.position = pos
			return nil
		}
		return fmt.Errorf("seek failed: %w", err)
	}

	p.position = pos
	return nil
}

// UpdatePosition updates the current playback position (to be called periodically)
func (p *Player) UpdatePosition() {
	if p.state == domain.PlaybackPlaying && p.speakerInit {
		speaker.Lock()
		defer speaker.Unlock()

		if p.streamer != nil {
			if p.seekStreamer != nil {
				p.position = p.seekStreamer.Position()
			}
		}
	}
}

// Close closes the player and releases resources
func (p *Player) Close() error {
	if p.state != domain.PlaybackStopped {
		if err := p.Stop(); err != nil {
			return err
		}
	}

	if p.seekStreamer != nil {
		if err := p.seekStreamer.Close(); err != nil {
			return err
		}
		p.seekStreamer = nil
	}
	p.streamer = nil

	return nil
}

// IsPlaying returns true if audio is currently playing
func (p *Player) IsPlaying() bool {
	return p.state == domain.PlaybackPlaying
}

// IsPaused returns true if audio is paused
func (p *Player) IsPaused() bool {
	return p.state == domain.PlaybackPaused
}

// GetVolume returns the current volume level
func (p *Player) GetVolume() float64 {
	if p.volume == nil {
		return p.desiredVolume
	}
	return p.volume.Volume
}

// SetVolume sets the volume level (0.0 to 2.0)
func (p *Player) SetVolume(v float64) {
	// Store the desired volume so it can be applied when a stream is loaded
	p.desiredVolume = v
	// If a volume effect already exists, apply immediately
	if p.volume != nil {
		p.volume.Volume = v
	}
}

// SampleRate returns the sample rate of the current stream
func (p *Player) SampleRate() int {
	return int(p.sampleRate)
}

// SetCompletionCallback sets the callback to be executed when playback finishes
func (p *Player) SetCompletionCallback(cb func()) {
	p.onCompletion = cb
}

// nopCloserSeeker wraps an io.ReadSeeker to implement io.ReadCloser
// while preserving the Seek method for the underlying reader.
type nopCloserSeeker struct {
	io.ReadSeeker
}

func (n nopCloserSeeker) Close() error {
	return nil
}
