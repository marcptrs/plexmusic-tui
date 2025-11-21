package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
	styles "plexmusic-tui/internal/tui/styles"
	"plexmusic-tui/internal/tui/util"
)

// NowPlayingComponent handles rendering the Now Playing pane with track info,
// album art, and playback controls.
type NowPlayingComponent struct {
	coordinator *app.Coordinator
}

// NewNowPlayingComponent creates a new NowPlayingComponent.
func NewNowPlayingComponent(coordinator *app.Coordinator) *NowPlayingComponent {
	return &NowPlayingComponent{
		coordinator: coordinator,
	}
}

// Render returns the rendered view of the Now Playing pane.
func (np *NowPlayingComponent) Render(width, height int) string {
	// If no track is present, show a 'Nothing Playing' placeholder
	if !np.coordinator.HasCurrentTrack() {
		return np.renderNothingPlaying(width, height)
	}

	tr := np.coordinator.CurrentTrack()
	trackTitle := styles.PrimaryTextStyle().Render(tr.Title)
	artist := styles.SecondaryTextStyle().Render(tr.Artist)
	album := styles.TertiaryTextStyle().Render(tr.Album)

	// Render album art using the playback renderer (if available).
	art := np.coordinator.PlaybackAlbumArt()
	var artView string
	artW := 0
	if art != nil && np.coordinator.PlaybackImgRenderer() != nil {
		// Give the art roughly 45% of the detail width with a lower bound
		artW = width * 45 / 100
		if artW < 6 {
			artW = 6
		}
		artH := artW / 2
		// Guard against zero size
		if artH < 3 {
			artH = 3
		}
		artView = np.coordinator.PlaybackImgRenderer().Render(art, artW, artH)
	} else {
		// Fallback to the thumbnail URL if image rendering is not available.
		thumb := np.coordinator.PlaybackAlbumArtThumb()
		if thumb != "" {
			artView = styles.PrimaryTextStyle().Render(fmt.Sprintf("Art: %s", thumb))
		} else {
			artView = styles.BlurredStyle.Render("(Album art)")
		}
	}

	// Build progress bar and time display
	progressBar := np.buildProgressBar(width, artW, tr.Duration)

	// Volume display
	volume := np.buildVolumeDisplay()

	rightColumn := lipgloss.JoinVertical(lipgloss.Left,
		trackTitle,
		artist,
		album,
		"",
		styles.BlurredStyle.Render(progressBar),
		styles.BlurredStyle.Render(volume),
	)

	// If we have artView (likely multi-line), render art and info side-by-side.
	if artView != "" {
		return lipgloss.JoinHorizontal(lipgloss.Left,
			artView,
			lipgloss.NewStyle().Padding(0, 2).Render(rightColumn),
		)
	}

	// Fallback: render info block only
	return rightColumn
}

// renderNothingPlaying renders the placeholder when no track is playing.
func (np *NowPlayingComponent) renderNothingPlaying(width, height int) string {
	help := styles.NothingPlayingHintStyle()
	volumeStr := np.buildVolumeDisplay()
	nothingPlayingText := lipgloss.JoinVertical(lipgloss.Center,
		styles.NothingPlayingStyle(),
		"",
		help,
		"",
		styles.BlurredStyle.Render(volumeStr),
	)
	// Center both horizontally and vertically within the available space
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, nothingPlayingText)
}

// buildProgressBar constructs a progress bar string with position/duration.
func (np *NowPlayingComponent) buildProgressBar(totalWidth, artWidth, trackDuration int) string {
	// Use sample pos/length and sample rate (if available) to compute a time-based position.
	posSamples := np.coordinator.StreamPosition()
	lengthSamples := np.coordinator.StreamLength()
	sr := int(np.coordinator.SampleRate())

	var posMs, lenMs int
	if sr > 0 {
		posMs = posSamples * 1000 / sr
		if lengthSamples > 0 {
			lenMs = lengthSamples * 1000 / sr
		} else {
			lenMs = trackDuration
		}
	} else {
		// Fallback to track duration
		posMs = 0
		lenMs = trackDuration
	}

	// If lenMs still zero, fallback
	if lenMs == 0 {
		lenMs = trackDuration
	}

	posStr := util.FormatTrackDuration(posMs)
	lenStr := util.FormatTrackDuration(lenMs)

	// Build a progress bar roughly sized to available width
	availableWidth := totalWidth
	if artWidth > 0 {
		availableWidth = totalWidth - artWidth - 4 // padding
	}

	barWidth := availableWidth - 16 // approximate timestamp width
	if barWidth < 8 {
		barWidth = 8
	}

	var pct float64
	if lenMs > 0 {
		pct = float64(posMs) / float64(lenMs)
		if pct < 0 {
			pct = 0
		} else if pct > 1 {
			pct = 1
		}
	} else {
		pct = 0
	}

	filled := int(pct * float64(barWidth))
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}

	barFill := strings.Repeat("█", filled)
	barEmpty := strings.Repeat(" ", barWidth-filled)
	return fmt.Sprintf("[%s%s] %s / %s",
		styles.FocusedStyle.Render(barFill),
		styles.BlurredStyle.Render(barEmpty),
		posStr,
		lenStr,
	)
}

// buildVolumeDisplay returns a formatted volume string.
func (np *NowPlayingComponent) buildVolumeDisplay() string {
	volume := ""
	// Try to get volume from coordinator's playback service reference
	// For now, we'll show a placeholder. The caller should update this
	// if they have access to the playback service.
	// Volume: 0 = 100%, 1 = 200%, -1 = 50% (logarithmic scale with Base: 2)

	// Since we don't have pbSvc here, we'll check if coordinator has volume info
	// This is a limitation of the component design; alternatively, pass pbSvc in.
	// For now, just render a placeholder or nothing.
	volume = fmt.Sprintf("Volume: %.0f%%", 100.0) // Placeholder

	return volume
}

// SetVolume updates the volume display (called from library page when pbSvc available).
func (np *NowPlayingComponent) SetVolume(pbVolume float64) {
	// This is informational; the component doesn't store state.
	// The caller should manage volume state via the playback service.
	_ = pbVolume
}
