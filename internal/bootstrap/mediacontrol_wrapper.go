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
)

type MediaControlWrapper struct {
	daemonClient     *mediacontrol.DaemonClient
	pbService        *service.PlaybackService
	appCtx           *app.AppContext
	ctx              context.Context
	lastPositionSent time.Time
}

func (w *MediaControlWrapper) Start(ctx context.Context) error {
	w.ctx = ctx

	if err := w.daemonClient.Start(ctx); err != nil {
		_ = err // TODO: log error
	}

	playbackEvents := w.pbService.Subscribe(ctx)
	daemonCommands := w.daemonClient.Commands()

	for {
		select {
		case <-ctx.Done():
			return w.daemonClient.Stop()

		case event := <-playbackEvents:
			if event.Type != "playback.position" {
				_ = event // TODO: log error
			}
			w.handlePlaybackEvent(event.Payload)

		case cmd := <-daemonCommands:
			w.handleDaemonCommand(cmd)
		}
	}
}

func (w *MediaControlWrapper) handlePlaybackEvent(event domain.PlaybackEvent) {
	switch event.Type {
	case "playback.started":
		if event.Track != nil {
			artwork := w.appCtx.Playback.AlbumArt()
			if err := w.daemonClient.SendPlaybackStarted(event.Track, artwork); err != nil {
				_ = err // TODO: log error
			}
		}

	case "playback.paused":
		if err := w.daemonClient.SendPlaybackPaused(event.Position, event.SampleRate); err != nil {
			_ = err // TODO: log error
		}

	case "playback.resumed":
		if err := w.daemonClient.SendPlaybackResumed(event.Position, event.SampleRate); err != nil {
			_ = err // TODO: log error
		}

	case "playback.stopped":
		if err := w.daemonClient.SendPlaybackStopped(); err != nil {
			_ = err // TODO: log error
		}

	case "playback.position":
		if event.State == domain.PlaybackPlaying && time.Since(w.lastPositionSent) >= 250*time.Millisecond &&
			event.SampleRate > 0 {
			w.lastPositionSent = time.Now()
			w.daemonClient.SendPosition(event.Position, event.Duration, event.SampleRate)
		}

	case "playback.seeked":
		if err := w.daemonClient.SendPosition(event.Position, event.Duration, event.SampleRate); err != nil {
			_ = err // TODO: log error
		}

	case "playback.artwork":
		if event.Artwork != nil {
			if err := w.daemonClient.SendArtworkImage(event.Artwork); err != nil {
				_ = err // TODO: log error
			}
		}
	}
}

func (w *MediaControlWrapper) handleDaemonCommand(cmd mediacontrol.DaemonCommand) {
	switch cmd.Type {
	case "play":
		state := w.pbService.GetState()
		if state == domain.PlaybackPaused {
			if err := w.pbService.Resume(); err != nil {
				_ = err // TODO: log error
			}
		}

	case "pause":
		state := w.pbService.GetState()
		if state == domain.PlaybackPlaying {
			if err := w.pbService.Pause(); err != nil {
				_ = err // TODO: log error
			}
		}

	case "toggle_play_pause":
		switch state := w.pbService.GetState(); state {
		case domain.PlaybackPlaying:
			if err := w.pbService.Pause(); err != nil {
				_ = err // TODO: log error
			}
		case domain.PlaybackPaused:
			if err := w.pbService.Resume(); err != nil {
				_ = err // TODO: log error
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
				if err := w.pbService.SeekToSeconds(posSeconds); err != nil {
					_ = err // TODO: log error
				}
			}
		}

	case "next":
		if w.appCtx != nil && w.pbService != nil {
			libSvc := w.appCtx.Services.LibraryService()
			if libSvc == nil {
				// TODO: Add logging
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
				// TODO: Add logging
				return
			}

			if next != nil {
				if err := w.pbService.PlayDomainTrack(w.ctx, libSvc, next); err != nil {
					// TODO: Add logging
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
			libSvc := w.appCtx.Services.LibraryService()
			if libSvc == nil {
				// TODO: Add logging
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
				// TODO: Add logging
				return
			}

			if prev != nil {
				if err := w.pbService.PlayDomainTrack(w.ctx, libSvc, prev); err != nil {
					// TODO: Add logging
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
		// TODO: Add logging
	}
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
		appCtx:       coordinator.GetAppContext(),
	}
}
