package tui

import tea "charm.land/bubbletea/v2"

// PageID identifies different pages/views in the application
type PageID string

const (
	LoginPageID           PageID = "login"
	ServerSelectionPageID PageID = "server_selection"
	LibraryPageID         PageID = "library"
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

// PageFactoryFn is a callback function that creates a Page for a given PageID
type PageFactoryFn func(PageID) Page
