package styles

import (
	"fmt"
	"io"
	"strings"

	"plexmusic-tui/internal/tui/colors"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

// NewDynamicDelegate creates a new delegate that dynamically uses current theme colors
func NewDynamicDelegate() CustomDelegate {
	return CustomDelegate{
		ShowDescription: true,
		height:          2,
		spacing:         0,
		Styles: CustomDelegateStyles{
			NormalTitle: lipgloss.NewStyle().
				Padding(0, 0, 0, 1),
			NormalDesc: lipgloss.NewStyle().
				Padding(0, 0, 0, 1),
			SelectedTitle: lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1),
			SelectedDesc: lipgloss.NewStyle().
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

		// Get current theme colors dynamically
		textColor, bgColor, _, _ := colors.GetThemeColors()

		// Render title with full-width background using theme colors
		var titleStr string
		if strings.Contains(title, "\x1b[") {
			// Preserve any inner ANSI color codes and apply theme background and padding.
			titleStr = lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Foreground(lipgloss.Color(textColor)).
				Width(width - lipgloss.Width(indicator)).
				Render(title)
		} else {
			// Apply theme colors to selected title
			themedSelectedTitle := s.SelectedTitle.
				Foreground(lipgloss.Color(textColor)).
				Background(lipgloss.Color(bgColor))
			titleStr = themedSelectedTitle.
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
				Foreground(lipgloss.Color(textColor)).
				Background(lipgloss.Color(bgColor)).
				Render(artistPart)
			// Slightly dim album: use muted color for contrast (subtle but readable)
			// Make album visible on selected rows; white text ensures clarity.
			albumStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color(textColor)).
				Background(lipgloss.Color(bgColor)).
				Render(albumPart)
			sep := lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Render(" - ")
			if artistPart == "" {
				descStr = albumStyled
			} else if albumPart == "" {
				descStr = artistStyled
			} else {
				descStr = lipgloss.JoinHorizontal(lipgloss.Left, artistStyled, sep, albumStyled)
			}
			descStr = lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Width(width - lipgloss.Width(indicator)).
				Render(descStr)
			padding := lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Width(lipgloss.Width(indicator)).
				Render("")
			indicator = lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Render(indicator)
			fmt.Fprintf(w, "%s%s\n%s%s", indicator, titleStr, padding, descStr)
		} else {
			fmt.Fprintf(w, "%s%s", indicator, titleStr)
		}
	} else {
		indicator = s.NormalIndicator

		var titleStr string
		// Get current theme colors for non-selected items
		textColor, bgColor, secondaryColor, tertiaryColor := colors.GetThemeColors()

		// If the item reports that it is playing, show the success color; otherwise use the normal title style.
		if pi, ok := item.(PlayingIndicator); ok && pi.IsPlaying() {
			titleStr = SuccessStyle.Render(title)
		} else if strings.Contains(title, "\x1b[") {
			// Preserve inner color codes and apply theme background and padding.
			titleStr = lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Foreground(lipgloss.Color(textColor)).
				Padding(0, 0, 0, 1).
				Render(title)
		} else {
			// Apply theme colors to normal title
			themedNormalTitle := s.NormalTitle.
				Foreground(lipgloss.Color(textColor)).
				Background(lipgloss.Color(bgColor))
			titleStr = themedNormalTitle.Width(width - lipgloss.Width(indicator)).Render(title)
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
			// Non-selected row: artist and album use theme colors and background.
			artistStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color(secondaryColor)).
				Background(lipgloss.Color(bgColor)).
				Render(artistPart)
			// Slightly dim album for visual difference — use tertiary color
			albumStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color(tertiaryColor)).
				Background(lipgloss.Color(bgColor)).
				Render(albumPart)
			sep := lipgloss.NewStyle().
				Foreground(lipgloss.Color(secondaryColor)).
				Background(lipgloss.Color(bgColor)).
				Render(" - ")
			if artistPart == "" {
				descStr = albumStyled
			} else if albumPart == "" {
				descStr = artistStyled
			} else {
				descStr = lipgloss.JoinHorizontal(lipgloss.Left, artistStyled, sep, albumStyled)
			}
			// Apply theme background + width
			descStr = lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Width(width - lipgloss.Width(indicator)).
				Render(descStr)
			padding := lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Width(lipgloss.Width(indicator)).
				Render("")
			indicator = lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Render(indicator)
			fmt.Fprintf(w, "%s%s\n%s%s", indicator, titleStr, padding, descStr)
		} else {
			fmt.Fprintf(w, "%s%s", indicator, titleStr)
		}
	}
}
