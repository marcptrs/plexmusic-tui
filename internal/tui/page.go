package tui

import tea "github.com/charmbracelet/bubbletea"

// PageID identifies different pages/views in the application
type PageID string

const (
	LoginPageID           PageID = "login"
	ServerSelectionPageID PageID = "server_selection"
	MainAppPageID         PageID = "main_app"
)

// PageChangeMsg signals a page transition
type PageChangeMsg struct {
	ID PageID
}

// Page represents a full-screen page in the TUI application.
// It embeds tea.Model for standard Bubble Tea behavior.
type Page interface {
	tea.Model
	Close() // Cleanup when navigating away
}
