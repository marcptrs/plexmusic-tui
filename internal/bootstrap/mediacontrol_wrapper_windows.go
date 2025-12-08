//go:build windows

package bootstrap

import (
	"context"
	"time"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/mediacontrol"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"

	log "github.com/charmbracelet/log/v2"
)

// MediaControlWrapper for Windows uses the in-process SMTC controller
// (no daemon required unlike macOS)
type MediaControlWrapper struct {
	controller       mediacontrol.MediaController
	pbService        *service.PlaybackService
	appCtx           *app.AppContext
	ctx              context.Context
	lastPositionSent time.Time
}

// commandHandler adapts playback operations to the CommandHandler interface
type commandHandler struct {
	wrapper *MediaControlWrapper
}

func (h *commandHandler) HandlePlay() {
	log.Info("Windows SMTC: HandlePlay")
	if h.wrapper.pbService.GetState() == domain.PlaybackPaused {
		if err := h.wrapper.pbService.Resume(); err != nil {
			log.Warn("Failed to resume", "error", err)
		}
	}
}

func (h *commandHandler) HandlePause() {
	log.Info("Windows SMTC: HandlePause")
	if h.wrapper.pbService.GetState() == domain.PlaybackPlaying {
		if err := h.wrapper.pbService.Pause(); err != nil {
			log.Warn("Failed to pause", "error", err)
		}
	}
}

func (h *commandHandler) HandleTogglePlayPause() {
	log.Info("Windows SMTC: HandleTogglePlayPause")
	switch h.wrapper.pbService.GetState() {
	case domain.PlaybackPlaying:
		if err := h.wrapper.pbService.Pause(); err != nil {
			log.Warn("Failed to pause", "error", err)
		}
	case domain.PlaybackPaused:
		if err := h.wrapper.pbService.Resume(); err != nil {
			log.Warn("Failed to resume", "error", err)
		}
	}
}

func (h *commandHandler) HandleStop() {
	log.Info("Windows SMTC: HandleStop")
	if err := h.wrapper.pbService.Stop(); err != nil {
		log.Warn("Failed to stop", "error", err)
	}
}

func (h *commandHandler) HandleNext() {
	log.Info("Windows SMTC: HandleNext")
	h.wrapper.playNext()
}

func (h *commandHandler) HandlePrevious() {
	log.Info("Windows SMTC: HandlePrevious")
	h.wrapper.playPrev()
}

func (h *commandHandler) HandleSeek(position time.Duration) {
	log.Info("Windows SMTC: HandleSeek", "position", position)
	if err := h.wrapper.pbService.SeekToSeconds(position.Seconds()); err != nil {
		log.Warn("Failed to seek", "error", err)
	}
}

func (w *MediaControlWrapper) playNext() {
	if w.appCtx == nil || w.pbService == nil {
		return
	}

	libSvc := w.appCtx.Services.LibraryService()
	if libSvc == nil {
		log.Warn("Cannot play next: library service not available")
		return
	}

	pc := service.NewPlaybackController(nil)
	queue := w.appCtx.Content.Queue()
	queueIdx := w.appCtx.Content.QueueIndex()
	tracks := w.appCtx.Content.Tracks()
	selected := w.appCtx.View.SelectedTrack()

	dq := queue
	dtracks := tracks

	isQueue, newQueueIdx, newSelected, next, err := pc.PlayNext(
		w.ctx, dq, queueIdx, dtracks, selected, libSvc,
	)
	if err != nil {
		log.Warn("Failed to determine next track", "error", err)
		return
	}

	if next != nil {
		if err := w.pbService.PlayDomainTrack(w.ctx, libSvc, next); err != nil {
			log.Warn("Failed to play next track", "error", err)
			return
		}
		if isQueue {
			w.appCtx.Content.SetQueueIndex(newQueueIdx)
		} else {
			w.appCtx.View.SetSelectedTrack(newSelected)
		}
	}
}

func (w *MediaControlWrapper) playPrev() {
	if w.appCtx == nil || w.pbService == nil {
		return
	}

	libSvc := w.appCtx.Services.LibraryService()
	if libSvc == nil {
		log.Warn("Cannot play previous: library service not available")
		return
	}

	pc := service.NewPlaybackController(nil)
	queue := w.appCtx.Content.Queue()
	queueIdx := w.appCtx.Content.QueueIndex()
	tracks := w.appCtx.Content.Tracks()
	selected := w.appCtx.View.SelectedTrack()

	dq := queue
	dtracks := tracks

	isQueue, newQueueIdx, newSelected, prev, err := pc.PlayPrev(
		w.ctx, dq, queueIdx, dtracks, selected, libSvc,
	)
	if err != nil {
		log.Warn("Failed to determine previous track", "error", err)
		return
	}

	if prev != nil {
		if err := w.pbService.PlayDomainTrack(w.ctx, libSvc, prev); err != nil {
			log.Warn("Failed to play previous track", "error", err)
			return
		}
		if isQueue {
			w.appCtx.Content.SetQueueIndex(newQueueIdx)
		} else {
			w.appCtx.View.SetSelectedTrack(newSelected)
		}
	}
}

// Start begins the Windows media control integration
func (w *MediaControlWrapper) Start(ctx context.Context) error {
	w.ctx = ctx
	log.Info("Starting Windows SMTC media control integration")

	// Create the in-process controller
	controller, err := mediacontrol.New()
	if err != nil {
		log.Warn("Failed to create Windows media controller", "error", err)
		return err
	}
	w.controller = controller

	// Start the controller
	if err := w.controller.Start(ctx); err != nil {
		return err
	}

	// Set up command handler
	handler := &commandHandler{wrapper: w}
	if err := w.controller.SetCommandHandler(handler); err != nil {
		log.Warn("Failed to set command handler", "error", err)
	}

	// Subscribe to playback events
	playbackEvents := w.pbService.Subscribe(ctx)

	log.Info("Windows SMTC integration started, listening for events")

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping Windows SMTC integration")
			return w.controller.Stop()

		case event := <-playbackEvents:
			if event.Type != "playback.position" {
				log.Debug("Windows SMTC received event", "type", event.Type)
			}
			w.handlePlaybackEvent(event.Payload)
		}
	}
}

func (w *MediaControlWrapper) handlePlaybackEvent(event domain.PlaybackEvent) {
	switch event.Type {
	case "playback.started":
		if event.Track != nil {
			log.Debug("Windows SMTC: updating metadata", "artist", event.Track.Artist, "title", event.Track.Title)
			w.controller.UpdateMetadata(mediacontrol.Metadata{
				Title:    event.Track.Title,
				Artist:   event.Track.Artist,
				Album:    event.Track.Album,
				Duration: time.Duration(event.Track.Duration) * time.Millisecond,
			})
			w.controller.UpdatePlaybackState(mediacontrol.StatePlaying)

			// Set artwork if available
			artwork := w.appCtx.Playback.AlbumArt()
			if artwork != nil {
				w.controller.SetArtwork(artwork)
			}
		}

	case "playback.paused":
		w.controller.UpdatePlaybackState(mediacontrol.StatePaused)

	case "playback.resumed":
		w.controller.UpdatePlaybackState(mediacontrol.StatePlaying)

	case "playback.stopped":
		w.controller.UpdatePlaybackState(mediacontrol.StateStopped)

	case "playback.position":
		if event.State == domain.PlaybackPlaying && time.Since(w.lastPositionSent) >= 250*time.Millisecond &&
			event.SampleRate > 0 {
			w.lastPositionSent = time.Now()
			posSeconds := float64(event.Position) / float64(event.SampleRate)
			durSeconds := float64(event.Duration) / float64(event.SampleRate)
			w.controller.UpdatePosition(
				time.Duration(posSeconds*float64(time.Second)),
				time.Duration(durSeconds*float64(time.Second)),
			)
		}

	case "playback.artwork":
		if event.Artwork != nil {
			log.Debug("Windows SMTC: setting artwork")
			w.controller.SetArtwork(event.Artwork)
		}
	}
}

// InProcessMediaControl is the same as MediaControlWrapper on Windows
// (kept for interface compatibility)
type InProcessMediaControl = MediaControlWrapper

// NewInProcessMediaControl creates a new Windows media control handler
func NewInProcessMediaControl(
	playbackService *service.PlaybackService,
	orchestrator *tui.Orchestrator,
	coordinator *app.Coordinator,
) (*InProcessMediaControl, error) {
	return &MediaControlWrapper{
		pbService: playbackService,
		appCtx:    coordinator.GetAppContext(),
	}, nil
}

// provideMediaControlWrapper creates the Windows media control wrapper
func provideMediaControlWrapper(
	playbackService *service.PlaybackService,
	orchestrator *tui.Orchestrator,
	coordinator *app.Coordinator,
) *MediaControlWrapper {
	return &MediaControlWrapper{
		pbService: playbackService,
		appCtx:    coordinator.GetAppContext(),
	}
}
