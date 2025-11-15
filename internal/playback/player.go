package playback

import (
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
)

// Player manages audio playback
type Player struct {
	state        domain.PlaybackState
	currentTrack *domain.Track
	streamer     beep.StreamSeekCloser
	ctrl         *beep.Ctrl
	volume       *effects.Volume
	speakerInit  bool
	sampleRate   beep.SampleRate
	position     int // Current position in samples
	length       int // Total length in samples
}

// NewPlayer creates a new audio player
func NewPlayer() *Player {
	return &Player{
		state: domain.PlaybackStopped,
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
func (p *Player) LoadStream(body io.ReadCloser, contentType string, _ beep.Format) error {
	var streamer beep.StreamSeekCloser
	var format beep.Format
	var err error

	// Try to decode based on content type
	if strings.Contains(contentType, "mp3") || strings.Contains(contentType, "mpeg") {
		streamer, format, err = mp3.Decode(body)
		if err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
	}

	if strings.Contains(contentType, "flac") || strings.Contains(contentType, "x-flac") {
		streamer, format, err = flac.Decode(body)
		if err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
	}

	if strings.Contains(contentType, "ogg") {
		streamer, format, err = vorbis.Decode(body)
		if err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
	}

	if strings.Contains(contentType, "wav") {
		streamer, format, err = wav.Decode(body)
		if err == nil {
			p.attachStreamer(streamer, format)
			return nil
		}
	}

	// If content type didn't help, try MP3 as default (most common format)
	streamer, format, err = mp3.Decode(body)
	if err != nil {
		body.Close()
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
		return 1.0
	}
	return p.volume.Volume
}

// SetVolume sets the volume level (0.0 to 2.0)
func (p *Player) SetVolume(v float64) {
	if p.volume != nil {
		p.volume.Volume = v
	}
}
