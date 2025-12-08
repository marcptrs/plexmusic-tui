package app

import (
	"image"
	"sync"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
)

// PlaybackContext manages all playback-related state including audio streaming,
// volume control, and album artwork. This separates playback concerns from
// general application state.
type PlaybackContext struct {
	// Playback state
	state        PlaybackState
	currentTrack *Track

	// Audio streaming state
	streamer       beep.StreamSeekCloser
	ctrl           *beep.Ctrl
	volume         *effects.Volume
	speakerInit    bool
	sampleRate     beep.SampleRate
	streamPosition int
	streamLength   int

	// Album artwork for current playback
	albumArt      image.Image
	albumArtThumb string

	mu sync.RWMutex
}

// NewPlaybackContext creates a new playback context
func NewPlaybackContext() *PlaybackContext {
	return &PlaybackContext{
		state: PlaybackStopped,
	}
}

// State

func (p *PlaybackContext) State() PlaybackState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

func (p *PlaybackContext) SetState(state PlaybackState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = state
}

func (p *PlaybackContext) IsPlaying() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state == PlaybackPlaying
}

func (p *PlaybackContext) IsPaused() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state == PlaybackPaused
}

func (p *PlaybackContext) IsStopped() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state == PlaybackStopped
}

// Current track

func (p *PlaybackContext) CurrentTrack() *Track {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentTrack
}

func (p *PlaybackContext) SetCurrentTrack(track *Track) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentTrack = track
}

func (p *PlaybackContext) HasCurrentTrack() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentTrack != nil
}

// Streaming

func (p *PlaybackContext) Streamer() beep.StreamSeekCloser {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.streamer
}

func (p *PlaybackContext) SetStreamer(s beep.StreamSeekCloser) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.streamer = s
}

func (p *PlaybackContext) Ctrl() *beep.Ctrl {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ctrl
}

func (p *PlaybackContext) SetCtrl(c *beep.Ctrl) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ctrl = c
}

func (p *PlaybackContext) Volume() *effects.Volume {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.volume
}

func (p *PlaybackContext) SetVolume(v *effects.Volume) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = v
}

func (p *PlaybackContext) SpeakerInit() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.speakerInit
}

func (p *PlaybackContext) SetSpeakerInit(init bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.speakerInit = init
}

func (p *PlaybackContext) SampleRate() beep.SampleRate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sampleRate
}

func (p *PlaybackContext) SetSampleRate(sr beep.SampleRate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sampleRate = sr
}

// Position tracking

func (p *PlaybackContext) StreamPosition() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.streamPosition
}

func (p *PlaybackContext) SetStreamPosition(pos int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.streamPosition = pos
}

func (p *PlaybackContext) StreamLength() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.streamLength
}

func (p *PlaybackContext) SetStreamLength(len int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.streamLength = len
}

func (p *PlaybackContext) CalculatedPositionMs() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.sampleRate == 0 {
		return 0
	}
	return (p.streamPosition * 1000) / int(p.sampleRate)
}

// Album artwork

func (p *PlaybackContext) AlbumArt() image.Image {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.albumArt
}

func (p *PlaybackContext) AlbumArtThumb() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.albumArtThumb
}

func (p *PlaybackContext) SetAlbumArt(img image.Image, thumbURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.albumArt = img
	p.albumArtThumb = thumbURL
}
