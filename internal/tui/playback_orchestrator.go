package tui

import (
	"context"
	"fmt"
	"io"
	"time"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	_ "plexmusic-tui/internal/tui/styles"
	"plexmusic-tui/internal/tui/util"
)

// Orchestrator provides a high-level API for the UI to start playback and
// navigate the queue/tracklist, while keeping playback logic out of the page.
type Orchestrator struct {
	coordinator app.Coordinatorer
	libSvc      interface {
		FetchStream(ctx context.Context, t *domain.Track) (io.ReadCloser, string, error)
	}
	pbSvc service.PlaybackServicer
}

// NewOrchestrator creates an orchestrator instance.
func NewOrchestrator(coord app.Coordinatorer, lib interface {
	FetchStream(ctx context.Context, t *domain.Track) (io.ReadCloser, string, error)
}, pb service.PlaybackServicer,
) *Orchestrator {
	return &Orchestrator{coordinator: coord, libSvc: lib, pbSvc: pb}
}

// Pause pauses playback, updating coordinator state and returning any error from pbSvc.
func (o *Orchestrator) Pause() error {
	if o.pbSvc == nil {
		return fmt.Errorf("playback service unavailable")
	}
	if err := o.pbSvc.Pause(); err != nil {
		if o.coordinator != nil {
			o.coordinator.SetNotification(
				fmt.Sprintf("Pause failed: %v", err),
				"error",
				5*time.Second,
			)
		}
		return err
	}
	if o.coordinator != nil {
		o.coordinator.SetPlaybackState(app.PlaybackPaused)
	}
	return nil
}

// Resume resumes playback, updating coordinator if necessary.
func (o *Orchestrator) Resume() error {
	if o.pbSvc == nil {
		return fmt.Errorf("playback service unavailable")
	}
	if err := o.pbSvc.Resume(); err != nil {
		if o.coordinator != nil {
			o.coordinator.SetNotification(
				fmt.Sprintf("Resume failed: %v", err),
				"error",
				5*time.Second,
			)
		}
		return err
	}
	if o.coordinator != nil {
		o.coordinator.SetPlaybackState(app.PlaybackPlaying)
	}
	return nil
}

// Stop stops playback, updating coordinator state.
func (o *Orchestrator) Stop() error {
	if o.pbSvc == nil {
		return fmt.Errorf("playback service unavailable")
	}
	if err := o.pbSvc.Stop(); err != nil {
		if o.coordinator != nil {
			o.coordinator.SetNotification(
				fmt.Sprintf("Stop failed: %v", err),
				"error",
				5*time.Second,
			)
		}
		return err
	}
	if o.coordinator != nil {
		o.coordinator.SetPlaybackState(app.PlaybackStopped)
	}
	return nil
}

// Seek delegates a position seek to the playback service and updates coordinator position.
func (o *Orchestrator) Seek(pos int) error {
	if o.pbSvc == nil {
		return fmt.Errorf("playback service unavailable")
	}
	if err := o.pbSvc.Seek(pos); err != nil {
		if o.coordinator != nil {
			o.coordinator.SetNotification(
				fmt.Sprintf("Seek failed: %v", err),
				"error",
				5*time.Second,
			)
		}
		return err
	}
	if o.coordinator != nil {
		o.coordinator.SetStreamPosition(pos)
	}
	return nil
}

// AdjustVolumeByPercent adjusts the volume using a playback controller helper.
func (o *Orchestrator) AdjustVolumeByPercent(percentageDelta int) error {
	if o.pbSvc == nil {
		return fmt.Errorf("playback service unavailable")
	}
	pc := service.NewPlaybackController(o.pbSvc)
	pc.AdjustVolume(float64(percentageDelta))
	if cfg := o.coordinator.ConfigManager(); cfg != nil {
		newVol := o.pbSvc.GetVolume()
		cfg.SetVolume(newVol)
		_ = cfg.Save()
	}
	return nil
}

// PlaybackServicer compatibility: Play and PlayDomainTrack wrappers
func (o *Orchestrator) Play(t *domain.Track) error {
	if o.pbSvc == nil {
		return fmt.Errorf("playback service unavailable")
	}
	return o.pbSvc.Play(t)
}

func (o *Orchestrator) PlayDomainTrack(ctx context.Context, lib interface {
	FetchStream(ctx context.Context, t *domain.Track) (io.ReadCloser, string, error)
}, track *domain.Track,
) error {
	if o.pbSvc == nil {
		return fmt.Errorf("playback service unavailable")
	}
	return o.pbSvc.PlayDomainTrack(ctx, lib, track)
}

// Get/Set wrappers to make orchestrator satisfy PlaybackServicer for UI usage
func (o *Orchestrator) GetVolume() float64 {
	if o.pbSvc == nil {
		return 0
	}
	return o.pbSvc.GetVolume()
}

func (o *Orchestrator) SetVolume(v float64) {
	if o.pbSvc != nil {
		o.pbSvc.SetVolume(v)
	}
}

// AdjustVolume adjusts the volume by a percentage delta using the PlaybackController logic.
func (o *Orchestrator) AdjustVolume(percentDelta float64) {
	if o.pbSvc == nil {
		return
	}
	pc := service.NewPlaybackController(o.pbSvc)
	pc.AdjustVolume(percentDelta)
}

func (o *Orchestrator) GetPosition() int {
	if o.pbSvc == nil {
		return 0
	}
	return o.pbSvc.GetPosition()
}

func (o *Orchestrator) GetDuration() int {
	if o.pbSvc == nil {
		return 0
	}
	return o.pbSvc.GetDuration()
}

func (o *Orchestrator) GetState() domain.PlaybackState {
	if o.pbSvc == nil {
		return domain.PlaybackStopped
	}
	return o.pbSvc.GetState()
}

// Subscribe proxies event subscriptions to the underlying playback service broker
func (o *Orchestrator) Subscribe(ctx context.Context) <-chan pubsub.Event[domain.PlaybackEvent] {
	if o.pbSvc == nil {
		ch := make(chan pubsub.Event[domain.PlaybackEvent], 1)
		close(ch)
		return ch
	}
	return o.pbSvc.Subscribe(ctx)
}

// PlayAppTrack plays an app.Track, handling conversion and coordinator updates and errors.
func (o *Orchestrator) PlayAppTrack(ctx context.Context, at *app.Track) error {
	if at == nil {
		return nil
	}
	dt := util.AppTrackToDomain(at)
	if dt == nil {
		return fmt.Errorf("failed to convert track")
	}
	// Attempt to fetch cover art and set it (page-level handlers may update playback art)
	// Not strictly necessary here; page will also fetch cover art via commands in UI.

	// Try to play via pbSvc
	if o.pbSvc != nil {
		if o.libSvc != nil {
			if err := o.pbSvc.PlayDomainTrack(ctx, o.libSvc, dt); err != nil {
				// Set coordinator notification
				if o.coordinator != nil {
					o.coordinator.SetNotification(
						fmt.Sprintf("Play failed: %v", err),
						"error",
						10*time.Second,
					)
				}
				return err
			}
		} else {
			if err := o.pbSvc.Play(dt); err != nil {
				if o.coordinator != nil {
					o.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", err), "error", 10*time.Second)
				}
				return err
			}
		}
	}
	// Set coordinator current track for immediate UI feedback
	if o.coordinator != nil {
		if at != nil {
			o.coordinator.SetCurrentTrack(at)
			o.coordinator.SetPlaybackState(app.PlaybackPlaying)
		}
	}
	return nil
}

// PlayNext is a helper that uses the given PlaybackController to compute and start
// playing the next track, reverting to returning an error if orchestration fails.
func (o *Orchestrator) PlayNext(
	ctx context.Context,
	pc *service.PlaybackController,
	queue []app.Track,
	queueIndex int,
	tracks []app.Track,
	selected int,
) error {
	// Convert app.Track to domain.Track slices
	dq := make([]domain.Track, len(queue))
	for i, t := range queue {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			dq[i] = *dt
		}
	}
	dtracks := make([]domain.Track, len(tracks))
	for i, t := range tracks {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			dtracks[i] = *dt
		}
	}
	isQueue, newQueueIdx, newSelected, next, err := pc.PlayNext(
		ctx,
		dq,
		queueIndex,
		dtracks,
		selected,
		o.libSvc,
	)
	if err != nil {
		// Although controller no longer calls pbSvc, preserve handling if controller returns error
		if o.coordinator != nil {
			o.coordinator.SetNotification(
				fmt.Sprintf("Play failed: %v", err),
				"error",
				10*time.Second,
			)
		}
		return err
	}

	// If a queue is in use and the controller signalled completion (newQueueIdx == -1),
	// stop playback and clear the queue selection instead of wrapping. This avoids
	// infinite looping of queued tracks — when the queue completes, stop and clear
	// the queue selection/state.
	if isQueue && len(dq) > 0 && newQueueIdx == -1 {
		if o.pbSvc != nil {
			if err := o.pbSvc.Stop(); err != nil {
				if o.coordinator != nil {
					o.coordinator.SetNotification(
						fmt.Sprintf("Stop failed: %v", err),
						"error",
						10*time.Second,
					)
				}
				return err
			}
		} else {
			if o.coordinator != nil {
				o.coordinator.SetPlaybackState(app.PlaybackStopped)
			}
		}
		if o.coordinator != nil {
			o.coordinator.SetQueueIndex(-1)
			o.coordinator.SetCurrentTrack(nil)
		}
		return nil
	}

	if isQueue {
		o.coordinator.SetQueueIndex(newQueueIdx)
	} else {
		o.coordinator.SetSelectedTrack(newSelected)
	}
	if next != nil {
		if at := util.DomainTrackToApp(next); at != nil {
			o.coordinator.SetCurrentTrack(at)
			o.coordinator.SetPlaybackState(app.PlaybackPlaying)
		}
		// Attempt to start playback via pbSvc
		if o.pbSvc != nil {
			if o.libSvc != nil {
				if err := o.pbSvc.PlayDomainTrack(ctx, o.libSvc, next); err != nil {
					if o.coordinator != nil {
						o.coordinator.SetNotification(
							fmt.Sprintf("Play failed: %v", err),
							"error",
							10*time.Second,
						)
					}
					return err
				}
			} else {
				if err := o.pbSvc.Play(next); err != nil {
					if o.coordinator != nil {
						o.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", err), "error", 10*time.Second)
					}
					return err
				}
			}
		} else {
			if o.coordinator != nil {
				o.coordinator.SetNotification("Play failed: playback service unavailable", "error", 10*time.Second)
			}
			return fmt.Errorf("playback service unavailable")
		}
	}
	return nil
}

// PlayPrev mirrors PlayNext for previous selection.
func (o *Orchestrator) PlayPrev(
	ctx context.Context,
	pc *service.PlaybackController,
	queue []app.Track,
	queueIndex int,
	tracks []app.Track,
	selected int,
) error {
	dq := make([]domain.Track, len(queue))
	for i, t := range queue {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			dq[i] = *dt
		}
	}
	dtracks := make([]domain.Track, len(tracks))
	for i, t := range tracks {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			dtracks[i] = *dt
		}
	}
	isQueue, newQueueIdx, newSelected, prev, err := pc.PlayPrev(
		ctx,
		dq,
		queueIndex,
		dtracks,
		selected,
		o.libSvc,
	)
	if err != nil {
		if o.coordinator != nil {
			o.coordinator.SetNotification(
				fmt.Sprintf("Play failed: %v", err),
				"error",
				10*time.Second,
			)
		}
		return err
	}
	if isQueue {
		o.coordinator.SetQueueIndex(newQueueIdx)
	} else {
		o.coordinator.SetSelectedTrack(newSelected)
	}
	if prev != nil {
		if at := util.DomainTrackToApp(prev); at != nil {
			o.coordinator.SetCurrentTrack(at)
			o.coordinator.SetPlaybackState(app.PlaybackPlaying)
		}
		// Attempt to start playback via pbSvc using prev
		if o.pbSvc != nil {
			if o.libSvc != nil {
				if err := o.pbSvc.PlayDomainTrack(ctx, o.libSvc, prev); err != nil {
					if o.coordinator != nil {
						o.coordinator.SetNotification(
							fmt.Sprintf("Play failed: %v", err),
							"error",
							10*time.Second,
						)
					}
					return err
				}
			} else {
				if err := o.pbSvc.Play(prev); err != nil {
					if o.coordinator != nil {
						o.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", err), "error", 10*time.Second)
					}
					return err
				}
			}
		} else {
			if o.coordinator != nil {
				o.coordinator.SetNotification("Play failed: playback service unavailable", "error", 10*time.Second)
			}
			return fmt.Errorf("playback service unavailable")
		}
	}
	return nil
}
