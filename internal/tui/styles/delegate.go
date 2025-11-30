package styles

import (
	"fmt"
	"io"

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
				Padding(0, 0, 0, 1),
			NormalDesc: lipgloss.NewStyle().
				Foreground(ColorMuted).
				Padding(0, 0, 0, 1),
			SelectedTitle: lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(ColorSelected).
				Bold(true).
				Padding(0, 1),
			SelectedDesc: lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
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
		titleStr := s.SelectedTitle.
			Width(width - lipgloss.Width(indicator)).
			Render(title)

		if d.ShowDescription && desc != "" {
			descStr := s.SelectedDesc.
				Width(width - lipgloss.Width(indicator)).
				Render(desc)
			fmt.Fprintf(w, "%s%s\n%s%s", indicator, titleStr, indicator, descStr)
		} else {
			fmt.Fprintf(w, "%s%s", indicator, titleStr)
		}
	} else {
		indicator = s.NormalIndicator

		titleStr := s.NormalTitle.Render(title)

		if d.ShowDescription && desc != "" {
			descStr := s.NormalDesc.Render(desc)
			fmt.Fprintf(w, "%s%s\n%s%s", indicator, titleStr, indicator, descStr)
		} else {
			fmt.Fprintf(w, "%s%s", indicator, titleStr)
		}
	}
}
