//go:build darwin

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

type MediaControlWrapper struct {
	daemonClient     *mediacontrol.DaemonClient
	pbService        *service.PlaybackService
	orchestrator     *tui.Orchestrator
	coordinator      *app.Coordinator
	appCtx           *app.AppContext
	ctx              context.Context
	lastPositionSent time.Time
}

func (w *MediaControlWrapper) Start(ctx context.Context) error {
	w.ctx = ctx
	log.Info("Starting media control integration with daemon")

	if err := w.daemonClient.Start(ctx); err != nil {
		log.Warn("Failed to start daemon client: %v (will retry in background)", err)
	}

	playbackEvents := w.pbService.Subscribe(ctx)
	daemonCommands := w.daemonClient.Commands()

	log.Info("Media control integration started, listening for events")

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping media control integration")
			return w.daemonClient.Stop()

		case event := <-playbackEvents:
			if event.Type != "playback.position" {
				log.Debug("Received playback event: %s", event.Type)
			}
			w.handlePlaybackEvent(event.Payload)

		case cmd := <-daemonCommands:
			log.Debug("Received daemon command: %s", cmd.Type)
			w.handleDaemonCommand(cmd)
		}
	}
}

func (w *MediaControlWrapper) handlePlaybackEvent(event domain.PlaybackEvent) {
	switch event.Type {
	case "playback.started":
		if event.Track != nil {
			log.Debug("Sending playback.started to daemon: %s - %s", event.Track.Artist, event.Track.Title)
			artwork := w.appCtx.Playback.AlbumArt()
			if err := w.daemonClient.SendPlaybackStarted(event.Track, artwork); err != nil {
				log.Debug("Failed to send playback.started: %v", err)
			}
		}

	case "playback.paused":
		log.Debug("Sending playback.paused to daemon")
		if err := w.daemonClient.SendPlaybackPaused(event.Position, event.SampleRate); err != nil {
			log.Debug("Failed to send playback.paused: %v", err)
		}

	case "playback.resumed":
		log.Debug("Sending playback.resumed to daemon")
		if err := w.daemonClient.SendPlaybackResumed(event.Position, event.SampleRate); err != nil {
			log.Debug("Failed to send playback.resumed: %v", err)
		}

	case "playback.stopped":
		log.Debug("Sending playback.stopped to daemon")
		if err := w.daemonClient.SendPlaybackStopped(); err != nil {
			log.Debug("Failed to send playback.stopped: %v", err)
		}

	case "playback.position":
		if event.State == domain.PlaybackPlaying && time.Since(w.lastPositionSent) >= 250*time.Millisecond &&
			event.SampleRate > 0 {
			w.lastPositionSent = time.Now()
			w.daemonClient.SendPosition(event.Position, event.Duration, event.SampleRate)
		}

	case "playback.seeked":
		log.Debug("Sending playback.seeked to daemon")
		if err := w.daemonClient.SendPosition(event.Position, event.Duration, event.SampleRate); err != nil {
			log.Debug("Failed to send playback.seeked: %v", err)
		}

	case "playback.artwork":
		if event.Artwork != nil {
			log.Debug("Sending playback.artwork to daemon")
			if err := w.daemonClient.SendArtworkImage(event.Artwork); err != nil {
				log.Debug("Failed to send playback.artwork: %v", err)
			}
		}
	}
}

func (w *MediaControlWrapper) handleDaemonCommand(cmd mediacontrol.DaemonCommand) {
	switch cmd.Type {
	case "play":
		state := w.pbService.GetState()
		if state == domain.PlaybackPaused {
			log.Info("Remote command: Resuming playback")
			if err := w.pbService.Resume(); err != nil {
				log.Warn("Failed to resume playback: %v", err)
			}
		}

	case "pause":
		state := w.pbService.GetState()
		if state == domain.PlaybackPlaying {
			log.Info("Remote command: Pausing playback")
			if err := w.pbService.Pause(); err != nil {
				log.Warn("Failed to pause playback: %v", err)
			}
		}

	case "toggle_play_pause":
		switch state := w.pbService.GetState(); state {
		case domain.PlaybackPlaying:
			log.Info("Media key: Pausing playback")
			if err := w.pbService.Pause(); err != nil {
				log.Warn("Failed to pause playback: %v", err)
			}
		case domain.PlaybackPaused:
			log.Info("Media key: Resuming playback")
			if err := w.pbService.Resume(); err != nil {
				log.Warn("Failed to resume playback: %v", err)
			}
		}

	case "seek":
		if cmd.Data != nil {
			if posVal, ok := cmd.Data["position"]; ok {
				var posSeconds float64
				switch v := posVal.(type) {
				case float64:
					posSeconds = v
				case int:
					posSeconds = float64(v)
				case int64:
					posSeconds = float64(v)
				}
				log.Info("Remote command: Seeking to %.2f seconds", posSeconds)
				if err := w.pbService.SeekToSeconds(posSeconds); err != nil {
					log.Warn("Failed to seek: %v", err)
				}
			}
		}

	case "next":
		if w.appCtx != nil && w.pbService != nil {
			log.Info("Media key: Playing next track")

			libSvc := w.appCtx.Services.LibraryService()
			if libSvc == nil {
				log.Warn("Media key: Cannot play next track - library service not available")
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
				log.Warn("Failed to determine next track: %v", err)
				return
			}

			if next != nil {
				if err := w.pbService.PlayDomainTrack(w.ctx, libSvc, next); err != nil {
					log.Warn("Failed to play next track: %v", err)
					return
				}
				if isQueue {
					w.appCtx.Content.SetQueueIndex(newQueueIdx)
				} else {
					w.appCtx.View.SetSelectedTrack(newSelected)
				}
			}
		}

	case "previous":
		if w.appCtx != nil && w.pbService != nil {
			log.Info("Media key: Playing previous track")

			libSvc := w.appCtx.Services.LibraryService()
			if libSvc == nil {
				log.Warn("Media key: Cannot play previous track - library service not available")
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
				log.Warn("Failed to determine previous track: %v", err)
				return
			}

			if prev != nil {
				if err := w.pbService.PlayDomainTrack(w.ctx, libSvc, prev); err != nil {
					log.Warn("Failed to play previous track: %v", err)
					return
				}
				if isQueue {
					w.appCtx.Content.SetQueueIndex(newQueueIdx)
				} else {
					w.appCtx.View.SetSelectedTrack(newSelected)
				}
			}
		}

	default:
		log.Warn("Unknown daemon command: %s", cmd.Type)
	}
}

func convertAppTracksToDomain(tracks []app.Track) []domain.Track {
	result := make([]domain.Track, len(tracks))
	for i, t := range tracks {
		dt := domain.Track{
			Title:           t.Title,
			Artist:          t.Artist,
			Album:           t.Album,
			Duration:        t.Duration,
			TrackNumber:     t.TrackNumber,
			PlaylistItemID:  t.PlaylistItemID,
			PlayQueueItemID: t.PlayQueueItemID,
			Key:             t.Key,
			RatingKey:       t.RatingKey,
			Thumb:           t.Thumb,
		}
		if len(t.Media) > 0 {
			dt.Media = make([]struct {
				Part []struct {
					Key string `json:"key"`
				} `json:"Part"`
			}, len(t.Media))
			for j, m := range t.Media {
				if len(m.Part) > 0 {
					dt.Media[j].Part = make([]struct {
						Key string `json:"key"`
					}, len(m.Part))
					for k, p := range m.Part {
						dt.Media[j].Part[k].Key = p.Key
					}
				}
			}
		}
		result[i] = dt
	}
	return result
}

// provideMediaControlWrapper creates the media control wrapper with daemon client
func provideMediaControlWrapper(
	playbackService *service.PlaybackService,
	orchestrator *tui.Orchestrator,
	coordinator *app.Coordinator,
) *MediaControlWrapper {
	daemonClient := mediacontrol.NewDaemonClient()

	return &MediaControlWrapper{
		daemonClient: daemonClient,
		pbService:    playbackService,
		orchestrator: orchestrator,
		coordinator:  coordinator,
		appCtx:       coordinator.GetAppContext(),
	}
}
