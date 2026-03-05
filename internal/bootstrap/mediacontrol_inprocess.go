//go:build darwin

package bootstrap

import (
	"context"
	"time"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/logging"
	"plexmusic-tui/internal/mediacontrol"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
)

// InProcessMediaControl handles media control integration using the in-process
// CGo bridge instead of the separate daemon process.
type InProcessMediaControl struct {
	controller       mediacontrol.MediaController
	pbService        *service.PlaybackService
	orchestrator     *tui.Orchestrator
	appCtx           *app.AppContext
	ctx              context.Context
	lastPositionSent time.Time
	logger           logging.Logger
}

// NewInProcessMediaControl creates a new in-process media control handler.
func NewInProcessMediaControl(
	playbackService *service.PlaybackService,
	orchestrator *tui.Orchestrator,
	coordinator *app.Coordinator,
) (*InProcessMediaControl, error) {
	controller, err := mediacontrol.New()
	if err != nil {
		return nil, err
	}

	logger := coordinator.GetLogger()
	return &InProcessMediaControl{
		controller:   controller,
		pbService:    playbackService,
		orchestrator: orchestrator,
		appCtx:       coordinator.GetAppContext(),
		logger:       logger,
	}, nil
}

// commandHandler adapts playback operations to the CommandHandler interface
type commandHandler struct {
	ctrl *InProcessMediaControl
}

func (h *commandHandler) HandlePlay() {
	if h.ctrl.pbService.GetState() == domain.PlaybackPaused {
		if err := h.ctrl.pbService.Resume(); err != nil {
			// TODO: Add logging
			_ = err // satisfy linter
		}
	}
}

func (h *commandHandler) HandlePause() {
	if h.ctrl.pbService.GetState() == domain.PlaybackPlaying {
		if err := h.ctrl.pbService.Pause(); err != nil {
			// TODO: Add logging
			_ = err // satisfy linter
		}
	}
}

func (h *commandHandler) HandleTogglePlayPause() {
	switch h.ctrl.pbService.GetState() {
	case domain.PlaybackPlaying:
		if err := h.ctrl.pbService.Pause(); err != nil {
			// TODO: Add logging
			_ = err // satisfy linter
		}
	case domain.PlaybackPaused:
		if err := h.ctrl.pbService.Resume(); err != nil {
			// TODO: Add logging
			_ = err // satisfy linter
		}
	}
}

func (h *commandHandler) HandleStop() {
	if err := h.ctrl.pbService.Stop(); err != nil {
		// TODO: Add logging
		_ = err // satisfy linter
	}
}

func (h *commandHandler) HandleNext() {
	h.ctrl.playNext()
}

func (h *commandHandler) HandlePrevious() {
	h.ctrl.playPrev()
}

func (h *commandHandler) HandleSeek(position time.Duration) {
	if err := h.ctrl.pbService.SeekToSeconds(position.Seconds()); err != nil {
		// TODO: Add logging
		_ = err // satisfy linter
	}
}

func (ctrl *InProcessMediaControl) playNext() {
	if ctrl.appCtx == nil || ctrl.pbService == nil {
		return
	}

	libSvc := ctrl.appCtx.Services.LibraryService()
	if libSvc == nil {
		// TODO: Add logging
		return
	}

	pc := service.NewPlaybackController(nil)
	queue := ctrl.appCtx.Content.Queue()
	queueIdx := ctrl.appCtx.Content.QueueIndex()
	tracks := ctrl.appCtx.Content.Tracks()
	selected := ctrl.appCtx.View.SelectedTrack()

	dq := queue
	dtracks := tracks

	isQueue, newQueueIdx, newSelected, next, err := pc.PlayNext(
		ctrl.ctx, dq, queueIdx, dtracks, selected, libSvc,
	)
	if err != nil {
		// TODO: Add logging
		return
	}

	if next != nil {
		if err := ctrl.pbService.PlayDomainTrack(ctrl.ctx, libSvc, next); err != nil {
			// TODO: Add logging
			return
		}
		if isQueue {
			ctrl.appCtx.Content.SetQueueIndex(newQueueIdx)
		} else {
			ctrl.appCtx.View.SetSelectedTrack(newSelected)
		}
	}
}

func (ctrl *InProcessMediaControl) playPrev() {
	if ctrl.appCtx == nil || ctrl.pbService == nil {
		return
	}

	libSvc := ctrl.appCtx.Services.LibraryService()
	if libSvc == nil {
		// TODO: Add logging
		return
	}

	pc := service.NewPlaybackController(nil)
	queue := ctrl.appCtx.Content.Queue()
	queueIdx := ctrl.appCtx.Content.QueueIndex()
	tracks := ctrl.appCtx.Content.Tracks()
	selected := ctrl.appCtx.View.SelectedTrack()

	dq := queue
	dtracks := tracks

	isQueue, newQueueIdx, newSelected, prev, err := pc.PlayPrev(
		ctrl.ctx, dq, queueIdx, dtracks, selected, libSvc,
	)
	if err != nil {
		// TODO: Add logging
		return
	}

	if prev != nil {
		if err := ctrl.pbService.PlayDomainTrack(ctrl.ctx, libSvc, prev); err != nil {
			// TODO: Add logging
			return
		}
		if isQueue {
			ctrl.appCtx.Content.SetQueueIndex(newQueueIdx)
		} else {
			ctrl.appCtx.View.SetSelectedTrack(newSelected)
		}
	}
}

// Start begins the in-process media control integration
func (ctrl *InProcessMediaControl) Start(ctx context.Context) error {
	ctrl.ctx = ctx

	// Start the controller
	if err := ctrl.controller.Start(ctx); err != nil {
		return err
	}

	// Set up command handler for remote commands from Control Center
	handler := &commandHandler{ctrl: ctrl}
	if err := ctrl.controller.SetCommandHandler(handler); err != nil {
		ctrl.logger.Error("Failed to set command handler", "error", err)
	}

	// Subscribe to playback events
	playbackEvents := ctrl.pbService.Subscribe(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctrl.controller.Stop()

		case event := <-playbackEvents:
			if event.Type != "playback.position" {
				_ = event // TODO: log error
			}
			ctrl.handlePlaybackEvent(event.Payload)
		}
	}
}

func (ctrl *InProcessMediaControl) handlePlaybackEvent(event domain.PlaybackEvent) {
	switch event.Type {
	case "playback.started":
		if event.Track != nil {
			ctrl.controller.UpdateMetadata(mediacontrol.Metadata{
				Title:    event.Track.Title,
				Artist:   event.Track.Artist,
				Album:    event.Track.Album,
				Duration: time.Duration(event.Track.Duration) * time.Millisecond,
			})
			ctrl.controller.UpdatePlaybackState(mediacontrol.StatePlaying)

			// Set artwork if available
			artwork := ctrl.appCtx.Playback.AlbumArt()
			if artwork != nil {
				ctrl.controller.SetArtwork(artwork)
			}
		}

	case "playback.paused":
		ctrl.controller.UpdatePlaybackState(mediacontrol.StatePaused)

	case "playback.resumed":
		ctrl.controller.UpdatePlaybackState(mediacontrol.StatePlaying)

	case "playback.stopped":
		ctrl.controller.UpdatePlaybackState(mediacontrol.StateStopped)

	case "playback.position":
		if event.State == domain.PlaybackPlaying && time.Since(ctrl.lastPositionSent) >= 250*time.Millisecond &&
			event.SampleRate > 0 {
			ctrl.lastPositionSent = time.Now()
			posSeconds := float64(event.Position) / float64(event.SampleRate)
			durSeconds := float64(event.Duration) / float64(event.SampleRate)
			ctrl.controller.UpdatePosition(
				time.Duration(posSeconds*float64(time.Second)),
				time.Duration(durSeconds*float64(time.Second)),
			)
		}

	case "playback.artwork":
		if event.Artwork != nil {
			ctrl.controller.SetArtwork(event.Artwork)
		}
	}
}
