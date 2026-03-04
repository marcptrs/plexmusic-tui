package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/service"
)

// AppModel is the top-level Bubble Tea model for the TUI.
// It owns the Router, global KeyMap and application-level dependencies
// and is responsible for handling global keys (quit, etc.) before
// delegating to the Router and pages.
type AppModel struct {
	router      *Router
	appCtx      *app.AppContext
	authService service.AuthServicer
	configMgr   *config.Manager
	keyMap      KeyMap

	// pageFactory creates pages on demand to avoid import cycles between
	// the tui package and concrete page implementations.
	pageFactory func(PageID) Page
}

// NewAppModel constructs an AppModel. It will wire the provided KeyMap into
// the Router's global key handling so there's a single source of truth.
//
// pageFactory is required for creating pages when PageChangeMsg is received.
func NewAppModel(
	router *Router,
	appCtx *app.AppContext,
	authSvc service.AuthServicer,
	cfgMgr *config.Manager,
	keyMap KeyMap,
	pageFactory func(PageID) Page,
) *AppModel {
	return &AppModel{
		router:      router,
		appCtx:      appCtx,
		authService: authSvc,
		configMgr:   cfgMgr,
		keyMap:      keyMap,
		pageFactory: pageFactory,
	}
}

// Init delegates initialization to the router/page.
func (a *AppModel) Init() tea.Cmd {
	if a.router == nil {
		return nil
	}
	initCmd := a.router.Init()

	// If we already have a coordinator-provided terminal size, ensure the
	// initial page receives it immediately so pages that rely on width/height
	// don't render an empty layout while waiting for a WindowSize message.
	if a.appCtx != nil {
		sizeCmd := func() tea.Msg {
			return tea.WindowSizeMsg{
				Width:  a.appCtx.View.Width(),
				Height: a.appCtx.View.Height(),
			}
		}
		return tea.Batch(initCmd, sizeCmd)
	}
	return initCmd
}

// CurrentPageID returns the router's active page ID for external inspection
// and testing purposes.
func (a *AppModel) CurrentPageID() PageID {
	if a.router == nil {
		return PageID("")
	}
	return a.router.CurrentPageID()
}

// Update first checks for app-level/global keys. If none match it delegates
// to the Router. The method mirrors the existing behavior where a command
// returned by the router may produce an immediate QuitRequestedMsg; that
// result is inspected so that AppModel can perform coordinated shutdown.
func (a *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle key messages at the app level first to guarantee global keys
	// are honored regardless of page/input interception.
	if km, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(km, a.keyMap.Quit) {
			// Graceful shutdown: close router (which closes current page),
			// then instruct Bubble Tea to quit.
			if a.router != nil {
				a.router.Close()
			}
			return a, tea.Quit
		}
	}

	// Handle explicit global messages (in case some component returns them)
	switch msg := msg.(type) {
	case QuitRequestedMsg:
		if a.router != nil {
			a.router.Close()
		}
		return a, tea.Quit

	// Window size messages should update the coordinator state so it can be
	// used when creating or initializing pages. We don't consume the message;
	// we still forward it to the router so the active page receives it too.
	case tea.WindowSizeMsg:
		if a.appCtx != nil {
			a.appCtx.View.SetWidth(msg.Width)
			a.appCtx.View.SetHeight(msg.Height)
		}

	case PageChangeMsg:
		// Create the requested page via the factory to avoid package cycles.
		if a.pageFactory == nil {
			return a, nil
		}
		newPage := a.pageFactory(msg.ID)
		if newPage == nil {
			return a, nil
		}
		// Ensure the newly-initialized page receives the current size immediately
		// by returning a WindowSizeMsg (built from coordinator's dimensions) along
		// with the router navigation command.
		navCmd := a.router.NavigateTo(newPage, msg.ID)
		sizeCmd := func() tea.Msg {
			if a.appCtx == nil {
				return nil
			}
			return tea.WindowSizeMsg{Width: a.appCtx.View.Width(), Height: a.appCtx.View.Height()}
		}
		return a, tea.Batch(navCmd, sizeCmd)
	}

	// Delegate to the router for page-level handling
	if a.router == nil {
		return a, nil
	}
	cmd := a.router.Update(msg)

	// IMPORTANT: do NOT execute cmd() here. Commands returned by pages are
	// expected to run asynchronously by the Bubble Tea runtime. Calling them
	// synchronously in Update blocks the main loop and causes perceptible
	// input delays when commands perform blocking operations (e.g. waiting
	// on a channel in a subscription command).
	if cmd != nil {
		return a, cmd
	}

	return a, nil
}

// View delegates rendering to the router.
func (a *AppModel) View() tea.View {
	if a.router == nil {
		return tea.NewView("")
	}
	content := a.router.View()
	return tea.NewView(content)
}
