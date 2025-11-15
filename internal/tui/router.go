package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// QuitRequestedMsg is sent when a global quit key is detected
type QuitRequestedMsg struct{}

// GlobalKeyMap defines keys that should be handled globally before page processing
type GlobalKeyMap struct {
	Quit key.Binding
}

// CheckGlobalKeys checks if a key message matches any global keys
func (g GlobalKeyMap) CheckGlobalKeys(msg tea.KeyMsg) tea.Msg {
	if key.Matches(msg, g.Quit) {
		return QuitRequestedMsg{}
	}
	return nil
}

// Router manages page lifecycle and transitions
type Router struct {
	currentPage Page
	currentID   PageID
	globalKeys  GlobalKeyMap
}

// NewRouter creates a router with an initial page and global key handling
func NewRouter(initialPage Page, initialID PageID, globalKeys GlobalKeyMap) *Router {
	return &Router{
		currentPage: initialPage,
		currentID:   initialID,
		globalKeys:  globalKeys,
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

// Update delegates to the current page, but checks global keys first
func (r *Router) Update(msg tea.Msg) tea.Cmd {
	if r.currentPage == nil {
		return nil
	}

	// Check for global keys FIRST, before the page can consume them
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if globalMsg := r.globalKeys.CheckGlobalKeys(keyMsg); globalMsg != nil {
			// Convert to a command that returns the global message
			return func() tea.Msg { return globalMsg }
		}
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
