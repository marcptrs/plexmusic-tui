package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// QuitRequestedMsg is sent when a global quit key is detected
type QuitRequestedMsg struct{}

// Router manages page lifecycle and transitions
type Router struct {
	currentPage Page
	currentID   PageID
}

// NewRouter creates a router with an initial page
func NewRouter(initialPage Page, initialID PageID) *Router {
	return &Router{
		currentPage: initialPage,
		currentID:   initialID,
	}
}

// CurrentPageID returns the active page ID
func (r *Router) CurrentPageID() PageID {
	return r.currentID
}

// NavigateTo switches to a new page
func (r *Router) NavigateTo(newPage Page, newID PageID) tea.Cmd {
	// Close old page
	if r.currentPage != nil {
		r.currentPage.Close()
	}

	// Switch to new page
	r.currentPage = newPage
	r.currentID = newID

	// Initialize new page
	return newPage.Init()
}

// Init initializes the current page
func (r *Router) Init() tea.Cmd {
	if r.currentPage != nil {
		return r.currentPage.Init()
	}
	return nil
}

// Update delegates to the current page
func (r *Router) Update(msg tea.Msg) tea.Cmd {
	if r.currentPage == nil {
		return nil
	}

	// Page.Update returns (tea.Model, tea.Cmd)
	updatedModel, cmd := r.currentPage.Update(msg)

	// Type assert back to Page
	if page, ok := updatedModel.(Page); ok {
		r.currentPage = page
	}

	return cmd
}

// View renders the current page
func (r *Router) View() string {
	if r.currentPage == nil {
		return ""
	}
	return r.currentPage.View()
}

// Close cleans up the current page
func (r *Router) Close() {
	if r.currentPage != nil {
		r.currentPage.Close()
	}
}
