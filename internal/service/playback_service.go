package service

import (
	"context"
	"io"

	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/playback"
	"plexmusic-tui/internal/pubsub"

	log "github.com/charmbracelet/log/v2"
)

// PlaybackEvent represents playback-related events
type PlaybackEvent struct {
	Type       string
	Track      *domain.Track
	State      domain.PlaybackState
	Position   int
	Length     int
	SampleRate int
	Volume     float64
	Error      error
}

// PlaybackService provides playback operations with event publishing
type PlaybackService struct {
	player *playback.Player
	broker *pubsub.Broker[PlaybackEvent]
}

// NewPlaybackService creates a new playback service
func NewPlaybackService() *PlaybackService {
	return &PlaybackService{
		player: playback.NewPlayer(),
		broker: pubsub.NewBroker[PlaybackEvent](),
	}
}

// Subscribe returns a channel for receiving playback events
func (s *PlaybackService) Subscribe(ctx context.Context) <-chan pubsub.Event[PlaybackEvent] {
	return s.broker.Subscribe(ctx)
}

// LoadStream loads an audio stream for playback
func (s *PlaybackService) LoadStream(body io.ReadCloser, contentType string) error {
	err := s.player.LoadStream(body, contentType)
	if err != nil {
		s.broker.Publish(pubsub.Event[PlaybackEvent]{
			Type: "playback.load_failed",
			Payload: PlaybackEvent{
				Type:  "playback.load_failed",
				Error: err,
			},
		})
		// Log debug info to the file so we can triage runtime decode issues
		log.Debug("playback.load_failed", "error", err)
		return err
	}

	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.loaded",
		Payload: PlaybackEvent{
			Type: "playback.loaded",
		},
	})
	log.Debug("playback.loaded")

	return nil
}

// Initialize initializes the audio subsystem
func (s *PlaybackService) Initialize() error {
	err := s.player.Initialize()
	if err != nil {
		s.broker.Publish(pubsub.Event[PlaybackEvent]{
			Type: "playback.init_failed",
			Payload: PlaybackEvent{
				Type:  "playback.init_failed",
				Error: err,
			},
		})
		log.Debug("playback.init_failed", "error", err)
		return err
	}

	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.initialized",
		Payload: PlaybackEvent{
			Type: "playback.initialized",
		},
	})
	log.Debug("playback.initialized")

	return nil
}

// Play starts playback of a track
func (s *PlaybackService) Play(track *domain.Track) error {
	err := s.player.Play(track)
	if err != nil {
		s.broker.Publish(pubsub.Event[PlaybackEvent]{
			Type: "playback.play_failed",
			Payload: PlaybackEvent{
				Type:  "playback.play_failed",
				Track: track,
				Error: err,
			},
		})
		// Add a debug log to help trace practical failures in real runs
		log.Debug("playback.play_failed", "error", err, "track", track)
		return err
	}

	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.started",
		Payload: PlaybackEvent{
			Type:  "playback.started",
			Track: track,
			State: domain.PlaybackPlaying,
		},
	})
	log.Debug("playback.started", "track", track)

	return nil
}

// Pause pauses playback
func (s *PlaybackService) Pause() error {
	err := s.player.Pause()
	if err != nil {
		return err
	}

	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.paused",
		Payload: PlaybackEvent{
			Type:  "playback.paused",
			State: domain.PlaybackPaused,
		},
	})

	return nil
}

// Resume resumes paused playback
func (s *PlaybackService) Resume() error {
	err := s.player.Resume()
	if err != nil {
		return err
	}

	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.resumed",
		Payload: PlaybackEvent{
			Type:  "playback.resumed",
			State: domain.PlaybackPlaying,
		},
	})

	return nil
}

// Stop stops playback
func (s *PlaybackService) Stop() error {
	err := s.player.Stop()
	if err != nil {
		return err
	}

	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.stopped",
		Payload: PlaybackEvent{
			Type:  "playback.stopped",
			State: domain.PlaybackStopped,
		},
	})

	return nil
}

// Seek seeks to a specific position
func (s *PlaybackService) Seek(pos int) error {
	err := s.player.Seek(pos)
	if err != nil {
		return err
	}

	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.seeked",
		Payload: PlaybackEvent{
			Type:     "playback.seeked",
			Position: pos,
		},
	})

	return nil
}

// SetVolume sets the playback volume
func (s *PlaybackService) SetVolume(volume float64) {
	s.player.SetVolume(volume)

	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.volume_changed",
		Payload: PlaybackEvent{
			Type:   "playback.volume_changed",
			Volume: volume,
		},
	})
}

// State returns the current playback state
func (s *PlaybackService) State() domain.PlaybackState {
	return s.player.State()
}

// CurrentTrack returns the currently playing track
func (s *PlaybackService) CurrentTrack() *domain.Track {
	return s.player.CurrentTrack()
}

// Position returns the current playback position
func (s *PlaybackService) Position() int {
	return s.player.Position()
}

// Length returns the total track length
func (s *PlaybackService) Length() int {
	return s.player.Length()
}

// GetVolume returns the current volume
func (s *PlaybackService) GetVolume() float64 {
	return s.player.GetVolume()
}

// SampleRate returns the current sample rate
func (s *PlaybackService) SampleRate() int {
	return s.player.SampleRate()
}

// UpdatePosition updates the current position (should be called periodically)
func (s *PlaybackService) UpdatePosition() {
	s.player.UpdatePosition()
	s.broker.Publish(pubsub.Event[PlaybackEvent]{
		Type: "playback.position",
		Payload: PlaybackEvent{
			Type:       "playback.position",
			Position:   s.player.Position(),
			Length:     s.player.Length(),
			SampleRate: s.player.SampleRate(),
		},
	})
}

// IsPlaying returns true if playing
func (s *PlaybackService) IsPlaying() bool {
	return s.player.IsPlaying()
}

// IsPaused returns true if paused
func (s *PlaybackService) IsPaused() bool {
	return s.player.IsPaused()
}

// Close closes the service and releases resources
func (s *PlaybackService) Close() error {
	s.broker.Close()
	return s.player.Close()
}
