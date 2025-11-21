package playback

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"

	"plexmusic-tui/internal/domain"

	log "github.com/charmbracelet/log/v2"
)

// Player manages audio playback
type Player struct {
	state         domain.PlaybackState
	currentTrack  *domain.Track
	streamer      beep.StreamSeekCloser
	ctrl          *beep.Ctrl
	volume        *effects.Volume
	desiredVolume float64 // Volume to apply when volume effect is created
	speakerInit   bool
	sampleRate    beep.SampleRate
	position      int // Current position in samples
	length        int // Total length in samples
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
	if p.streamer != nil {
		p.streamer.Close()
	}

	var streamer beep.StreamSeekCloser
	var format beep.Format
	var err error

	// Helper to copy the stream to a buffer and retry decoding
	tryDecodeFromBuffer := func(buf []byte) (beep.StreamSeekCloser, beep.Format, error) {
		rdr := bytes.NewReader(buf)
		log.Debug("tryDecodeFromBuffer", "content_type", contentType, "buf_len", len(buf))
		// Try MP3 fallback first
		if strings.Contains(contentType, "mp3") || strings.Contains(contentType, "mpeg") || true {
			if s, f, e := mp3.Decode(io.NopCloser(rdr)); e == nil {
				return s, f, nil
			} else {
				log.Debug("mp3 decode failed on buffer", "err", e)
			}
			// Reset reader for next try
			rdr.Seek(0, io.SeekStart)
		}
		if strings.Contains(contentType, "flac") || strings.Contains(contentType, "x-flac") {
			if s, f, e := flac.Decode(io.NopCloser(rdr)); e == nil {
				return s, f, nil
			} else {
				log.Debug("flac decode failed on buffer", "err", e)
			}
			rdr.Seek(0, io.SeekStart)
		}
		if strings.Contains(contentType, "ogg") {
			if s, f, e := vorbis.Decode(io.NopCloser(rdr)); e == nil {
				return s, f, nil
			} else {
				log.Debug("vorbis decode failed on buffer", "err", e)
			}
			rdr.Seek(0, io.SeekStart)
		}
		if strings.Contains(contentType, "wav") {
			if s, f, e := wav.Decode(io.NopCloser(rdr)); e == nil {
				return s, f, nil
			} else {
				log.Debug("wav decode failed on buffer", "err", e)
			}
			rdr.Seek(0, io.SeekStart)
		}
		// Last attempt, try mp3 decode regardless
		rdr.Seek(0, io.SeekStart)
		return mp3.Decode(io.NopCloser(rdr))
	}

	// Try to decode based on content type
	if strings.Contains(contentType, "mp3") || strings.Contains(contentType, "mpeg") {
		streamer, format, err = mp3.Decode(body)
		if err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
		// If EOF or another decode error happened, capture the body and retry
		var buf []byte
		if buf, err = io.ReadAll(body); err != nil {
			_ = body.Close()
			return fmt.Errorf("failed to read stream after decode error: %w", err)
		}
		log.Debug("mp3 initial decode failed; retrying with fallback", "content_type", contentType, "buf_len", len(buf), "err", err)
		// Retry decoding from buffer
		if streamer, format, err = tryDecodeFromBuffer(buf); err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
		// decode failed again — fall through to other decoders
	}

	if strings.Contains(contentType, "flac") || strings.Contains(contentType, "x-flac") {
		streamer, format, err = flac.Decode(body)
		if err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
		// Try fallback buffer approach
		var buf2 []byte
		if buf2, err = io.ReadAll(body); err == nil {
			log.Debug("flac initial decode failed; retrying with fallback", "content_type", contentType, "buf_len", len(buf2), "err", err)
			if streamer2, format2, err2 := tryDecodeFromBuffer(buf2); err2 == nil {
				p.attachStreamer(streamer2, format2)
				return nil
			}
		}
	}

	if strings.Contains(contentType, "ogg") {
		streamer, format, err = vorbis.Decode(body)
		if err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
		var buf3 []byte
		if buf3, err = io.ReadAll(body); err == nil {
			log.Debug("vorbis initial decode failed; retrying with fallback", "content_type", contentType, "buf_len", len(buf3), "err", err)
			if streamer3, format3, err3 := tryDecodeFromBuffer(buf3); err3 == nil {
				p.attachStreamer(streamer3, format3)
				return nil
			}
		}
	}

	if strings.Contains(contentType, "wav") {
		streamer, format, err = wav.Decode(body)
		if err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
		var buf4 []byte
		if buf4, err = io.ReadAll(body); err == nil {
			log.Debug("wav initial decode failed; retrying with fallback", "content_type", contentType, "buf_len", len(buf4), "err", err)
			if streamer4, format4, err4 := tryDecodeFromBuffer(buf4); err4 == nil {
				p.attachStreamer(streamer4, format4)
				return nil
			}
		}
	}

	// If content type didn't help, try to read the body into a buffer and try multiple decoders
	buf, rerr := io.ReadAll(body)
	if rerr != nil {
		body.Close()
		return fmt.Errorf("failed to buffer stream for decoding fallback: %w", rerr)
	}
	log.Debug("decode fallback; trying buffer-based decoding", "content_type", contentType, "buf_len", len(buf))
	streamer, format, err = tryDecodeFromBuffer(buf)
	if err != nil {
		return fmt.Errorf("failed to decode audio: %w", err)
	}

	p.attachStreamer(streamer, format)
	return nil
}

// attachStreamer attaches a streamer to the player and sets up playback chain
func (p *Player) attachStreamer(streamer beep.StreamSeekCloser, format beep.Format) {
	p.streamer = streamer
	p.sampleRate = format.SampleRate
	p.length = streamer.Len()

	// Create the control node
	p.ctrl = &beep.Ctrl{Streamer: streamer}

	// Create the volume effect
	p.volume = &effects.Volume{Streamer: p.ctrl, Base: 2}

	// Apply the desired volume to the newly created effect
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

	p.currentTrack = track
	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()
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
	p.ctrl.Paused = true
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
	p.ctrl.Paused = false
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

	speaker.Lock()
	err := p.streamer.Seek(pos)
	speaker.Unlock()

	if err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	p.position = pos
	return nil
}

// UpdatePosition updates the current playback position (to be called periodically)
func (p *Player) UpdatePosition() {
	if p.state == domain.PlaybackPlaying && p.speakerInit {
		speaker.Lock()
		pos := p.streamer.Position()
		speaker.Unlock()
		p.position = pos
	}
}

// Close closes the player and releases resources
func (p *Player) Close() error {
	if p.state != domain.PlaybackStopped {
		if err := p.Stop(); err != nil {
			return err
		}
	}

	if p.streamer != nil {
		if err := p.streamer.Close(); err != nil {
			return err
		}
	}

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
