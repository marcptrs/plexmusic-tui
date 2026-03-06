package pages

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"plexmusic-tui/internal/app"
	domain "plexmusic-tui/internal/domain"
	styles "plexmusic-tui/internal/tui/styles"
	"plexmusic-tui/internal/tui/util"
)

// View renders the library page using a full-screen background with palette colors
// on the left (album art area) and menu/queue overlay on the right.
func (p *LibraryPage) View() tea.View {
	// Ensure we have width/height for layout calculations
	if p.width == 0 {
		if p.appCtx != nil && p.appCtx.View.Width() > 0 {
			p.width = p.appCtx.View.Width()
		} else {
			p.width = 120
		}
	}
	if p.height == 0 {
		if p.appCtx != nil && p.appCtx.View.Height() > 0 {
			p.height = p.appCtx.View.Height()
		} else {
			p.height = 35
		}
	}

	// If no server is selected or the token is missing, show a helpful message
	server := (*domain.PlexServer)(nil)
	token := ""
	if p.appCtx != nil {
		server = p.appCtx.Session.GetCurrentServer()
		token = p.appCtx.Session.Token()
	}
	if server == nil || token == "" {
		title := styles.TitleStyle.Render("Plex Music")
		var msg string
		if token == "" {
			msg = "You're not signed in. Please sign in from the Login page to continue."
		} else {
			msg = "No Plex server selected. Press Esc to choose a server from the server selection screen."
		}
		content := styles.BlurredStyle.Render(msg)
		help := styles.HelpStyle.Render("Esc: Server Selection • Ctrl+C: Quit")

		return tea.NewView(lipgloss.Place(
			p.width,
			p.height,
			lipgloss.Center,
			lipgloss.Center,
			lipgloss.NewStyle().
				Padding(1, 2).
				Render(lipgloss.JoinVertical(lipgloss.Center, title, "", content, "", help)),
		))
	}

	// Calculate layout dimensions accounting for status and help lines
	contentHeight := p.height - 3
	if contentHeight < 6 {
		contentHeight = 6
	}

	// Build the right-side content (menu/queue)
	var rightContent string
	active := p.appCtx.View.ActiveTab()
	if active < app.HomeTab || active > app.SettingsTab {
		p.appCtx.View.SetActiveTab(app.HomeTab)
		active = app.HomeTab
	}

	rightWidth := (p.width / 2) - 1
	listHeight := contentHeight - 2

	switch active {
	case app.HomeTab:
		if p.showingTracks {
			rightContent = p.renderTracks(rightWidth)
		} else {
			if p.hubsLoading {
				rightContent = p.renderLoadingHubs(rightWidth, listHeight)
			} else {
				p.homeComponent.SetSize(rightWidth, listHeight)
				p.homeComponent.RefreshFromCoordinator()
				homeView := p.homeComponent.View()
				rightContent = homeView.Content
			}
		}
	case app.LibraryTab:
		if p.showingTracks {
			rightContent = p.renderTracks(rightWidth)
		} else {
			rightContent = p.renderRecentlyAdded(rightWidth)
		}
	case app.PlaylistsTab:
		if p.showingTracks {
			rightContent = p.renderTracks(rightWidth)
		} else {
			rightContent = p.renderPlaylists(rightWidth)
		}
	case app.QueueTab:
		rightContent = p.renderQueue(rightWidth)
	case app.SearchTab:
		p.searchComponent.SetSize(rightWidth, listHeight)
		searchView := p.searchComponent.View()
		rightContent = searchView.Content
	case app.SettingsTab:
		p.settingsComponent.SetSize(rightWidth, listHeight)
		settingsView := p.settingsComponent.View()
		rightContent = settingsView.Content
	default:
		if p.hubsLoading {
			rightContent = p.renderLoadingHubs(rightWidth, listHeight)
		} else {
			p.homeComponent.SetSize(rightWidth, listHeight)
			p.homeComponent.RefreshFromCoordinator()
			homeView := p.homeComponent.View()
			rightContent = homeView.Content
		}
	}

	// Render the background with colored areas and overlay
	mainContent := p.background.RenderWithOverlay(p.width, contentHeight, rightContent)

	// Build status line
	serverName := "none"
	if server != nil && server.Name != "" {
		serverName = server.Name
	}

	var statusLine string
	sonicStatus := ""
	if p.appCtx != nil {
		if p.appCtx.Content.HasSonicAvailable() {
			sonicStatus = styles.SuccessStyle.Render("Sonic: enabled")
		} else if p.appCtx.Content.HasPlexPass() {
			sonicStatus = styles.InfoStyle.Render("Sonic: no analyzed tracks (Plex Pass present)")
		} else {
			sonicStatus = styles.BlurredStyle.Render("Sonic: unavailable")
		}
	}

	if p.playbackInitializing {
		statusLine = lipgloss.JoinHorizontal(lipgloss.Left,
			styles.BlurredStyle.Render("Server: "),
			styles.BlurredStyle.Render(serverName),
			styles.BlurredStyle.Render(" • "),
			p.spinner.View(),
			styles.BlurredStyle.Render(" Starting playback..."),
		)
	} else {
		statusLine = styles.BlurredStyle.Render(lipgloss.JoinHorizontal(lipgloss.Left,
			"Server: ",
			serverName,
		))
	}

	if sonicStatus != "" {
		statusLine = lipgloss.JoinHorizontal(lipgloss.Left,
			statusLine,
			" • ",
			sonicStatus,
		)
	}

	// Build help view
	p.help.SetWidth(p.width)
	helpView := p.help.View(p.keys)

	// Compose final layout
	finalView := lipgloss.JoinVertical(lipgloss.Left,
		statusLine,
		mainContent,
		helpView,
	)

	// If full-screen now playing is focused, show that instead
	if p.focusedNowPlaying {
		return tea.NewView(lipgloss.Place(
			p.width,
			p.height,
			lipgloss.Center,
			lipgloss.Center,
			func() string {
				return p.nowPlaying.RenderFull(p.width, p.height)
			}(),
		))
	}

	return tea.NewView(lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Left,
		lipgloss.Top,
		finalView,
	))
}

// renderRecentlyAdded displays the current recently-added albums list.
func (p *LibraryPage) renderRecentlyAdded(width int) string {
	// Build a combined home view showing radio stations and recently added
	var b strings.Builder

	// Only show radio stations from hubs - keep home screen clean
	hubs := p.appCtx.Content.LibraryHubs()
	for _, hub := range hubs {
		// Only render station hubs (radio stations)
		if strings.Contains(strings.ToLower(hub.Context), "station") && len(hub.Playlists) > 0 {
			b.WriteString(styles.SectionTitleStyle.Render("Stations"))
			for _, pl := range hub.Playlists {
				prefix := "  "
				prefixStyled := lipgloss.NewStyle().Background(styles.ColorPaneBackground).Render(prefix)
				b.WriteString("\n" + prefixStyled + styles.PrimaryTextStyle().Render(pl.Title))
			}
			b.WriteString("\n\n")
			break // Only show one stations hub
		}
	}

	// Recently Added list
	b.WriteString(styles.SectionTitleStyle.Render("Recently Added"))
	b.WriteString("\n")
	// On the home screen, always limit to 5 albums for a cleaner view
	items := p.recentlyAddedComponent.Items()
	const homeLimit = 5
	selectedIdx := p.recentlyAddedComponent.Index()
	limit := homeLimit
	if len(items) < limit {
		limit = len(items)
	}
	for i := 0; i < limit; i++ {
		item := items[i]
		if albumItem, ok := item.(util.AlbumItem); ok {
			prefix := "  "
			if i == selectedIdx {
				prefix = "> "
			}
			// Use centralized title+artist renderer for consistent background and colors
			artistInfo := fmt.Sprintf("%s (%d)", albumItem.Album.Artist, albumItem.Album.Year)
			title := styles.RenderTitleArtist(albumItem.Album.Title, artistInfo, i == selectedIdx, false)
			b.WriteString(fmt.Sprintf("%s%s\n", prefix, title))
		}
	}
	if len(items) > homeLimit {
		b.WriteString(fmt.Sprintf("\n  %d more items...\n", len(items)-homeLimit))
	}
	return b.String()
}

// renderPlaylists displays the playlists list.
func (p *LibraryPage) renderPlaylists(width int) string {
	playlistView := p.playlistComponent.View()
	return playlistView.Content
}

// renderQueue displays the queued tracks list.
func (p *LibraryPage) renderQueue(width int) string {
	p.queueComponent.SetWidth(width)
	queueView := p.queueComponent.View()
	return queueView.Content
}

// renderTracks displays the currently selected tracks in the left pane.
func (p *LibraryPage) renderTracks(width int) string {
	p.trackComponent.SetWidth(width)
	trackView := p.trackComponent.View()
	return trackView.Content
}

// renderLoadingHubs renders a centered loading spinner while hubs are being fetched
func (p *LibraryPage) renderLoadingHubs(width, height int) string {
	spinnerStr := p.spinner.View()
	loadingText := "Loading..."

	content := lipgloss.JoinHorizontal(lipgloss.Left,
		spinnerStr,
		" ",
		styles.BlurredStyle.Render(loadingText),
	)

	// Center horizontally and vertically
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	view := tea.NewView(style.Render(content))
	return view.Content
}

// renderWithModal composes the base view layout with the queue modal overlay.
func (p *LibraryPage) renderWithModal(base tea.View) tea.View {
	return p.queueComponent.RenderWithModal(base, p.width, p.height)
}
