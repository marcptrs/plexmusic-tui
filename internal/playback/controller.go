package playback

import (
	"fmt"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/speaker"
)

// State represents the playback state
type State int

const (
	StateStopped State = iota
	StatePlaying
	StatePaused
)

// Track represents a playable track
type Track struct {
	Key string
	// Add other track fields as needed
}

// Controller manages audio playback
type Controller struct {
	state          State
	currentTrack   *Track
	streamer       beep.StreamSeekCloser
	ctrl           *beep.Ctrl
	volume         *effects.Volume
	speakerInit    bool
	sampleRate     beep.SampleRate
	streamPosition int64
	streamLength   int64
}

// NewController creates a new playback controller
func NewController() *Controller {
	return &Controller{
		state: StateStopped,
	}
}

// State returns the current playback state
func (c *Controller) State() State {
	return c.state
}

// CurrentTrack returns the currently playing track
func (c *Controller) CurrentTrack() *Track {
	return c.currentTrack
}

// SetCurrentTrack sets the current track
func (c *Controller) SetCurrentTrack(track *Track) {
	c.currentTrack = track
}

// Streamer returns the current streamer
func (c *Controller) Streamer() beep.StreamSeekCloser {
	return c.streamer
}

// SetStreamer sets the audio streamer
func (c *Controller) SetStreamer(s beep.StreamSeekCloser) {
	c.streamer = s
}

// Ctrl returns the control wrapper for pause/resume
func (c *Controller) Ctrl() *beep.Ctrl {
	return c.ctrl
}

// SetCtrl sets the control wrapper
func (c *Controller) SetCtrl(ctrl *beep.Ctrl) {
	c.ctrl = ctrl
}

// Volume returns the volume effect
func (c *Controller) Volume() *effects.Volume {
	return c.volume
}

// SetVolume sets the volume effect
func (c *Controller) SetVolume(vol *effects.Volume) {
	c.volume = vol
}

// SpeakerInit returns whether the speaker is initialized
func (c *Controller) SpeakerInit() bool {
	return c.speakerInit
}

// SetSpeakerInit sets the speaker initialization status
func (c *Controller) SetSpeakerInit(init bool) {
	c.speakerInit = init
}

// SampleRate returns the sample rate
func (c *Controller) SampleRate() beep.SampleRate {
	return c.sampleRate
}

// SetSampleRate sets the sample rate
func (c *Controller) SetSampleRate(rate beep.SampleRate) {
	c.sampleRate = rate
}

// StreamPosition returns the current stream position
func (c *Controller) StreamPosition() int64 {
	return c.streamPosition
}

// SetStreamPosition sets the stream position
func (c *Controller) SetStreamPosition(pos int64) {
	c.streamPosition = pos
}

// StreamLength returns the total stream length
func (c *Controller) StreamLength() int64 {
	return c.streamLength
}

// SetStreamLength sets the stream length
func (c *Controller) SetStreamLength(len int64) {
	c.streamLength = len
}

// Play starts playback
func (c *Controller) Play() {
	if c.ctrl != nil {
		speaker.Lock()
		c.ctrl.Paused = false
		speaker.Unlock()
	}
	c.state = StatePlaying
}

// Pause pauses playback
func (c *Controller) Pause() {
	if c.ctrl != nil {
		speaker.Lock()
		c.ctrl.Paused = true
		speaker.Unlock()
	}
	c.state = StatePaused
}

// Stop stops playback completely
func (c *Controller) Stop() {
	if c.streamer != nil {
		speaker.Clear()
		c.streamer.Close()
		c.streamer = nil
	}
	c.state = StateStopped
	c.currentTrack = nil
	c.ctrl = nil
	c.volume = nil
}

// IsPlaying returns true if playback is currently playing
func (c *Controller) IsPlaying() bool {
	return c.state == StatePlaying
}

// IsPaused returns true if playback is currently paused
func (c *Controller) IsPaused() bool {
	return c.state == StatePaused
}

// IsStopped returns true if playback is currently stopped
func (c *Controller) IsStopped() bool {
	return c.state == StateStopped
}

// InitializeSpeaker initializes the speaker for playback
func (c *Controller) InitializeSpeaker() error {
	if c.speakerInit {
		return nil // Already initialized
	}

	if c.sampleRate == 0 {
		return fmt.Errorf("sample rate not set")
	}

	err := speaker.Init(c.sampleRate, c.sampleRate.N(time.Second/10))
	if err != nil {
		return fmt.Errorf("failed to initialize speaker: %w", err)
	}

	c.speakerInit = true
	return nil
}

// ClearPlayback clears any existing playback
func (c *Controller) ClearPlayback() {
	if c.streamer != nil {
		speaker.Clear()
		c.streamer.Close()
		c.streamer = nil
	}
}

// GetPosition returns the current playback position
// Must be called with speaker.Lock held
func (c *Controller) GetPosition() int64 {
	if c.streamer == nil {
		return 0
	}
	return int64(c.streamer.Position())
}

// GetPositionTime returns the current playback position as duration
func (c *Controller) GetPositionTime() time.Duration {
	if c.sampleRate == 0 {
		return 0
	}
	return time.Duration(float64(c.GetPosition()) / float64(c.sampleRate) * float64(time.Second))
}

// GetLengthTime returns the total stream length as duration
func (c *Controller) GetLengthTime() time.Duration {
	if c.sampleRate == 0 {
		return 0
	}
	return time.Duration(float64(c.streamLength) / float64(c.sampleRate) * float64(time.Second))
}
