package pages

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
	domain "plexmusic-tui/internal/domain"
	styles "plexmusic-tui/internal/tui/styles"
	"plexmusic-tui/internal/tui/util"
)

// View renders the library page using a tabbed layout. It includes a nav pane,
// main content pane, and a detail/now-playing pane. When the queue is visible,
// a modal overlay is displayed.
func (p *LibraryPage) View() string {
	// Ensure we have width/height for layout calculations. If the page hasn't
	// received a WindowSizeMsg yet (width/height == 0) fall back to either the
	// coordinator-provided dimensions or to pragmatic defaults so we can render
	// the main UI immediately instead of returning an empty string.
	if p.width == 0 {
		if p.appCtx != nil && p.appCtx.View.Width() > 0 {
			p.width = p.appCtx.View.Width()
		} else {
			p.width = 120 // reasonable default for layout calculations
		}
	}
	if p.height == 0 {
		if p.appCtx != nil && p.appCtx.View.Height() > 0 {
			p.height = p.appCtx.View.Height()
		} else {
			p.height = 35 // reasonable default for layout calculations
		}
	}

	// If no server is selected or the token is missing, show a helpful message
	// (instead of a blank page) so the user knows why content is empty and how
	// to proceed.
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
			// No authentication token present
			msg = "You're not signed in. Please sign in from the Login page to continue."
		} else {
			// Token exists but no server selected
			msg = "No Plex server selected. Press Esc to choose a server from the server selection screen."
		}
		content := styles.BlurredStyle.Render(msg)
		help := styles.HelpStyle.Render("Esc: Server Selection • Ctrl+C: Quit")

		return lipgloss.Place(
			p.width,
			p.height,
			lipgloss.Center,
			lipgloss.Center,
			lipgloss.NewStyle().
				Padding(1, 2).
				Render(lipgloss.JoinVertical(lipgloss.Center, title, "", content, "", help)),
		)
	}

	// Split the main layout into two panes: left controlled by tabs (contentWidth)
	// and right showing the Now Playing (detailWidth). Ensure both fit into the
	// available width, adjusting when necessary.

	// Calculate widths to be 50/50 by default, but swap sides if the user has
	// configured the cover art to be on the right.
	usableWidth := p.width - 4 // padding on each side
	leftWidth := usableWidth / 2
	rightWidth := usableWidth - leftWidth

	// Ensure minimums
	if leftWidth < 30 {
		leftWidth = 30
		rightWidth = usableWidth - leftWidth
	}
	if rightWidth < 20 {
		rightWidth = 20
		leftWidth = usableWidth - rightWidth
	}

	// Calculate heights - account for status line (1) and help view (2) = 3 lines total
	// With background styling, we don't need as much vertical spacing as with borders
	contentHeight := p.height - 3
	if contentHeight < 6 {
		contentHeight = 6
	}
	listHeight := contentHeight - 2
	if listHeight < 0 {
		listHeight = 0
	}

	// Update list sizes to ensure they match the view dimensions
	// This is critical if the view dimensions were defaulted (e.g. if WindowSizeMsg was 0)
	p.recentlyAddedComponent.SetSize(leftWidth, listHeight)
	p.playlistComponent.SetSize(leftWidth, listHeight)
	p.trackComponent.SetSize(leftWidth, listHeight)
	p.queueComponent.SetSize(leftWidth, listHeight)

	active := p.appCtx.View.ActiveTab()
	// Ensure active tab is valid. If it's out of the expected range, set Home
	// as a safe default to ensure the UI renders content instead of an empty
	// fallback state.
	if active < app.HomeTab || active > app.SettingsTab {
		p.appCtx.View.SetActiveTab(app.HomeTab)
		active = app.HomeTab
	}

	// Build left-hand content based on active tab.
	var leftContent string
	switch active {
	case app.HomeTab:
		if p.showingTracks {
			leftContent = p.renderTracks(leftWidth)
		} else {
			// Use scrollable home component for home tab
			if p.hubsLoading {
				leftContent = p.renderLoadingHubs(leftWidth, listHeight)
			} else {
				p.homeComponent.SetSize(leftWidth, listHeight)
				p.homeComponent.RefreshFromCoordinator()
				leftContent = p.homeComponent.View()
			}
		}
	case app.LibraryTab:
		if p.showingTracks {
			leftContent = p.renderTracks(leftWidth)
		} else {
			leftContent = p.renderRecentlyAdded(leftWidth)
		}
	case app.PlaylistsTab:
		if p.showingTracks {
			leftContent = p.renderTracks(leftWidth)
		} else {
			leftContent = p.renderPlaylists(leftWidth)
		}
	case app.QueueTab:
		leftContent = p.renderQueue(leftWidth)
	case app.SearchTab:
		p.searchComponent.SetSize(leftWidth, listHeight)
		leftContent = p.searchComponent.View()
	case app.SettingsTab:
		p.settingsComponent.SetSize(leftWidth, listHeight)
		leftContent = p.settingsComponent.View()
	default:
		leftContent = p.renderRecentlyAdded(leftWidth)
	}

	// Main content — split the UI across left/right panes. If the user has
	// configured the cover art position, swap the roles accordingly.
	// We'll render cover art at the top of the left pane followed by Now Playing
	// info, while the right pane will display the queue list.
	// The page's content is composed from left/right panes.
	// Adjust height to accommodate help view at the bottom (approx 2 lines)
	// contentHeight calculated above

	// Decide on left/right roles based on configured position early so we
	// can determine if drawerOpen should affect the left or right pane.
	pos := "left"
	if p.appCtx != nil && p.appCtx.Services.ConfigManager() != nil {
		pos = p.appCtx.Services.ConfigManager().GetCoverArtPosition()
	}

	// Build right-side content - Home by default when queue is empty, otherwise queue.
	p.queueComponent.SetSize(rightWidth, listHeight)
	var rightPaneContent string
	if len(p.appCtx.Content.Queue()) == 0 {
		// Show home content when queue is empty
		if p.hubsLoading {
			rightPaneContent = p.renderLoadingHubs(rightWidth, listHeight)
		} else {
			p.homeComponent.SetSize(rightWidth, listHeight)
			p.homeComponent.RefreshFromCoordinator()
			rightPaneContent = p.homeComponent.View()
		}
	} else {
		rightPaneContent = p.renderQueue(rightWidth)
	}

	// Calculate the art size and info view so they can be used regardless of
	// drawer state (we render art and info separately when the drawer is on
	// the right side or when art is configured on the right).
	// Account for pane styling overhead to maintain consistent layout proportions.
	availableHeight := contentHeight - 2
	if availableHeight < 12 {
		// Ensure enough space for minimum art (6) and info (6) if possible,
		// otherwise we'll just overflow slightly which is better than crashing.
		availableHeight = 12
	}

	artHeight := leftWidth / 2
	// Reserve space for info (min 6)
	if artHeight > availableHeight-6 {
		artHeight = availableHeight - 6
	}
	if artHeight < 6 {
		artHeight = 6
	}
	infoHeight := availableHeight - artHeight
	// Ensure min infoHeight
	if infoHeight < 6 {
		infoHeight = 6
	}

	// Render art if available, otherwise show the logo as fallback.
	// Use lipgloss.Place to center content within the area.
	artView := ""
	if p.appCtx.Playback.AlbumArt() != nil && p.appCtx.Services.PlaybackImgRenderer() != nil {
		art := p.appCtx.Services.PlaybackImgRenderer().
			Render(p.appCtx.Playback.AlbumArt(), leftWidth, artHeight)
		art = strings.TrimRight(art, "\r\n ")
		artView = lipgloss.Place(leftWidth, artHeight, lipgloss.Center, lipgloss.Center, art)
	} else {
		// Show logo when no album art is available with shimmer effect
		logo := strings.TrimSpace(retroLogoVertical)
		lines := strings.Split(logo, "\n")

		// Create a shimmer effect by cycling through colors
		// Use multiple shades of blue and cyan to create a wave effect
		colors := []lipgloss.Color{
			lipgloss.Color("33"), // Dark blue
			lipgloss.Color("39"), // Blue (primary)
			lipgloss.Color("45"), // Bright cyan
			lipgloss.Color("51"), // Cyan
			lipgloss.Color("87"), // Light cyan
			lipgloss.Color("51"), // Cyan
			lipgloss.Color("45"), // Bright cyan
			lipgloss.Color("39"), // Blue (primary)
		}

		// Apply shimmer colors to each line based on offset
		for i, line := range lines {
			colorIndex := (i + p.logoShimmerOffset) % len(colors)
			shimmerStyle := lipgloss.NewStyle().
				Foreground(colors[colorIndex]).
				Bold(true)
			lines[i] = shimmerStyle.Render(line)
		}
		styledLogo := strings.Join(lines, "\n")
		// Center the shimmering logo
		artView = lipgloss.Place(leftWidth, artHeight, lipgloss.Center, lipgloss.Center, styledLogo)
	}

	// Render info via the component method
	infoView := p.nowPlaying.RenderInfo(leftWidth, infoHeight)

	// If either the tracklist is showing, or the drawer is open on the left,
	// render the left pane as the list content; otherwise render the cover
	// art stacked with the now playing info.
	leftContentHeight := contentHeight
	var leftPane string
	// The left pane shows the list when the art is on the right AND either a
	// tracklist is active or a drawer is open. Otherwise, the left pane
	// renders the cover art + info.
	if pos == "right" && (p.showingTracks || p.drawerOpen) {
		// When art is on the right, render the left list content (keeps layout stable).
		leftPane = styles.PaneStyle(leftWidth, leftContentHeight).
			Render(leftContent)
	} else {
		// Stacked artwork + info - reuse artView and infoView computed above
		leftPane = styles.PaneStyle(leftWidth, leftContentHeight).
			Render(lipgloss.JoinVertical(lipgloss.Center, artView, infoView))
	}

	// pos already set earlier

	var leftColumn, rightColumn string
	if pos == "right" {
		// Swap roles: left contains the Queue or content (if showingTracks/drawerOpen),
		// right contains the cover art + info.
		if p.drawerOpen || p.showingTracks {
			leftColumn = styles.PaneStyle(leftWidth, leftContentHeight).
				Render(leftContent)
		} else {
			leftColumn = styles.PaneStyle(leftWidth, leftContentHeight).Render(rightPaneContent)
		}
		rightColumn = styles.PaneStyle(rightWidth, contentHeight).
			Render(lipgloss.JoinVertical(lipgloss.Center, artView, infoView))
	} else {
		// Default: cover art left, home/queue right
		leftColumn = leftPane
		// When a drawer is open or the tracklist is active (and art is left),
		// render the active content in the right pane; otherwise show home/queue.
		if p.drawerOpen || p.showingTracks {
			rightColumn = styles.PaneStyle(rightWidth, contentHeight).Render(leftContent)
		} else {
			rightColumn = styles.PaneStyle(rightWidth, contentHeight).Render(rightPaneContent)
		}
	}

	// Compose left and right panes.
	panesRow := lipgloss.JoinHorizontal(lipgloss.Left, leftColumn, rightColumn)
	layout := panesRow

	// If Queue modal is visible, overlay it
	if p.appCtx.View.ShowQueueModal() {
		return p.renderWithModal(layout)
	}

	// If Now Playing is focused, show it full screen.
	if p.focusedNowPlaying {
		// Leave the rest of the layout behind — a full-screen now-playing UI is focused.
		return lipgloss.Place(
			p.width,
			p.height,
			lipgloss.Center,
			lipgloss.Center,
			func() string {
				return p.nowPlaying.RenderFull(p.width, p.height)
			}(),
		)
	}

	// Drawers removed; no overlay logic.

	server = p.appCtx.Session.GetCurrentServer()
	// Build status line with server/content counts.
	serverName := "none"
	if server != nil && server.Name != "" {
		serverName = server.Name
	}
	var statusLine string
	// Compose sonic capability indicator to help users understand whether
	// server supports sonic analysis and whether any analyzed tracks were detected
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
	// Append sonic status if available
	if sonicStatus != "" {
		statusLine = lipgloss.JoinHorizontal(lipgloss.Left,
			statusLine,
			" • ",
			sonicStatus,
		)
	}

	// Build a transient notification message; we'll render it as a floating
	// toast overlay instead of reserving a top layout line.
	notifStr := ""
	notifSeverity := ""
	if p.appCtx != nil && p.appCtx.View.NotificationActive() {
		msg, sev, _ := p.appCtx.View.Notification()
		notifSeverity = sev
		switch sev {
		case "error":
			notifStr = styles.ErrorStyle.Render(fmt.Sprintf(" ⚠ %s", msg))
		case "success":
			notifStr = styles.SuccessStyle.Render(fmt.Sprintf(" ✓ %s", msg))
		default:
			notifStr = styles.InfoStyle.Render(fmt.Sprintf(" %s", msg))
			notifSeverity = sev
		}
	}

	p.help.Width = p.width
	helpView := p.help.View(p.keys)

	// We use p.height-2 for the centered layout to leave room for the help view?
	// Actually centeredLayout uses lipgloss.Place which might fill the height.
	// We should probably let the layout flow naturally or adjust the Place height.
	// Since we adjusted the pane heights, the content should fit.
	// Let's just join them vertically.

	// Note: lipgloss.Place with p.height might push the help view off screen if we are not careful.
	// But we reduced the pane height by 8 (header + footer + help).
	// Header is ~3 lines (Title + Status + Notif).
	// Help is ~1-2 lines.
	// So p.height - 8 should be safe.

	// We don't strictly need lipgloss.Place for the whole page if we are building it vertically.
	// But let's keep the structure similar.

	mainLayout := lipgloss.JoinVertical(lipgloss.Left,
		statusLine,
		layout, // The panes
	)

	// Place the main layout in the available space
	finalView := lipgloss.JoinVertical(lipgloss.Left,
		mainLayout,
		helpView,
	)

	// If a transient notification is active, overlay it on top-right of the
	// final view using our util helper. We attempt to render a small boxed
	// toast to avoid using up the reserved top row in the layout.
	if notifStr != "" {
		// Constrain the toast width to ~1/3 of the viewport or the message width
		toastMaxWidth := p.width / 3
		if toastMaxWidth < 20 {
			toastMaxWidth = 20
		}
		// Compose the toast box using the ToastBoxStyle with an inner
		// severity-specific text style. We wrap the notification text in a
		// small label like "Info:" or icons to increase contrast.
		var inner string
		inner = notifStr
		switch notifSeverity {
		case "error":
			inner = styles.ToastErrorStyle.Render(inner)
		case "success":
			inner = styles.ToastSuccessStyle.Render(inner)
		default:
			inner = styles.ToastInfoStyle.Render(inner)
		}
		// Trim surrounding whitespace/newlines so we don't leave padded
		// spaces that would be rendered with the background color.
		toast := strings.TrimSpace(inner)

		// Overlay the toast on the same line as the status line (top offset 0)
		// and keep it right-aligned with minimal right padding so it does
		// not interfere with the border. This places the toast inline with
		// server info instead of reserving an extra top area.
		finalView = util.OverlayTopRight(finalView, toast, p.width, 0, 2)
	}

	// Place the final view at the top-left so content starts at the first
	// terminal row and we don't leave an empty line at the top due to
	// vertical centering.
	return lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Left,
		lipgloss.Top,
		finalView,
	)
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
	return p.playlistComponent.View()
}

// renderQueue displays the queued tracks list.
func (p *LibraryPage) renderQueue(width int) string {
	p.queueComponent.SetWidth(width)
	return p.queueComponent.View()
}

// renderTracks displays the currently selected tracks in the left pane.
func (p *LibraryPage) renderTracks(width int) string {
	p.trackComponent.SetWidth(width)
	return p.trackComponent.View()
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

	return style.Render(content)
}

// renderWithModal composes the base view layout with the queue modal overlay.
func (p *LibraryPage) renderWithModal(base string) string {
	return p.queueComponent.RenderWithModal(base, p.width, p.height)
}
