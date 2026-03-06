package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"plexmusic-tui/internal/app"
	imageutil "plexmusic-tui/internal/image"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui/styles"
)

// BackgroundComponent renders a full-screen background with palette-derived colors
// extracted from album art (displayed as ANSI colored blocks). Left side shows the
// colored album area, right side has an overlay menu/queue.
type BackgroundComponent struct {
	ctx   *app.AppContext
	pbSvc service.PlaybackServicer

	// Caching for palette extraction to avoid repeated computation
	lastAlbumThumb string
	lastPalette    *imageutil.Palette
}

// NewBackgroundComponent creates a new BackgroundComponent.
func NewBackgroundComponent(
	ctx *app.AppContext,
	pbSvc service.PlaybackServicer,
) *BackgroundComponent {
	return &BackgroundComponent{
		ctx:   ctx,
		pbSvc: pbSvc,
	}
}

// Render renders the full-screen background with palette colors.
// If overlayContent is provided, it will be rendered as a centered modal overlay.
func (bg *BackgroundComponent) Render(width, height int, overlayContent string) string {
	return bg.render(width, height, overlayContent, false)
}

// RenderWithOverlay renders the background with a styled overlay modal.
func (bg *BackgroundComponent) RenderWithOverlay(width, height int, overlayContent string) string {
	return bg.render(width, height, overlayContent, true)
}

// render is the internal rendering method
func (bg *BackgroundComponent) render(width, height int, overlayContent string, styled bool) string {
	// Ensure minimum dimensions
	if width < 20 || height < 10 {
		return ""
	}

	// Extract palette from current album art
	palette := bg.extractPalette()
	bgColor := bg.paletteToHexColor(palette.Primary)
	secondaryColor := bg.paletteToHexColor(palette.Secondary)

	// Create background style with primary palette color
	bgStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Width(width).
		Height(height)

	// Precompute album art rendering to avoid blocking on first render
	art := bg.ctx.Playback.AlbumArt()
	if art != nil && bg.ctx.Services.PlaybackImgRenderer() != nil {
		artWidth := width / 2
		if artWidth < 20 {
			artWidth = 20
		}
		thumb := bg.ctx.Playback.AlbumArtThumb()
		if thumb != bg.lastAlbumThumb {
			bg.ctx.Services.PlaybackImgRenderer().Precompute(art, artWidth, height)
			bg.lastAlbumThumb = thumb
		}
	}

	// If no overlay content, just return colored background
	if overlayContent == "" {
		return bgStyle.Render(strings.Repeat(" ", width*height))
	}

	// Render layout with overlay on the right side
	return bg.renderWithOverlay(width, height, overlayContent, palette, styled, bgStyle, secondaryColor)
}

// extractPalette extracts and caches the palette from the current album art
func (bg *BackgroundComponent) extractPalette() *imageutil.Palette {
	thumb := bg.ctx.Playback.AlbumArtThumb()

	// Return cached palette if album hasn't changed
	if thumb == bg.lastAlbumThumb && bg.lastPalette != nil {
		return bg.lastPalette
	}

	// Extract new palette from current art
	art := bg.ctx.Playback.AlbumArt()
	if art == nil {
		// Use a default palette when no album art is available
		bg.lastPalette = imageutil.DefaultPalette()
	} else {
		bg.lastPalette = imageutil.ExtractPalette(art)
	}

	bg.lastAlbumThumb = thumb
	return bg.lastPalette
}

// renderWithOverlay renders the background with an overlay modal on the right side
func (bg *BackgroundComponent) renderWithOverlay(
	width, height int,
	overlayContent string,
	palette *imageutil.Palette,
	styled bool,
	bgStyle lipgloss.Style,
	secondaryColor string,
) string {
	// Layout: left side gets colored background (based on primary color),
	// right side gets overlay content
	// Use 70% width for album art pane
	artWidth := width * 70 / 100
	if artWidth < 20 {
		artWidth = 20
	}

	overlayWidth := width - artWidth
	if overlayWidth < 20 {
		overlayWidth = 20
		artWidth = width - overlayWidth
	}

	// Left side: render album art (which will be displayed as ANSI color blocks)
	var leftContent string
	art := bg.ctx.Playback.AlbumArt()
	if art != nil && bg.ctx.Services.PlaybackImgRenderer() != nil {
		// Render album art - it will be converted to ANSI color blocks by the renderer
		leftContent = bg.ctx.Services.PlaybackImgRenderer().Render(art, artWidth, height)
		leftContent = strings.TrimRight(leftContent, "\r\n ")

		// Safely center the album art if it's smaller than the available space
		artLines := strings.Split(leftContent, "\n")
		artHeight := len(artLines)

		if artHeight < height {
			// Use lipgloss.Place to center the content
			leftContent = lipgloss.Place(
				artWidth,
				height,
				lipgloss.Center, // Horizontal center
				lipgloss.Center, // Vertical center
				leftContent,
			)
		}
	} else {
		// Fallback: show colored placeholder when no art available
		albumAreaLines := []string{}
		for i := 0; i < height; i++ {
			switch i {
			case height/2 - 1:
				albumAreaLines = append(albumAreaLines, strings.Repeat(" ", artWidth))
			case height / 2:
				// Center "Album Art" placeholder
				padding := (artWidth - 9) / 2
				if padding < 0 {
					padding = 0
				}
				line := strings.Repeat(" ", padding) + "Album Art" + strings.Repeat(" ", artWidth-padding-9)
				if len(line) < artWidth {
					line += strings.Repeat(" ", artWidth-len(line))
				}
				albumAreaLines = append(albumAreaLines, line)
			default:
				albumAreaLines = append(albumAreaLines, strings.Repeat(" ", artWidth))
			}
		}
		leftContent = strings.Join(albumAreaLines, "\n")
	}

	// Apply background color to left side using the primary palette color
	// This ensures the album art has a proper background
	leftStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(bg.paletteToHexColor(palette.Primary))).
		Width(artWidth).
		Height(height)
	leftSide := leftStyle.Render(leftContent)

	// Set global theme based on current album art
	bgColor := bg.paletteToHexColor(palette.Primary)
	theme := styles.CreateThemeFromColor(bgColor)
	styles.SetGlobalTheme(theme)

	// Right side: overlay content with themed styling
	overlayStyle := styles.ThemedBackgroundStyle().
		Padding(1, 1).
		Width(overlayWidth - 2).
		Height(height).
		Align(lipgloss.Left).
		AlignVertical(lipgloss.Top)

	rightContent := overlayStyle.Render(overlayContent)
	rightStyle := lipgloss.NewStyle().
		Width(overlayWidth).
		Height(height).
		AlignVertical(lipgloss.Center).
		Align(lipgloss.Left)
	rightSide := rightStyle.Render(rightContent)

	// Combine left and right sides horizontally
	combined := lipgloss.JoinHorizontal(lipgloss.Top, leftSide, rightSide)
	return combined
}

// paletteToHexColor converts a color.Color to a hex string for Lipgloss
func (bg *BackgroundComponent) paletteToHexColor(c any) string {
	if c == nil {
		return "#333333"
	}

	if color, ok := c.(interface {
		RGBA() (uint32, uint32, uint32, uint32)
	}); ok {
		r, g, b, _ := color.RGBA()
		// Convert from 16-bit to 8-bit
		r8 := uint8(r >> 8)
		g8 := uint8(g >> 8)
		b8 := uint8(b >> 8)
		return fmt.Sprintf("#%02X%02X%02X", r8, g8, b8)
	}

	return "#333333"
}

// adjustColorBrightness adjusts the brightness of a hex color by a factor
// factor > 0 makes it brighter, factor < 0 makes it darker
func adjustColorBrightness(hexColor string, factor float64) string {
	// Remove # if present
	hexColor = strings.TrimPrefix(hexColor, "#")

	// Parse hex color
	var r, g, b uint8
	if len(hexColor) == 6 {
		fmt.Sscanf(hexColor, "%02x%02x%02x", &r, &g, &b)
	} else {
		// Default to dark gray if parsing fails
		return "#2a2a2a"
	}

	// Adjust brightness
	adjust := func(c uint8) uint8 {
		// Convert to float, adjust, and clamp to 0-255
		newVal := float64(c) * (1.0 + factor)
		if newVal < 0 {
			return 0
		}
		if newVal > 255 {
			return 255
		}
		return uint8(newVal)
	}

	newR := adjust(r)
	newG := adjust(g)
	newB := adjust(b)

	return fmt.Sprintf("#%02x%02x%02x", newR, newG, newB)
}

// CurrentBackgroundColor returns the current background color being used
func (bg *BackgroundComponent) CurrentBackgroundColor() string {
	return styles.CurrentTheme().BackgroundColor
}

// CreateThemedDelegate creates a list delegate with theme-based styling
func (bg *BackgroundComponent) CreateThemedDelegate() styles.CustomDelegate {
	delegate := styles.NewCustomDelegate()

	// Apply global theme styles to the delegate
	delegate.Styles.NormalTitle = styles.ThemedPrimaryTextStyle().Padding(0, 0, 0, 1)
	delegate.Styles.NormalDesc = styles.ThemedSecondaryTextStyle().Padding(0, 0, 0, 1)
	delegate.Styles.SelectedTitle = styles.ThemedSelectedStyle().Padding(0, 0, 0, 1)
	delegate.Styles.SelectedDesc = styles.ThemedTertiaryTextStyle().
		Background(lipgloss.Color(styles.CurrentTheme().TextColor)).
		Padding(0, 0, 0, 1)

	return delegate
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// applyFadeEffectWithAmount applies a fading effect based on a fade amount (0.0 to 1.0)
func applyFadeEffectWithAmount(char string, fadeAmount float64) string {
	// fadeAmount = 0.0 (no fade), fadeAmount = 1.0 (fully faded)

	// For spaces, keep as-is
	if char == " " || char == "\t" || char == "\n" || char == "\r" {
		return " "
	}

	// Block characters with progressive fading
	if char == "█" {
		if fadeAmount < 0.3 {
			return "▓"
		} else if fadeAmount < 0.6 {
			return "▒"
		} else if fadeAmount < 0.9 {
			return "░"
		} else {
			return " "
		}
	}
	if char == "▓" {
		if fadeAmount < 0.5 {
			return "▒"
		} else if fadeAmount < 0.8 {
			return "░"
		} else {
			return " "
		}
	}
	if char == "▒" {
		if fadeAmount < 0.6 {
			return "░"
		} else {
			return " "
		}
	}
	if char == "░" {
		return " "
	}

	// Vertical lines
	if char == "│" || char == "┃" {
		if fadeAmount < 0.5 {
			return "┆"
		} else {
			return " "
		}
	}
	if char == "┆" {
		return " "
	}

	// Horizontal lines
	if char == "─" || char == "━" {
		if fadeAmount < 0.5 {
			return "┄"
		} else {
			return " "
		}
	}
	if char == "┄" || char == "┈" {
		return " "
	}

	// For other characters, fade based on amount
	if fadeAmount > 0.7 {
		return " " // Fully faded
	}
	if fadeAmount > 0.4 {
		// Partially faded - keep character but it will be on darker background
		return char
	}
	// Minimal fade - keep original
	return char
}

// SetTheme allows updating the color theme dynamically (for palette changes)
func (bg *BackgroundComponent) SetTheme(palette *imageutil.Palette) {
	if palette != nil {
		bg.lastPalette = palette
	}
}
