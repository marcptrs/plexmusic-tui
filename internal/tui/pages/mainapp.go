package pages

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/tui"
	"plexmusic-tui/internal/ui"
)

// MainAppPage handles the main application UI (albums, playlists, playback)
// TODO: Full implementation - this is a placeholder for Phase 3 integration
type MainAppPage struct {
	coordinator *app.Coordinator

	width, height int
}

// NewMainAppPage creates a new main application page
func NewMainAppPage(coord *app.Coordinator) *MainAppPage {
	return &MainAppPage{
		coordinator: coord,
	}
}

// Init initializes the main app page
func (p *MainAppPage) Init() tea.Cmd {
	// TODO: Fetch initial data (libraries, albums, etc.)
	return nil
}

// Update handles messages for the main app page
func (p *MainAppPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return p, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Navigate back to server selection
			return p, func() tea.Msg {
				return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
			}
		}
	}

	return p, nil
}

// View renders the main app page
func (p *MainAppPage) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00D9FF")).
		MarginBottom(1)

	helpStyle := ui.HelpStyle

	title := titleStyle.Render("Main Application")
	content := "TODO: Main app implementation (albums, playlists, playback)\n\n"
	content += "This placeholder confirms the page navigation flow works.\n"

	help := helpStyle.Render("\nCtrl+C or q: Quit • Esc: Back")

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		title,
		content,
		help,
	)
}

// Close cleans up main app page resources
func (p *MainAppPage) Close() {
	// TODO: Stop playback if active
	// TODO: Clean up event subscriptions
}
