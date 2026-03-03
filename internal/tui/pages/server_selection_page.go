package pages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	styles "plexmusic-tui/internal/tui/styles"
)

// ServerSelectionPage handles server selection UI
const maxServerFetchAttempts = 3

// serverItem adapts domain.PlexServer to list.Item
type serverItem domain.PlexServer

func (i serverItem) Title() string       { return i.Name }
func (i serverItem) Description() string { return fmt.Sprintf("%s:%s", i.Host, i.Port) }
func (i serverItem) FilterValue() string { return i.Name }

type ServerSelectionPage struct {
	appCtx      *app.AppContext
	authService service.AuthServicer
	configMgr   *config.Manager

	width, height int

	list                list.Model
	loadingServers      bool
	serverFetchAttempts int
	errorMsg            string

	// Event subscription
	ctx     context.Context
	cancel  context.CancelFunc
	eventCh <-chan pubsub.Event[domain.AuthEvent]

	keys tui.ServerSelectionKeyMap
}

// NewServerSelectionPage creates a new server selection page
func NewServerSelectionPage(
	appCtx *app.AppContext,
	authSvc service.AuthServicer,
	cfgMgr *config.Manager,
) *ServerSelectionPage {
	ctx, cancel := context.WithCancel(context.Background())
	// Establish subscription immediately to avoid races where the service
	// publishes events before the page has a chance to subscribe.
	eventCh := authSvc.Subscribe(ctx)

	l := list.New(nil, styles.NewCustomDelegate(), 20, 10)
	l.Title = "Select Plex Server"
	l.SetShowHelp(true)
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings() // Disable q/Q quit keys - use ctrl+c for quit

	keys := tui.ServerSelectionKeyMap{
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
	}

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return keys.ShortHelp()
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return keys.FullHelp()[0]
	}

	return &ServerSelectionPage{
		appCtx:         appCtx,
		authService:    authSvc,
		configMgr:      cfgMgr,
		ctx:            ctx,
		cancel:         cancel,
		eventCh:        eventCh,
		list:           l,
		loadingServers: true,
		keys:           keys,
	}
}

// Init initializes the server selection page
func (p *ServerSelectionPage) Init() tea.Cmd {
	// Ensure we start the subscription command as soon as possible and also
	// (in parallel) issue the fetch. The subscription itself is established
	// during construction, but we still include subscribe cmd to start
	// listening for events.
	return tea.Batch(
		p.subscribeToAuthEvents(),
		p.fetchServers(),
	)
}

// Update handles messages for the server selection page
func (p *ServerSelectionPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		p.list.SetSize(msg.Width-4, msg.Height-4)
		return p, nil

	case tea.KeyMsg:
		// Let global key handling deal with quitting; Esc here is reserved for future back/cancel behavior.
		// If loading, ignore all other input
		if p.loadingServers {
			return p, nil
		}

		if key.Matches(msg, p.keys.Select) {
			if len(p.list.Items()) > 0 {
				return p, p.selectServer()
			}
		}

	case domain.AuthEvent:
		return p, p.handleAuthEvent(msg)
	}

	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

// View renders the server selection page
func (p *ServerSelectionPage) View() tea.View {
	// If width isn't initialized yet, avoid rendering (matches login page behavior)
	if p.width == 0 {
		return tea.NewView("")
	}

	var content string

	if p.loadingServers {
		content = p.renderLoading()
	} else if p.errorMsg != "" {
		content = p.renderError()
	} else if len(p.list.Items()) == 0 {
		content = p.renderNoServers()
	} else {
		content = p.list.View()
	}

	// Center the content and add padding like the login page
	return tea.NewView(lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.NewStyle().Padding(1, 2).Render(content),
	))
}

// Close cleans up resources
func (p *ServerSelectionPage) Close() {
	if p.cancel != nil {
		p.cancel()
	}
}

// fetchServers fetches available servers
func (p *ServerSelectionPage) fetchServers() tea.Cmd {
	return func() tea.Msg {
		token := p.appCtx.Session.Token()
		if token == "" {
			return domain.AuthEvent{
				Type:  "servers.fetch_failed",
				Error: fmt.Errorf("no authentication token"),
			}
		}

		// Respect the page cancellation but also limit the request time.
		// The AuthService will publish events (servers.loaded or servers.fetch_failed).
		// Here we rely on the subscription to receive those events rather than
		// returning them directly from this command.
		reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()

		// Fire request and ignore return; service will publish event(s) via broker.
		_, _ = p.authService.FetchServers(reqCtx, token)

		return nil
	}
}

// subscribeToAuthEvents subscribes to authentication service events
func (p *ServerSelectionPage) subscribeToAuthEvents() tea.Cmd {
	// Prefer the existing subscription created in the constructor; if not
	// present, create a subscription using the page context.
	eventCh := p.eventCh
	if eventCh == nil {
		eventCh = p.authService.Subscribe(p.ctx)
		p.eventCh = eventCh
	}

	// Only forward server-related events (servers.loaded, servers.fetch_failed).
	// Ignore other auth events (auth.success/auth.failed) that are irrelevant to
	// this page and which could result in incorrectly updating state.
	return func() tea.Msg {
		for event := range eventCh {
			if event.Type == "servers.loaded" || event.Type == "servers.fetch_failed" {
				return event.Payload
			}
			// skip unrelated events and continue listening
			continue
		}
		return nil
	}
}

// handleAuthEvent processes authentication events
func (p *ServerSelectionPage) handleAuthEvent(event domain.AuthEvent) tea.Cmd {
	switch event.Type {
	case "servers.loaded":
		p.loadingServers = false
		p.errorMsg = ""
		// Reset attempts counter on success
		p.serverFetchAttempts = 0

		items := make([]list.Item, len(event.Servers))
		for i, s := range event.Servers {
			items[i] = serverItem(s)
		}
		p.list.SetItems(items)

		// Try to select previously used server and auto-select if found
		lastServer := ""
		if p.configMgr != nil {
			lastServer = p.configMgr.GetLastSelectedServer()
		}
		if lastServer != "" {
			for i, server := range event.Servers {
				// Use host/name canonical form to compare, and fall back to server name only.
				key := fmt.Sprintf("%s/%s", server.Host, server.Name)
				if key == lastServer || server.Name == lastServer {
					p.list.Select(i)
					// If matched by server name only, attempt to upgrade the stored key
					// to the canonical host/name format to avoid future ambiguity.
					if p.configMgr != nil && server.Host != "" {
						newKey := fmt.Sprintf("%s/%s", server.Host, server.Name)
						if newKey != lastServer {
							p.configMgr.SetLastSelectedServer(newKey)
							if err := p.configMgr.Save(); err != nil {
								// TODO: Add logging
							}
						}
					}
					// Immediately select and navigate to library page when the previously-used
					// server is found so the user is returned directly to the main view.
					return p.selectServer()
				}
			}
		}

	case "servers.fetch_failed":
		// If the request failed due to context cancellation or deadline exceeded,
		// attempt a retry up to maxServerFetchAttempts.
		retryable := false
		if event.Error != nil {
			if errors.Is(event.Error, context.Canceled) ||
				errors.Is(event.Error, context.DeadlineExceeded) {
				retryable = true
			}
		}

		if retryable && p.serverFetchAttempts < maxServerFetchAttempts {
			// Re-issue the fetch command to retry.
			return p.fetchServers()
		}

		// If we reach here, either it wasn't a cancellation/timeout or we've
		// exhausted retries; display the error to the user.
		p.loadingServers = false
		p.errorMsg = fmt.Sprintf("Failed to fetch servers: %v", event.Error)
	}

	return nil
}

// selectServer handles server selection
func (p *ServerSelectionPage) selectServer() tea.Cmd {
	selectedItem := p.list.SelectedItem()
	if selectedItem == nil {
		return nil
	}
	selected := domain.PlexServer(selectedItem.(serverItem))

	// Store servers and selected index in session context
	items := p.list.Items()
	domainServers := make([]domain.PlexServer, len(items))
	for i, item := range items {
		domainServers[i] = domain.PlexServer(item.(serverItem))
	}
	p.appCtx.Session.SetServers(domainServers)
	p.appCtx.Session.SetSelectedServer(p.list.Index())

	// Save to config (if config manager present)
	if p.configMgr != nil {
		// Persist as host/name to provide uniqueness across servers
		key := fmt.Sprintf("%s/%s", selected.Host, selected.Name)
		p.configMgr.SetLastSelectedServer(key)
		if err := p.configMgr.Save(); err != nil {
			// Log error but continue - non-fatal
			// TODO: Add logging
		}
	}

	// Transition to library page
	return func() tea.Msg {
		return tui.PageChangeMsg{ID: tui.LibraryPageID}
	}
}

// renderLoading renders the loading state
func (p *ServerSelectionPage) renderLoading() string {
	title := styles.TitleStyle.Render("Plex Music")
	loading := styles.FocusedStyle.Render("Loading servers...")
	help := styles.HelpStyle.Render("Press q to quit")

	return lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		loading,
		"",
		help,
	)
}

// renderError renders the error state
func (p *ServerSelectionPage) renderError() string {
	title := styles.TitleStyle.Render("Plex Music")
	errorText := styles.ErrorStyle.Render(p.errorMsg)
	help := styles.HelpStyle.Render("Press q to quit")

	return lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		errorText,
		"",
		help,
	)
}

// renderNoServers renders the no servers state
func (p *ServerSelectionPage) renderNoServers() string {
	title := styles.TitleStyle.Render("Plex Music")
	noServers := styles.ErrorStyle.Render("No Plex servers found")
	help := styles.HelpStyle.Render("Press q to quit")

	return lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		noServers,
		"",
		help,
	)
}
