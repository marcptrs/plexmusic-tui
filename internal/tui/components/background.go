package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"plexmusic-tui/internal/app"
	imageutil "plexmusic-tui/internal/image"
	"plexmusic-tui/internal/service"
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

	// Create a multi-color blended background for the left side
	leftSide := bg.createMultiColorBackground(artWidth, height, leftContent, palette)

	// Right side: overlay content with secondary color background
	overlayStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(secondaryColor)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(1, 1).
		Width(overlayWidth - 2).
		Height(height - 2).
		Align(lipgloss.Left).
		AlignVertical(lipgloss.Top)

	if styled {
		overlayStyle = overlayStyle.Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFFFFF"))
	}

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

// createMultiColorBackground creates a background with blended colors from the palette
func (bg *BackgroundComponent) createMultiColorBackground(
	width, height int, content string, palette *imageutil.Palette,
) string {
	if palette == nil {
		// Fallback to single color if no palette available
		style := lipgloss.NewStyle().
			Background(lipgloss.Color("#333333")).
			Width(width).
			Height(height)
		return style.Render(content)
	}

	// Get multiple colors from the palette
	colors := []string{
		bg.paletteToHexColor(palette.Primary),
		bg.paletteToHexColor(palette.Secondary),
		bg.paletteToHexColor(palette.Accent),
		bg.paletteToHexColor(palette.Muted),
	}

	// Filter out empty/invalid colors
	validColors := []string{}
	for _, color := range colors {
		if color != "" && color != "#000000" {
			validColors = append(validColors, color)
		}
	}

	// If we don't have enough colors, fallback to single color
	if len(validColors) < 2 {
		style := lipgloss.NewStyle().
			Background(lipgloss.Color(bg.paletteToHexColor(palette.Primary))).
			Width(width).
			Height(height)
		return style.Render(content)
	}

	// Create a multi-color background by using different colors for different rows
	var result strings.Builder
	contentLines := strings.Split(content, "\n")

	for i := 0; i < height; i++ {
		// Select color based on row position to create a gradient effect
		colorIndex := i % len(validColors)
		color := validColors[colorIndex]

		contentLine := ""
		if i < len(contentLines) {
			contentLine = contentLines[i]
		}

		// Create a line with the selected background color
		lineStyle := lipgloss.NewStyle().
			Background(lipgloss.Color(color)).
			Width(width)

		// Ensure content line is exactly the width we want
		if len(contentLine) < width {
			contentLine += strings.Repeat(" ", width-len(contentLine))
		} else if len(contentLine) > width {
			contentLine = contentLine[:width]
		}

		result.WriteString(lineStyle.Render(contentLine) + "\n")
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// SetTheme allows updating the color theme dynamically (for palette changes)
func (bg *BackgroundComponent) SetTheme(palette *imageutil.Palette) {
	if palette != nil {
		bg.lastPalette = palette
	}
}
