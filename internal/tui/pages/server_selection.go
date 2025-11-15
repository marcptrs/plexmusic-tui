package pages

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log/v2"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	"plexmusic-tui/internal/ui"
)

// ServerSelectionPage handles server selection UI
const maxServerFetchAttempts = 3

type ServerSelectionPage struct {
	coordinator *app.Coordinator
	authService service.AuthServicer
	configMgr   *config.Manager

	width, height int

	servers             []domain.PlexServer
	selectedIndex       int
	loadingServers      bool
	serverFetchAttempts int
	errorMsg            string

	// Event subscription
	ctx     context.Context
	cancel  context.CancelFunc
	eventCh <-chan pubsub.Event[service.AuthEvent]
}

// NewServerSelectionPage creates a new server selection page
func NewServerSelectionPage(
	coord *app.Coordinator,
	authSvc service.AuthServicer,
	cfgMgr *config.Manager,
) *ServerSelectionPage {
	ctx, cancel := context.WithCancel(context.Background())
	// Establish subscription immediately to avoid races where the service
	// publishes events before the page has a chance to subscribe.
	eventCh := authSvc.Subscribe(ctx)

	return &ServerSelectionPage{
		coordinator:    coord,
		authService:    authSvc,
		configMgr:      cfgMgr,
		ctx:            ctx,
		cancel:         cancel,
		eventCh:        eventCh,
		loadingServers: true,
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
		return p, nil

	case tea.KeyMsg:
		// Let global key handling deal with quitting; Esc here is reserved for future back/cancel behavior.
		// If loading, ignore all other input
		if p.loadingServers {
			return p, nil
		}

		switch msg.String() {

		case "up", "k":
			if p.selectedIndex > 0 {
				p.selectedIndex--
			}

		case "down", "j":
			if p.selectedIndex < len(p.servers)-1 {
				p.selectedIndex++
			}

		case "enter", " ":
			if len(p.servers) > 0 {
				return p, p.selectServer()
			}
		}

		return p, nil

	case service.AuthEvent:
		return p, p.handleAuthEvent(msg)

	default:
		return p, nil
	}
}

// View renders the server selection page
func (p *ServerSelectionPage) View() string {
	var content string

	if p.loadingServers {
		content = p.renderLoading()
	} else if p.errorMsg != "" {
		content = p.renderError()
	} else if len(p.servers) == 0 {
		content = p.renderNoServers()
	} else {
		content = p.renderServerList()
	}

	// Center the content
	return lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
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
		token := p.coordinator.GetToken()
		if token == "" {
			return service.AuthEvent{
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
func (p *ServerSelectionPage) handleAuthEvent(event service.AuthEvent) tea.Cmd {
	switch event.Type {
	case "servers.loaded":
		p.loadingServers = false
		p.servers = event.Servers
		p.errorMsg = ""
		// Reset attempts counter on success
		p.serverFetchAttempts = 0

		// Try to select previously used server and auto-select if found
		lastServer := ""
		if p.configMgr != nil {
			lastServer = p.configMgr.GetLastSelectedServer()
		}
		if lastServer != "" {
			for i, server := range p.servers {
				// Use host/name canonical form to compare, and fall back to server name only.
				key := fmt.Sprintf("%s/%s", server.Host, server.Name)
				if key == lastServer || server.Name == lastServer {
					p.selectedIndex = i
					// If matched by server name only, attempt to upgrade the stored key
					// to the canonical host/name format to avoid future ambiguity.
					if p.configMgr != nil && server.Host != "" {
						newKey := fmt.Sprintf("%s/%s", server.Host, server.Name)
						if newKey != lastServer {
							p.configMgr.SetLastSelectedServer(newKey)
							if err := p.configMgr.Save(); err != nil {
								log.Warn("failed to save updated lastSelectedServer", "error", err)
							}
						}
					}
					// Immediately select and navigate to main app when the previously-used
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
			if errors.Is(event.Error, context.Canceled) || errors.Is(event.Error, context.DeadlineExceeded) {
				retryable = true
			}
		}

		if retryable && p.serverFetchAttempts < maxServerFetchAttempts {
			// Debug: fetch failed; log via charm log rather than writing to a file
			log.Debug("ServerSelectionPage: fetch failed; retrying", "attempt", p.serverFetchAttempts, "error", event.Error)
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
	if p.selectedIndex < 0 || p.selectedIndex >= len(p.servers) {
		return nil
	}

	selected := p.servers[p.selectedIndex]

	// Store servers and selected index in coordinator
	// Convert domain.PlexServer to app.PlexServer
	appServers := make([]app.PlexServer, len(p.servers))
	for i, s := range p.servers {
		appServers[i] = app.PlexServer{
			Name:         s.Name,
			Host:         s.Host,
			Port:         s.Port,
			AccessToken:  s.AccessToken,
			LocalAddress: s.LocalAddress,
			Scheme:       s.Scheme,
		}
	}
	p.coordinator.SetServers(appServers)
	p.coordinator.SetSelectedServer(p.selectedIndex)

	// Save to config (if config manager present)
	if p.configMgr != nil {
		// Persist as host/name to provide uniqueness across servers
		key := fmt.Sprintf("%s/%s", selected.Host, selected.Name)
		p.configMgr.SetLastSelectedServer(key)
		if err := p.configMgr.Save(); err != nil {
			// Log error but continue - non-fatal
			log.Warn("failed to save config", "error", err)
		}
	}

	// Transition to main app
	return func() tea.Msg {
		return tui.PageChangeMsg{ID: tui.MainAppPageID}
	}
}

// renderLoading renders the loading state
func (p *ServerSelectionPage) renderLoading() string {
	title := ui.TitleStyle.Render("Plex Music")
	loading := ui.FocusedStyle.Render("Loading servers...")
	help := ui.HelpStyle.Render("Press q to quit")

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
	title := ui.TitleStyle.Render("Plex Music")
	errorText := ui.ErrorStyle.Render(p.errorMsg)
	help := ui.HelpStyle.Render("Press q to quit")

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
	title := ui.TitleStyle.Render("Plex Music")
	noServers := ui.ErrorStyle.Render("No Plex servers found")
	help := ui.HelpStyle.Render("Press q to quit")

	return lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		noServers,
		"",
		help,
	)
}

// renderServerList renders the server selection list
func (p *ServerSelectionPage) renderServerList() string {
	title := ui.TitleStyle.Render("Select Plex Server")

	// Build server list
	var serverLines []string
	for i, server := range p.servers {
		prefix := "  "
		style := lipgloss.NewStyle()

		if i == p.selectedIndex {
			prefix = "> "
			style = ui.FocusedStyle
		}

		// Show last selected indicator (safe when no config manager provided)
		lastServer := ""
		if p.configMgr != nil {
			lastServer = p.configMgr.GetLastSelectedServer()
		}
		lastIndicator := ""
		// Build canonical key for this server
		key := fmt.Sprintf("%s/%s", server.Host, server.Name)
		if key == lastServer {
			lastIndicator = " (last used)"
		}

		serverLine := fmt.Sprintf("%s%s%s", prefix, server.Name, lastIndicator)
		serverLines = append(serverLines, style.Render(serverLine))
	}

	serverList := lipgloss.JoinVertical(lipgloss.Left, serverLines...)

	help := ui.HelpStyle.Render("↑/↓: navigate • enter: select • q: quit")

	return lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		serverList,
		"",
		help,
	)
}
