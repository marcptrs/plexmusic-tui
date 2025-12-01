package styles

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CustomDelegate provides a list item delegate with full-width selection
// and consistent styling that matches the background-based pane system.
type CustomDelegate struct {
	ShowDescription bool
	Styles          CustomDelegateStyles
	height          int
	spacing         int
}

// PlayingIndicator provides a way for list items to indicate the "playing" state
// so the delegate can style playing items consistently without importing util.
type PlayingIndicator interface {
	IsPlaying() bool
}

// CustomDelegateStyles defines the styles for list items
type CustomDelegateStyles struct {
	NormalTitle       lipgloss.Style
	NormalDesc        lipgloss.Style
	SelectedTitle     lipgloss.Style
	SelectedDesc      lipgloss.Style
	SelectedIndicator string
	NormalIndicator   string
}

// NewCustomDelegate creates a new custom delegate with default styling
func NewCustomDelegate() CustomDelegate {
	return CustomDelegate{
		ShowDescription: true,
		height:          2,
		spacing:         0,
		Styles: CustomDelegateStyles{
			NormalTitle: lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(ColorPaneBackground).
				Padding(0, 0, 0, 1),
			NormalDesc: lipgloss.NewStyle().
				Foreground(ColorMuted).
				Background(ColorPaneBackground).
				Padding(0, 0, 0, 1),
			SelectedTitle: lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(ColorSelected).
				Bold(true).
				Padding(0, 1),
			SelectedDesc: lipgloss.NewStyle().
				Foreground(ColorSelectedText).
				Background(ColorSelected).
				Padding(0, 1),
			SelectedIndicator: "▸ ",
			NormalIndicator:   "  ",
		},
	}
}

func (d CustomDelegate) Height() int {
	if d.ShowDescription {
		return d.height
	}
	return 1
}

func (d CustomDelegate) Spacing() int {
	return d.spacing
}

func (d CustomDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d CustomDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var (
		title, desc string
		indicator   string
		s           = &d.Styles
	)

	if i, ok := item.(list.DefaultItem); ok {
		title = i.Title()
		desc = i.Description()
	} else {
		return
	}

	isSelected := index == m.Index()

	// Get the width for full-width rendering
	width := m.Width()
	if width <= 0 {
		width = 80 // fallback
	}

	if isSelected {
		indicator = s.SelectedIndicator

		// Render title with full-width background
		var titleStr string
		if strings.Contains(title, "\x1b[") {
			// Preserve any inner ANSI color codes and apply pane background and padding.
			titleStr = lipgloss.NewStyle().Background(ColorSelected).
				Width(width - lipgloss.Width(indicator)).
				Render(title)
		} else {
			titleStr = s.SelectedTitle.
				Width(width - lipgloss.Width(indicator)).
				Render(title)
		}

		if d.ShowDescription && desc != "" {
			var descStr string
			// Try to split into Artist - Album for finer-grained styling (use ASCII dash)
			parts := strings.SplitN(desc, " - ", 2)
			artistPart := parts[0]
			albumPart := ""
			if len(parts) > 1 {
				albumPart = parts[1]
			}
			// For selected rows ensure artist/album have high contrast on the selected background
			artistStyled := lipgloss.NewStyle().
				Foreground(ColorSelectedText).
				Background(ColorSelected).
				Render(artistPart)
			// Slightly dim album: use muted color for contrast (subtle but readable)
			// Make album visible on selected rows; white text ensures clarity.
			albumStyled := lipgloss.NewStyle().Foreground(ColorSelectedText).Background(ColorSelected).Render(albumPart)
			sep := lipgloss.NewStyle().Background(ColorSelected).Render(" - ")
			if artistPart == "" {
				descStr = albumStyled
			} else if albumPart == "" {
				descStr = artistStyled
			} else {
				descStr = lipgloss.JoinHorizontal(lipgloss.Left, artistStyled, sep, albumStyled)
			}
			descStr = lipgloss.NewStyle().
				Background(ColorSelected).
				Width(width - lipgloss.Width(indicator)).
				Render(descStr)
			padding := lipgloss.NewStyle().Background(ColorSelected).Width(lipgloss.Width(indicator)).Render("")
			indicator = lipgloss.NewStyle().Background(ColorSelected).Render(indicator)
			fmt.Fprintf(w, "%s%s\n%s%s", indicator, titleStr, padding, descStr)
		} else {
			fmt.Fprintf(w, "%s%s", indicator, titleStr)
		}
	} else {
		indicator = s.NormalIndicator

		var titleStr string
		// If the item reports that it is playing, show the success color; otherwise use the normal title style.
		if pi, ok := item.(PlayingIndicator); ok && pi.IsPlaying() {
			titleStr = SuccessStyle.Render(title)
		} else if strings.Contains(title, "\x1b[") {
			// Preserve inner color codes and apply minimal padding via NormalTitle.
			titleStr = lipgloss.NewStyle().Background(ColorPaneBackground).Padding(0, 0, 0, 1).Render(title)
		} else {
			titleStr = s.NormalTitle.Width(width - lipgloss.Width(indicator)).Render(title)
		}

		if d.ShowDescription && desc != "" {
			var descStr string
			// Split description into artist and album
			parts := strings.SplitN(desc, " - ", 2)
			artistPart := parts[0]
			albumPart := ""
			if len(parts) > 1 {
				albumPart = parts[1]
			}
			// Non-selected row: artist and album are same color (Secondary) and use pane background.
			artistStyled := lipgloss.NewStyle().Foreground(ColorSecondary).Background(ColorPaneBackground).Render(artistPart)
			// Slightly dim album for visual difference — use dimmed hue instead of font fainting
			albumStyled := lipgloss.NewStyle().Foreground(ColorSecondaryDim).Background(ColorPaneBackground).Render(albumPart)
			sep := lipgloss.NewStyle().Foreground(ColorSecondary).Background(ColorPaneBackground).Render(" - ")
			if artistPart == "" {
				descStr = albumStyled
			} else if albumPart == "" {
				descStr = artistStyled
			} else {
				descStr = lipgloss.JoinHorizontal(lipgloss.Left, artistStyled, sep, albumStyled)
			}
			// end: description rendering for non-selected row
			// Apply background + width separately to keep line length under linter limits
			descStr = lipgloss.NewStyle().Background(ColorPaneBackground).
				Width(width - lipgloss.Width(indicator)).
				Render(descStr)
			padding := lipgloss.NewStyle().Background(ColorPaneBackground).Width(lipgloss.Width(indicator)).Render("")
			indicator = lipgloss.NewStyle().Background(ColorPaneBackground).Render(indicator)
			fmt.Fprintf(w, "%s%s\n%s%s", indicator, titleStr, padding, descStr)
		} else {
			fmt.Fprintf(w, "%s%s", indicator, titleStr)
		}
	}
}
