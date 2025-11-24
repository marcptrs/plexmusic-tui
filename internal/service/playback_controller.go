package service

import (
	"context"
	"io"
	"math"

	"plexmusic-tui/internal/domain"

	log "github.com/charmbracelet/log/v2"
)

// PlaybackController provides simple playback volume control operations.
// It's intentionally minimal to avoid circular imports with the app package.
// Complex playback orchestration (play, next, prev) remains in library_page.go
// where the full type information is available.
type PlaybackController struct {
	pbSvc PlaybackServicer
}

// NewPlaybackController creates a new PlaybackController.
func NewPlaybackController(pbSvc PlaybackServicer) *PlaybackController {
	return &PlaybackController{
		pbSvc: pbSvc,
	}
}

// AdjustVolume changes the volume by the given percentage (e.g., 5 for +5%, -5 for -5%).
// Volume range is logarithmic: -1 = 50%, 0 = 100%, 1 = 200%.
func (pc *PlaybackController) AdjustVolume(percentDelta float64) {
	if pc.pbSvc == nil {
		return
	}

	log.Debug("PlaybackController.AdjustVolume", "delta", percentDelta)

	currentVol := pc.pbSvc.GetVolume()

	// Volume uses logarithmic scale: vol = log2(percentage / 100)
	// So: percentage = 2^vol * 100
	currentPct := math.Pow(2, currentVol) * 100
	newPct := currentPct + percentDelta

	// Clamp to reasonable range: 0% to 200%
	if newPct < 0 {
		newPct = 0
	}
	if newPct > 200 {
		newPct = 200
	}

	newVol := math.Log2(newPct / 100)
	pc.pbSvc.SetVolume(newVol)

	log.Debug("volume adjusted",
		"from_pct", int(math.Round(currentPct)),
		"to_pct", int(math.Round(newPct)))
}

// Next computes the next track selection and optionally plays it via the pbSvc
func (pc *PlaybackController) Next(queue []domain.Track, queueIndex int, tracks []domain.Track, selected int) (isQueue bool, newQueueIndex int, newSelected int, next *domain.Track) {
	// Queue-first behavior. If queue is non-empty, advance to the next
	// queued index if available. Do NOT wrap the queue. When the end
	// of the queue is reached, return newQueueIndex == -1 and next == nil.
	if len(queue) > 0 {
		idx := queueIndex
		// If there is no current queue index, start at 0
		if idx < 0 {
			idx = 0
			// We have a valid next: queue[0]
			return true, idx, selected, &queue[idx]
		}
		// Compute next index (advance by one)
		idx++
		// If we've reached the end of the queue, signal completion
		if idx >= len(queue) {
			return true, -1, selected, nil
		}
		// Normal next item in queue
		return true, idx, selected, &queue[idx]
	}

	// When no queue, navigate tracks (wrap on tracks to keep old behavior).
	if len(tracks) == 0 {
		return false, queueIndex, selected, nil
	}
	idx := selected
	if idx < 0 || idx >= len(tracks)-1 {
		idx = 0
	} else {
		idx++
	}
	// do not play here, only compute indices and return the selected track
	return false, queueIndex, idx, &tracks[idx]
}

// Prev computes the previous track selection and optionally plays it via the pbSvc
func (pc *PlaybackController) Prev(queue []domain.Track, queueIndex int, tracks []domain.Track, selected int) (isQueue bool, newQueueIndex int, newSelected int, prev *domain.Track) {
	if len(queue) > 0 {
		idx := queueIndex
		if idx <= 0 {
			idx = len(queue) - 1
		} else {
			idx--
		}
		return true, idx, selected, &queue[idx]
	}

	if len(tracks) == 0 {
		return false, queueIndex, selected, nil
	}
	idx := selected
	if idx <= 0 {
		idx = len(tracks) - 1
	} else {
		idx--
	}
	// do not play here, only compute indices and return the selected track
	return false, queueIndex, idx, &tracks[idx]
}

// PlayNext computes next selection and starts playback using the playback service.
// Returns whether queue was used, the new indices, the domain.Track played, and any error.
func (pc *PlaybackController) PlayNext(ctx context.Context, queue []domain.Track, queueIndex int, tracks []domain.Track, selected int, lib interface {
	FetchStream(ctx context.Context, t *domain.Track) (io.ReadCloser, string, error)
},
) (isQueue bool, newQueueIndex int, newSelected int, played *domain.Track, err error) {
	isQueue, newQueueIndex, newSelected, next := pc.Next(queue, queueIndex, tracks, selected)
	// If the controller indicates the queue has completed (newQueueIndex == -1),
	// there's nothing to play next.
	if next == nil {
		return isQueue, newQueueIndex, newSelected, nil, nil
	}
	// Do not call pbSvc directly; orchestration should be handled by the UI/orchestrator.
	return isQueue, newQueueIndex, newSelected, next, nil
}

// PlayPrev computes previous selection and starts playback using the playback service.
func (pc *PlaybackController) PlayPrev(ctx context.Context, queue []domain.Track, queueIndex int, tracks []domain.Track, selected int, lib interface {
	FetchStream(ctx context.Context, t *domain.Track) (io.ReadCloser, string, error)
},
) (isQueue bool, newQueueIndex int, newSelected int, played *domain.Track, err error) {
	isQueue, newQueueIndex, newSelected, prev := pc.Prev(queue, queueIndex, tracks, selected)
	if prev == nil {
		return isQueue, newQueueIndex, newSelected, nil, nil
	}
	// Do not call pbSvc directly; orchestration should be handled by the UI/orchestrator.
	return isQueue, newQueueIndex, newSelected, prev, nil
}
