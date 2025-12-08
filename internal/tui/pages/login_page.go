package pages

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log/v2"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	styles "plexmusic-tui/internal/tui/styles"
)

// LoginPage handles authentication UI
type LoginPage struct {
	appCtx      *app.AppContext
	authService service.AuthServicer
	configMgr   *config.Manager

	width, height int

	usernameInput textinput.Model
	passwordInput textinput.Model
	focusIndex    int

	authenticating bool
	errorMsg       string

	// Event subscription
	ctx    context.Context
	cancel context.CancelFunc

	help help.Model
	keys tui.LoginKeyMap
}

// NewLoginPage creates a new login page
func NewLoginPage(appCtx *app.AppContext, authSvc service.AuthServicer) *LoginPage {
	// Backward-compatible wrapper, creating login page without config manager
	return NewLoginPageWithConfig(appCtx, authSvc, nil)
}

func NewLoginPageWithConfig(
	appCtx *app.AppContext,
	authSvc service.AuthServicer,
	cfgMgr *config.Manager,
) *LoginPage {
	usernameInput := textinput.New()
	usernameInput.Placeholder = "Email or username"
	usernameInput.Focus()
	usernameInput.CharLimit = 100
	usernameInput.Width = 40

	passwordInput := textinput.New()
	passwordInput.Placeholder = "Password"
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.EchoCharacter = '•'
	passwordInput.CharLimit = 100
	passwordInput.Width = 40

	ctx, cancel := context.WithCancel(context.Background())

	return &LoginPage{
		appCtx:        appCtx,
		authService:   authSvc,
		configMgr:     cfgMgr,
		usernameInput: usernameInput,
		passwordInput: passwordInput,
		focusIndex:    0,
		ctx:           ctx,
		cancel:        cancel,
		help:          help.New(),
		keys:          tui.DefaultLoginKeyMap(),
	}
}

// Init initializes the login page
func (p *LoginPage) Init() tea.Cmd {
	// If there's already a token stored in the config, restore it and navigate to the next page.
	if p.configMgr != nil {
		if token := p.configMgr.GetAuthToken(); token != "" {
			p.appCtx.Session.SetToken(token)

			// If there's a previously selected server, proactively fetch servers
			// and wait for the auth service to publish 'servers.loaded' so we can
			// auto-select and navigate straight into the library page view.
			if p.configMgr.GetLastSelectedServer() != "" {
				return tea.Batch(
					textinput.Blink,
					p.subscribeToAuthEvents(),
					p.fetchServers(),
				)
			}

			// Navigate directly to server selection when no previous server is present
			return tea.Batch(
				textinput.Blink,
				p.subscribeToAuthEvents(),
				func() tea.Msg {
					return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
				},
			)
		}
	}

	return tea.Batch(
		textinput.Blink,
		p.subscribeToAuthEvents(),
	)
}

// Update handles messages for the login page
func (p *LoginPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height

	case tea.KeyMsg:
		// If authenticating, ignore all input
		if p.authenticating {
			return p, nil
		}

		switch {
		case key.Matches(msg, p.keys.Up):
			p.focusIndex--
			if p.focusIndex < 0 {
				p.focusIndex = 2
			}
			for i := range 2 {
				if i == p.focusIndex {
					cmds = append(cmds, p.getInput(i).Focus())
				} else {
					p.getInput(i).Blur()
				}
			}
			return p, tea.Batch(cmds...)

		case key.Matches(msg, p.keys.Down):
			p.focusIndex++
			if p.focusIndex > 2 {
				p.focusIndex = 0
			}
			for i := range 2 {
				if i == p.focusIndex {
					cmds = append(cmds, p.getInput(i).Focus())
				} else {
					p.getInput(i).Blur()
				}
			}
			return p, tea.Batch(cmds...)

		case key.Matches(msg, p.keys.Enter):
			if p.focusIndex == 2 || (p.focusIndex == 1 && p.passwordInput.Value() != "") {
				// Submit form
				return p, p.authenticate()
			} else {
				// Move to next field
				p.focusIndex++
				if p.focusIndex > 2 {
					p.focusIndex = 0
				}
				for i := range 2 {
					if i == p.focusIndex {
						cmds = append(cmds, p.getInput(i).Focus())
					} else {
						p.getInput(i).Blur()
					}
				}
				return p, tea.Batch(cmds...)
			}
		}

	case domain.AuthEvent:
		return p.handleAuthEvent(msg)
	}

	// Update text inputs
	if p.focusIndex < 2 && !p.authenticating {
		var cmd tea.Cmd
		if p.focusIndex == 0 {
			p.usernameInput, cmd = p.usernameInput.Update(msg)
		} else {
			p.passwordInput, cmd = p.passwordInput.Update(msg)
		}
		cmds = append(cmds, cmd)
	}

	return p, tea.Batch(cmds...)
}

// View renders the login page
func (p *LoginPage) View() string {
	if p.width == 0 {
		return ""
	}

	var s string
	s += styles.TitleStyle.Render("🎵 Plex Music TUI") + "\n\n"

	if p.authenticating {
		s += styles.SuccessStyle.Render("Authenticating...") + "\n\n"
	} else if p.errorMsg != "" {
		s += styles.ErrorStyle.Render("❌ "+p.errorMsg) + "\n\n"
	} else {
		s += "Please sign in to your Plex account\n\n"
	}

	// Username input
	if p.focusIndex == 0 {
		s += styles.FocusedStyle.Render(p.usernameInput.View())
	} else {
		s += styles.BlurredStyle.Render(p.usernameInput.View())
	}
	s += "\n\n"

	// Password input
	if p.focusIndex == 1 {
		s += styles.FocusedStyle.Render(p.passwordInput.View())
	} else {
		s += styles.BlurredStyle.Render(p.passwordInput.View())
	}
	s += "\n\n"

	// Submit button
	submitButton := "[ Sign In ]"
	if p.focusIndex == 2 {
		s += styles.ButtonStyle.Render(submitButton)
	} else {
		s += styles.ButtonBlurredStyle.Render(submitButton)
	}
	s += "\n\n"

	if !p.authenticating {
		s += p.help.View(p.keys)
	}

	return lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.NewStyle().Padding(1, 2).Render(s),
	)
}

// getInput returns the text input at the given index
func (p *LoginPage) getInput(i int) *textinput.Model {
	switch i {
	case 0:
		return &p.usernameInput
	case 1:
		return &p.passwordInput
	default:
		return nil
	}
}

// authenticate performs authentication
func (p *LoginPage) authenticate() tea.Cmd {
	username := p.usernameInput.Value()
	password := p.passwordInput.Value()

	if username == "" || password == "" {
		return func() tea.Msg {
			return domain.AuthEvent{
				Type:  "auth.failed",
				Error: fmt.Errorf("username and password are required"),
			}
		}
	}

	p.authenticating = true
	p.errorMsg = ""

	return func() tea.Msg {
		token, err := p.authService.AuthenticateUser(p.ctx, username, password)
		if err != nil {
			// Ignore context cancellation errors (happens during shutdown)
			if p.ctx.Err() != nil {
				return nil
			}
			return domain.AuthEvent{
				Type:  "auth.failed",
				Error: err,
			}
		}
		return domain.AuthEvent{
			Type:  "auth.success",
			Token: token,
		}
	}
}

// subscribeToAuthEvents creates a command that waits for the next auth event
func (p *LoginPage) subscribeToAuthEvents() tea.Cmd {
	eventCh := p.authService.Subscribe(p.ctx)
	// Return a command that waits for an event without blocking Init
	return func() tea.Msg {
		// This will block in background until event arrives
		// But since it runs AFTER Init completes, it won't hang startup
		select {
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			return event.Payload
		case <-p.ctx.Done():
			return nil
		}
	}
}

// fetchServers triggers a server list fetch via the AuthService to populate
// available servers for the current authenticated user.
func (p *LoginPage) fetchServers() tea.Cmd {
	return func() tea.Msg {
		token := p.appCtx.Session.Token()
		if token == "" {
			return nil
		}

		reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()

		// Fire request; AuthService publishes servers.loaded or servers.fetch_failed.
		_, _ = p.authService.FetchServers(reqCtx, token)
		return nil
	}
}

// handleAuthEvent processes authentication events
func (p *LoginPage) handleAuthEvent(event domain.AuthEvent) (tea.Model, tea.Cmd) {
	switch event.Type {
	case "auth.success":
		p.authenticating = false
		p.errorMsg = ""
		p.appCtx.Session.SetToken(event.Token)

		// Persist token to config if available
		if p.configMgr != nil {
			p.configMgr.SetAuthToken(event.Token)
			if err := p.configMgr.Save(); err != nil {
				log.Warn("failed to save config", "error", err)
			}
		}

		// If we have a previously selected server saved, fetch servers and wait
		// for the 'servers.loaded' event. Otherwise, navigate to server selection.
		if p.configMgr != nil && p.configMgr.GetLastSelectedServer() != "" {
			return p, tea.Batch(p.subscribeToAuthEvents(), p.fetchServers())
		}

		// Transition to server selection
		return p, func() tea.Msg {
			return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
		}

	case "auth.failed":
		p.authenticating = false
		p.errorMsg = event.Error.Error()
		return p, nil

	case "servers.loaded":
		// Store servers in session context
		p.appCtx.Session.SetServers(event.Servers)

		// Try to auto-select the previously used server. Accept both canonical
		// host/name and the legacy server name-only format. When a legacy
		// name-only match is found, update the stored key to host/name
		// when possible to avoid future ambiguity.
		lastServer := ""
		if p.configMgr != nil {
			lastServer = p.configMgr.GetLastSelectedServer()
		}
		if lastServer != "" {
			for i, s := range event.Servers {
				key := fmt.Sprintf("%s/%s", s.Host, s.Name)
				if key == lastServer || s.Name == lastServer {
					p.appCtx.Session.SetSelectedServer(i)
					// If matched by server name only, attempt to upgrade the stored
					// key to the canonical host/name format to avoid future ambiguity.
					if p.configMgr != nil && s.Host != "" && s.Name == lastServer {
						newKey := fmt.Sprintf("%s/%s", s.Host, s.Name)
						if newKey != lastServer {
							p.configMgr.SetLastSelectedServer(newKey)
							if err := p.configMgr.Save(); err != nil {
								log.Warn("failed to save updated lastSelectedServer", "error", err)
							}
						}
					}
					// Navigate straight to the library page page
					return p, func() tea.Msg {
						return tui.PageChangeMsg{ID: tui.LibraryPageID}
					}
				}
			}
		}

		// If we didn't auto-select, fall back to server selection
		return p, func() tea.Msg {
			return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
		}

	case "servers.fetch_failed":
		// Fall back to server selection if we couldn't retrieve server list
		p.errorMsg = fmt.Sprintf("Failed to fetch servers: %v", event.Error)
		return p, func() tea.Msg {
			return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
		}
	}

	return p, nil
}

// Close cleans up resources
func (p *LoginPage) Close() {
	p.cancel()
}
