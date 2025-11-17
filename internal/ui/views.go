/*
Package ui contains view helpers and layout utilities.

Deprecated: Styling definitions are canonical in `internal/tui/styles`. The ui package should be limited to view/layout helpers
(e.g., GetContentPaneWidth, FormatTrackDuration, ViewBuilder) and not used as the source of styles.

Migration notes:
  - Replace usages of ui.* style references (e.g., ui.TitleStyle) with styles.* from
    `plexmusic-tui/internal/tui/styles` (for example: `styles "plexmusic-tui/internal/tui/styles"` then `styles.TitleStyle`).
  - Remove the compatibility wrapper `internal/ui/styles.go` once all code has migrated to `internal/tui/styles`.
  - Update AGENTS.md and README to reflect that `internal/tui/styles` is the canonical style package.

See AGENTS.md and ARCHITECTURE.md for more details on the migration plan.
*/
package ui

import (
	"fmt"
	"image"
	"strings"
	"time"

	styles "plexmusic-tui/internal/tui/styles"
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
	titleRendered := styles.TitleStyle.Render(title)
	contentRendered := styles.BlurredStyle.Render(content)
	helpRendered := styles.BlurredStyle.Render(help)
	return fmt.Sprintf("\n%s\n%s\n%s", titleRendered, contentRendered, helpRendered)
}

// RenderTitleView creates a simple title-only view
func (vb *ViewBuilder) RenderTitleView(title string) string {
	return fmt.Sprintf("\n%s\n", styles.TitleStyle.Render(title))
}

// RenderSuccessMessage creates a success message view
func (vb *ViewBuilder) RenderSuccessMessage(title, message string) string {
	titleRendered := styles.TitleStyle.Render(title)
	msgRendered := styles.SuccessStyle.Render(fmt.Sprintf("\n  %s", message))
	helpRendered := styles.BlurredStyle.Render("\n\n  Press Enter or Ctrl+C to exit\n")
	return fmt.Sprintf("\n%s\n%s%s", titleRendered, msgRendered, helpRendered)
}

// RenderErrorMessage creates an error message view
func (vb *ViewBuilder) RenderErrorMessage(title, message string) string {
	titleRendered := styles.TitleStyle.Render(title)
	errRendered := styles.ErrorStyle.Render(fmt.Sprintf("\n  %s", message))
	helpRendered := styles.BlurredStyle.Render("\n\n  Press Enter or Ctrl+C to exit\n")
	return fmt.Sprintf("\n%s\n%s%s", titleRendered, errRendered, helpRendered)
}

// RenderListItem creates a styled list item with optional selection indicator
func (vb *ViewBuilder) RenderListItem(item string, isFocused bool) string {
	if isFocused {
		cursor := styles.FocusedStyle.Render("> ")
		return fmt.Sprintf("%s%s", cursor, styles.FocusedStyle.Render(item))
	}
	return fmt.Sprintf("  %s", styles.BlurredStyle.Render(item))
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
	titleRendered := styles.TitleStyle.Render(title)
	helpRendered := styles.BlurredStyle.Render(help)
	return fmt.Sprintf("\n%s\n\n%s%s", titleRendered, content, helpRendered)
}

// RenderLoadingView creates a loading state view
func (vb *ViewBuilder) RenderLoadingView(title string) string {
	titleRendered := styles.TitleStyle.Render(title)
	return fmt.Sprintf("\n%s\n\n  Please wait...\n", titleRendered)
}
