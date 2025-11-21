package service

import (
	"math"

	log "github.com/charmbracelet/log/v2"
)

// PlaybackController provides simple playback volume control operations.
// It's intentionally minimal to avoid circular imports with the app package.
// Complex playback orchestration (play, next, prev) remains in library_page.go
// where the full type information is available.
type PlaybackController struct {
	pbSvc *PlaybackService
}

// NewPlaybackController creates a new PlaybackController.
func NewPlaybackController(pbSvc *PlaybackService) *PlaybackController {
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

	log.Debug("volume adjusted", "from_pct", int(currentPct), "to_pct", int(newPct))
}
