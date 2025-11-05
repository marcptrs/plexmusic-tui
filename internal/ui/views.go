package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/disintegration/imaging"
)

// ViewRenderingModel provides rendering capabilities for model views.
// This interface defines the contract for models that can be rendered.
type ViewRenderingModel interface {
	// GetWidth returns the terminal width
	GetWidth() int
	// GetHeight returns the terminal height
	GetHeight() int
	// GetNavPaneWidth returns the navigation pane width
	GetNavPaneWidth() int
	// GetContentPaneWidth returns the content pane width
	GetContentPaneWidth() int
	// GetDetailPaneWidth returns the detail pane width
	GetDetailPaneWidth() int
	// GetFocusedPane returns the currently focused pane
	GetFocusedPane() int
	// GetCurrentAlbumArt returns the current album art image
	GetCurrentAlbumArt() image.Image
	// GetCurrentAlbumArtThumb returns the thumbnail URL of current album art
	GetCurrentAlbumArtThumb() string
	// GetPlaybackAlbumArt returns the playback album art image
	GetPlaybackAlbumArt() image.Image
	// GetPlaybackAlbumArtThumb returns the thumbnail URL of playback album art
	GetPlaybackAlbumArtThumb() string
	// GetCurrentTrackThumb returns the current track's thumbnail URL
	GetCurrentTrackThumb() string
	// SetCurrentAlbumArt sets the current album art image and thumb
	SetCurrentAlbumArt(img image.Image, thumb string)
	// SetPlaybackAlbumArt sets the playback album art image and thumb
	SetPlaybackAlbumArt(img image.Image, thumb string)
}

// RenderPlaceholder creates a simple text-based placeholder using box drawing characters
func RenderPlaceholder(width, height int, message string) string {
	// Create a simple text-based placeholder using box drawing characters
	var output strings.Builder

	// Draw top border
	output.WriteString("╔" + strings.Repeat("═", width-2) + "╗\n")

	// Calculate center position for message
	centerY := height / 2
	messageLen := len(message)
	messagePadding := (width - 2 - messageLen) / 2
	if messagePadding < 0 {
		messagePadding = 0
		message = message[:width-4] // Truncate if too long
	}

	// Draw middle rows
	for y := 1; y < height-1; y++ {
		output.WriteString("║")
		if y == centerY {
			// Center the message
			output.WriteString(strings.Repeat(" ", messagePadding))
			output.WriteString(message)
			output.WriteString(strings.Repeat(" ", width-2-messagePadding-len(message)))
		} else if y == centerY-2 {
			// Music note symbol
			symbol := "♫"
			symbolPadding := (width - 2 - len(symbol)) / 2
			output.WriteString(strings.Repeat(" ", symbolPadding))
			output.WriteString(symbol)
			output.WriteString(strings.Repeat(" ", width-2-symbolPadding-len(symbol)))
		} else {
			output.WriteString(strings.Repeat(" ", width-2))
		}
		output.WriteString("║\n")
	}

	// Draw bottom border
	output.WriteString("╚" + strings.Repeat("═", width-2) + "╝")

	return output.String()
}

// RenderImageKitty renders an image using the Kitty graphics protocol
func RenderImageKitty(img image.Image, width, height int) string {
	// Resize image to desired pixel dimensions
	// Terminal characters are roughly 1:2 (width:height) in aspect ratio
	// For a square image: if we have 40 width chars and 20 height chars,
	// the character area is already square (20 * 2 = 40 in visual terms)
	// But we need more pixels width-wise to account for actual rendering
	// Using 20 pixels per character width, 20 pixels per character height
	pixelWidth := width * 20
	pixelHeight := height * 20
	resized := imaging.Fit(img, pixelWidth, pixelHeight, imaging.Lanczos)

	// Encode image to PNG in memory
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return "" // Fall back to empty on error
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Kitty graphics protocol escape sequence
	// Format: \x1b_Ga=T,f=100,t=d,m=1;<base64 data>\x1b\\
	// a=T: transmit and display
	// f=100: PNG format
	// t=d: direct transmission
	// m=1: single chunk
	var output strings.Builder

	// Split base64 data into chunks (Kitty has a 4096 byte limit per chunk)
	chunkSize := 4096
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]

		if i == 0 {
			// First chunk with metadata
			output.WriteString("\x1b_Ga=T,f=100,t=d,m=1;")
			output.WriteString(chunk)
			output.WriteString("\x1b\\")
		} else {
			// Continuation chunks
			output.WriteString("\x1b_Gm=1;")
			output.WriteString(chunk)
			output.WriteString("\x1b\\")
		}
	}

	output.WriteString("\n")
	return output.String()
}

// RenderImageITerm2 renders an image using iTerm2's inline image protocol
func RenderImageITerm2(img image.Image, width, height int) string {
	// Resize image - using 20 pixels per character for both dimensions
	pixelWidth := width * 20
	pixelHeight := height * 20
	resized := imaging.Fit(img, pixelWidth, pixelHeight, imaging.Lanczos)

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return ""
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	// iTerm2 inline image format
	// \x1b]1337;File=inline=1;width=Npx;height=Npx:<base64 data>\a
	bounds := resized.Bounds()
	return fmt.Sprintf("\x1b]1337;File=inline=1;width=%dpx;height=%dpx:%s\a\n",
		bounds.Dx(), bounds.Dy(), encoded)
}

// RenderImageSixel renders an image using the Sixel protocol
func RenderImageSixel(img image.Image, width, height int) string {
	// Resize image - using 20 pixels per character for both dimensions
	pixelWidth := width * 20
	pixelHeight := height * 20
	resized := imaging.Fit(img, pixelWidth, pixelHeight, imaging.Lanczos)

	// Note: Full Sixel encoding is complex. For a complete implementation,
	// you would need a proper Sixel encoder library.
	// This is a placeholder that falls back to Unicode blocks
	// A proper implementation would use a library like github.com/mattn/go-sixel
	return RenderImageUnicodeBlocks(resized, width, height)
}

// RenderImageUnicodeBlocks renders an image using Unicode half-block characters (fallback)
func RenderImageUnicodeBlocks(img image.Image, width, height int) string {
	if img == nil {
		return ""
	}

	// Resize image to fit terminal dimensions
	// Use half-block characters (▀) which are 2 vertical pixels per character
	// width and height are already in a 2:1 ratio (e.g., 80x40) for square display
	resized := imaging.Fit(img, width, height*2, imaging.Lanczos)

	var output strings.Builder
	bounds := resized.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// Get colors for top and bottom pixels
			topColor := resized.At(x, y)
			r1, g1, b1, _ := topColor.RGBA()

			// Get bottom pixel color
			bottomColor := topColor // Default to top if no bottom pixel
			if y+1 < bounds.Max.Y {
				bottomColor = resized.At(x, y+1)
			}
			r2, g2, b2, _ := bottomColor.RGBA()

			// Convert from 16-bit color to 8-bit
			r1, g1, b1 = r1>>8, g1>>8, b1>>8
			r2, g2, b2 = r2>>8, g2>>8, b2>>8

			// Use upper half block with top color as foreground and bottom color as background
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r1, g1, b1))).
				Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r2, g2, b2)))

			output.WriteString(style.Render("▀"))
		}
		output.WriteString("\n")
	}

	return output.String()
}

// GetNavPaneWidth calculates the navigation pane width
func GetNavPaneWidth(totalWidth int) int {
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	navWidth := usableWidth * 20 / 100
	if navWidth < 20 {
		navWidth = 20
	}
	return navWidth
}

// GetContentPaneWidth calculates the content pane width
func GetContentPaneWidth(totalWidth int) int {
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	contentWidth := usableWidth * 30 / 100
	if contentWidth < 30 {
		contentWidth = 30
	}
	return contentWidth
}

// GetDetailPaneWidth calculates the detail pane width
func GetDetailPaneWidth(totalWidth int) int {
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	detailWidth := usableWidth * 40 / 100
	if detailWidth < 40 {
		detailWidth = 40
	}
	return detailWidth
}

// Helper types for playback state and panes (moved from main.go)
// These are used by view rendering functions

// FormatTrackDuration formats a track duration in milliseconds to MM:SS format
func FormatTrackDuration(durationMs int) string {
	durationMin := durationMs / 60000 // Convert ms to minutes
	durationSec := (durationMs % 60000) / 1000
	return fmt.Sprintf("%d:%02d", durationMin, durationSec)
}

// FormatTimeDuration formats a time.Duration to MM:SS format
func FormatTimeDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// ViewBuilder provides helper functions for constructing common view layouts
type ViewBuilder struct{}

// NewViewBuilder creates a new ViewBuilder
func NewViewBuilder() *ViewBuilder {
	return &ViewBuilder{}
}

// RenderMessageView creates a simple message view with title, content, and help text
func (vb *ViewBuilder) RenderMessageView(title, content, help string) string {
	titleRendered := TitleStyle.Render(title)
	contentRendered := BlurredStyle.Render(content)
	helpRendered := BlurredStyle.Render(help)
	return fmt.Sprintf("\n%s\n%s\n%s", titleRendered, contentRendered, helpRendered)
}

// RenderTitleView creates a simple title-only view
func (vb *ViewBuilder) RenderTitleView(title string) string {
	return fmt.Sprintf("\n%s\n", TitleStyle.Render(title))
}

// RenderSuccessMessage creates a success message view
func (vb *ViewBuilder) RenderSuccessMessage(title, message string) string {
	titleRendered := TitleStyle.Render(title)
	msgRendered := SuccessStyle.Render(fmt.Sprintf("\n  %s", message))
	helpRendered := BlurredStyle.Render("\n\n  Press Enter or Ctrl+C to exit\n")
	return fmt.Sprintf("\n%s\n%s%s", titleRendered, msgRendered, helpRendered)
}

// RenderErrorMessage creates an error message view
func (vb *ViewBuilder) RenderErrorMessage(title, message string) string {
	titleRendered := TitleStyle.Render(title)
	errRendered := ErrorStyle.Render(fmt.Sprintf("\n  %s", message))
	helpRendered := BlurredStyle.Render("\n\n  Press Enter or Ctrl+C to exit\n")
	return fmt.Sprintf("\n%s\n%s%s", titleRendered, errRendered, helpRendered)
}

// RenderListItem creates a styled list item with optional selection indicator
func (vb *ViewBuilder) RenderListItem(item string, isFocused bool) string {
	if isFocused {
		cursor := FocusedStyle.Render("> ")
		return fmt.Sprintf("%s%s", cursor, FocusedStyle.Render(item))
	}
	return fmt.Sprintf("  %s", BlurredStyle.Render(item))
}

// RenderList creates a formatted list with selection indicators
func (vb *ViewBuilder) RenderList(items []string, selectedIndex int) string {
	var result strings.Builder
	for i, item := range items {
		result.WriteString(vb.RenderListItem(item, i == selectedIndex))
		result.WriteString("\n")
	}
	return result.String()
}

// RenderFrame creates a frame with title, content, and help text
func (vb *ViewBuilder) RenderFrame(title, content, help string) string {
	titleRendered := TitleStyle.Render(title)
	helpRendered := BlurredStyle.Render(help)
	return fmt.Sprintf("\n%s\n\n%s%s", titleRendered, content, helpRendered)
}

// RenderLoadingView creates a loading state view
func (vb *ViewBuilder) RenderLoadingView(title string) string {
	titleRendered := TitleStyle.Render(title)
	return fmt.Sprintf("\n%s\n\n  Please wait...\n", titleRendered)
}
