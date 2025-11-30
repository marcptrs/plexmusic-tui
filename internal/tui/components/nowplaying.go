package components

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/service"
	styles "plexmusic-tui/internal/tui/styles"
	"plexmusic-tui/internal/tui/util"
)

// padOrCropLines ensures the provided string is exactly `height` lines.
func padOrCropLinesComp(s string, width, height int) string {
	if height <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	blank := strings.Repeat(" ", width)
	for len(lines) < height {
		lines = append(lines, blank)
	}
	var b bytes.Buffer
	for i, l := range lines {
		b.WriteString(l)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// NowPlayingComponent handles rendering the Now Playing pane with track info,
// album art, and playback controls.
type NowPlayingComponent struct {
	coordinator app.Coordinatorer
	pbSvc       service.PlaybackServicer // Use interface instead of concrete type for flexibility
	// precompute tracking to avoid repeated Precompute calls when nothing changed
	lastPrecomputeThumb string
	lastPrecomputeW     int
	lastPrecomputeH     int
}

// NewNowPlayingComponent creates a new NowPlayingComponent.
func NewNowPlayingComponent(
	coordinator app.Coordinatorer,
	pbSvc service.PlaybackServicer,
) *NowPlayingComponent {
	return &NowPlayingComponent{
		coordinator: coordinator,
		pbSvc:       pbSvc,
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
		// Ask the renderer to precompute the exact size to prevent the
		// first-render blocking PNG encode. Debounce using the thumb URL and
		// requested size so we don't precompute repeatedly for the same content.
		thumb := np.coordinator.PlaybackAlbumArtThumb()
		if thumb != np.lastPrecomputeThumb || artW != np.lastPrecomputeW || artH != np.lastPrecomputeH {
			np.coordinator.PlaybackImgRenderer().Precompute(art, artW, artH)
			np.lastPrecomputeThumb = thumb
			np.lastPrecomputeW = artW
			np.lastPrecomputeH = artH
		}

		artView = np.coordinator.PlaybackImgRenderer().Render(art, artW, artH)
		artView = strings.TrimRight(artView, "\r\n ")
		artView = padOrCropLinesComp(artView, artW, artH)
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

	// Next track info
	var nextTrackView string
	queue := np.coordinator.Queue()
	idx := np.coordinator.QueueIndex()
	if idx+1 < len(queue) {
		next := queue[idx+1]
		nextTrackView = styles.MutedStyle.Render(fmt.Sprintf("Next: %s • %s", next.Title, next.Artist))
	}

	rightColumn := lipgloss.JoinVertical(lipgloss.Left,
		trackTitle,
		artist,
		album,
		"",
		styles.BlurredStyle.Render(progressBar),
		styles.BlurredStyle.Render(volume),
		"",
		nextTrackView,
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

// RenderInfo renders only the textual/info section of Now Playing without the
// album art. This is used when the album art is displayed separately in a
// stacked/column layout (e.g. left column art + info under it).
func (np *NowPlayingComponent) RenderInfo(width, height int) string {
	// If no track present show 'Nothing Playing' placeholder area
	if !np.coordinator.HasCurrentTrack() {
		return np.renderNothingPlaying(width, height)
	}

	tr := np.coordinator.CurrentTrack()
	trackTitle := styles.PrimaryTextStyle().Render(tr.Title)
	artist := styles.SecondaryTextStyle().Render(tr.Artist)
	album := styles.TertiaryTextStyle().Render(tr.Album)
	progressBar := np.buildProgressBar(width, 0, tr.Duration)
	volume := np.buildVolumeDisplay()

	// Next track info
	var nextTrackView string
	queue := np.coordinator.Queue()
	idx := np.coordinator.QueueIndex()
	if idx+1 < len(queue) {
		next := queue[idx+1]
		nextTrackView = styles.MutedStyle.Render(fmt.Sprintf("Next: %s • %s", next.Title, next.Artist))
	}

	// Stack text fields vertically with a small spacer and center them
	info := lipgloss.JoinVertical(lipgloss.Center,
		trackTitle,
		artist,
		album,
		"",
		styles.BlurredStyle.Render(progressBar),
		styles.BlurredStyle.Render(volume),
		"",
		nextTrackView,
	)
	// Center the info block within the provided width/height
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, info)
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

	// Prefer the playback service volume if it was provided to the component
	if np.pbSvc != nil {
		vol := np.pbSvc.GetVolume()
		return fmt.Sprintf("Volume: %.0f%%", float64(np.toPercent(vol)))
	}
	// Otherwise, check the coordinator
	// This is a limitation of the component design; alternatively, pass pbSvc in.
	// For now, just render a placeholder or nothing.
	if v := np.coordinator.Volume(); v != nil {
		return fmt.Sprintf("Volume: %.0f%%", float64(np.toPercent(v.Volume)))
	}
	volume = fmt.Sprintf("Volume: %.0f%%", 100.0)

	return volume
}

// Helper to convert a logarithmic volume into percent for display
func (np *NowPlayingComponent) toPercent(vol float64) int {
	// Volume: 0 = 100%, 1 = 200%, -1 = 50% (logarithmic base 2)
	pct := 1.0
	if vol != 0 {
		pct = pow2(vol)
	} else {
		pct = 1.0
	}
	// Use rounding to avoid off-by-one truncation errors when converting
	// from the internal logarithmic volume representation to a user-facing
	// percentage. Truncation was causing the displayed volume to occasionally
	// change by 4% instead of 5% due to floating point precision issues.
	return int(math.Round(pct * 100))
}

func pow2(v float64) float64 { return math.Pow(2, v) }

// SetVolume updates the volume display (called from library page when pbSvc available).
func (np *NowPlayingComponent) SetVolume(pbVolume float64) {
	// This is informational; the component doesn't store state.
	// The caller should manage volume state via the playback service.
	_ = pbVolume
}

// RenderFull produces a full-screen Now Playing view. Coordinates match the
// previous page-level `renderNowPlayingFull` function.
func (np *NowPlayingComponent) RenderFull(width int, height int) string {
	if !np.coordinator.HasCurrentTrack() {
		help := styles.NothingPlayingHintStyle()
		return lipgloss.JoinVertical(
			lipgloss.Center,
			styles.TitleStyle.Render("Now Playing"),
			"",
			styles.NothingPlayingStyle(),
			"",
			help,
		)
	}

	tr := np.coordinator.CurrentTrack()
	title := styles.PrimaryTextStyle().Render(tr.Title)
	artist := styles.SecondaryTextStyle().Render(tr.Artist)
	album := styles.TertiaryTextStyle().Render(tr.Album)

	art := np.coordinator.PlaybackAlbumArt()
	var artView string
	if art != nil && np.coordinator.PlaybackImgRenderer() != nil {
		// Compute a larger art size for full-screen mode
		artW := width * 60 / 100
		if artW < 20 {
			artW = 20
		}
		artH := height * 50 / 100
		if artH < 10 {
			artH = 10
		}
		// Precompute for the large/full-screen size as a background op to avoid
		// blocking the first render.
		thumb := np.coordinator.PlaybackAlbumArtThumb()
		if thumb != np.lastPrecomputeThumb || artW != np.lastPrecomputeW || artH != np.lastPrecomputeH {
			np.coordinator.PlaybackImgRenderer().Precompute(art, artW, artH)
			np.lastPrecomputeThumb = thumb
			np.lastPrecomputeW = artW
			np.lastPrecomputeH = artH
		}

		artView = np.coordinator.PlaybackImgRenderer().Render(art, artW, artH)
		artView = strings.TrimRight(artView, "\r\n ")
		artView = padOrCropLinesComp(artView, artW, artH)
	} else {
		thumb := np.coordinator.PlaybackAlbumArtThumb()
		if thumb != "" {
			artView = styles.PrimaryTextStyle().Render(fmt.Sprintf("Art: %s", thumb))
		} else {
			artView = styles.BlurredStyle.Render("(Album art)")
		}
	}

	// Playback info and controls
	posSamples := np.coordinator.StreamPosition()
	lengthSamples := np.coordinator.StreamLength()
	sr := int(np.coordinator.SampleRate())
	var posMs, lenMs int
	if sr > 0 {
		posMs = posSamples * 1000 / sr
		if lengthSamples > 0 {
			lenMs = lengthSamples * 1000 / sr
		} else {
			lenMs = tr.Duration
		}
	} else {
		posMs = 0
		lenMs = tr.Duration
	}
	posStr := util.FormatTrackDuration(posMs)
	lenStr := util.FormatTrackDuration(lenMs)

	barWidth := width - 10
	if barWidth < 12 {
		barWidth = 12
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
	progressBar := fmt.Sprintf(
		"[%s%s] %s / %s",
		styles.FocusedStyle.Render(barFill),
		styles.BlurredStyle.Render(barEmpty),
		posStr,
		lenStr,
	)

	info := lipgloss.JoinVertical(
		lipgloss.Left,
		styles.TitleStyle.Render("Now Playing"),
		"",
		title,
		artist,
		album,
		"",
		styles.BlurredStyle.Render(progressBar),
		"",
		styles.BlurredStyle.Render(
			fmt.Sprintf(
				"Volume: %s",
				styles.BlurredStyle.Render(fmt.Sprintf("%.2f", func() float64 {
					if vol := np.coordinator.Volume(); vol != nil {
						return vol.Volume
					}
					return 1.0
				}())),
			),
		),
		"",
		"",
	)

	return lipgloss.JoinHorizontal(lipgloss.Center,
		lipgloss.NewStyle().Padding(1, 2).Render(artView),
		lipgloss.NewStyle().Padding(1, 2).Render(info),
	)
}
