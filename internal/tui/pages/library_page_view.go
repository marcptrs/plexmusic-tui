package pages

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
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
		if p.coordinator != nil && p.coordinator.Width() > 0 {
			p.width = p.coordinator.Width()
		} else {
			p.width = 120 // reasonable default for layout calculations
		}
	}
	if p.height == 0 {
		if p.coordinator != nil && p.coordinator.Height() > 0 {
			p.height = p.coordinator.Height()
		} else {
			p.height = 35 // reasonable default for layout calculations
		}
	}

	// If no server is selected or the token is missing, show a helpful message
	// (instead of a blank page) so the user knows why content is empty and how
	// to proceed.
	server := (*app.PlexServer)(nil)
	token := ""
	if p.coordinator != nil {
		server = p.coordinator.GetCurrentServer()
		token = p.coordinator.GetToken()
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

	// Calculate heights
	contentHeight := p.height - 8
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

	active := p.coordinator.ActiveTab()
	// Ensure active tab is valid. If it's out of the expected range, set Home
	// as a safe default to ensure the UI renders content instead of an empty
	// fallback state.
	if active < app.HomeTab || active > app.SettingsTab {
		p.coordinator.SetActiveTab(app.HomeTab)
		active = app.HomeTab
	}

	// Build left-hand content based on active tab.
	var leftContent string
	switch active {
	case app.HomeTab, app.LibraryTab:
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
	if p.coordinator != nil && p.coordinator.ConfigManager() != nil {
		pos = p.coordinator.ConfigManager().GetCoverArtPosition()
	}

	// Build right-side content - Queue by default, or drawer content when open.
	p.queueComponent.SetSize(rightWidth, listHeight)
	var queueContent string
	if len(p.coordinator.Queue()) == 0 {
		// Show retro logo if queue is empty
		// Use vertical logo if space is tight (approx 80 chars needed for full logo)
		logoStr := retroLogo
		if rightWidth < 80 {
			logoStr = retroLogoVertical
		}
		logo := styles.PrimaryTextStyle().Render(logoStr)
		queueContent = lipgloss.Place(
			rightWidth,
			listHeight,
			lipgloss.Center,
			lipgloss.Center,
			logo,
		)
	} else {
		queueContent = p.renderQueue(rightWidth)
	}

	// Calculate the art size and info view so they can be used regardless of
	// drawer state (we render art and info separately when the drawer is on
	// the right side or when art is configured on the right).
	// The pane has a border (2 lines), so available height is contentHeight - 2.
	availableHeight := contentHeight - 2
	if availableHeight < 12 {
		// Ensure enough space for minimum art (6) and info (6) if possible,
		// otherwise we'll just overflow slightly which is better than crashing.
		availableHeight = 12
	}

	artHeight := leftWidth
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

	// Render art if available, otherwise show fallback. Then center it within the
	// left pane's art area so the layout appears balanced.
	artView := ""
	if p.coordinator.PlaybackAlbumArt() != nil && p.coordinator.PlaybackImgRenderer() != nil {
		artView = p.coordinator.PlaybackImgRenderer().
			Render(p.coordinator.PlaybackAlbumArt(), leftWidth, artHeight)
		artView = strings.TrimRight(artView, "\r\n ")
	} else {
		if p.coordinator.PlaybackAlbumArtThumb() != "" {
			artView = styles.PrimaryTextStyle().Render(fmt.Sprintf("Art: %s", p.coordinator.PlaybackAlbumArtThumb()))
		} else {
			artView = ""
		}
	}
	// Normalize the artView to have exactly artHeight lines, then center it.
	artView = padOrCropLines(artView, leftWidth, artHeight)
	// Center the art view horizontally and keep it at the top of the art area
	artView = lipgloss.Place(leftWidth, artHeight, lipgloss.Center, lipgloss.Top, artView)

	// Render info via the component method and ensure it centers in the reserved area
	infoView := p.nowPlaying.RenderInfo(leftWidth, infoHeight)
	infoView = padOrCropLines(infoView, leftWidth, infoHeight)

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
			Height(leftContentHeight).
			Render(leftContent)
	} else {
		// Stacked artwork + info
		// Choose an art height that is roughly square but doesn't consume all
		// the available vertical space; reserve at least 6 lines for the info.
		// The pane has a border (2 lines), so available height is contentHeight - 2.
		availableHeight := contentHeight - 2
		if availableHeight < 12 {
			availableHeight = 12
		}

		artHeight := leftWidth
		if artHeight > availableHeight-6 {
			artHeight = availableHeight - 6
		}
		if artHeight < 6 {
			artHeight = 6
		}
		infoHeight := availableHeight - artHeight
		if infoHeight < 6 {
			infoHeight = 6
		}

		// Render art if available, otherwise show fallback
		artView := ""
		if p.coordinator.PlaybackAlbumArt() != nil && p.coordinator.PlaybackImgRenderer() != nil {
			artView = p.coordinator.PlaybackImgRenderer().Render(p.coordinator.PlaybackAlbumArt(), leftWidth, artHeight)
			artView = strings.TrimRight(artView, "\r\n ")
		} else {
			if p.coordinator.PlaybackAlbumArtThumb() != "" {
				artView = styles.PrimaryTextStyle().Render(fmt.Sprintf("Art: %s", p.coordinator.PlaybackAlbumArtThumb()))
			} else {
				artView = ""
			}
		}

		// Normalize the artView to have exactly artHeight lines, then center it.
		artView = padOrCropLines(artView, leftWidth, artHeight)
		// Center the art view horizontally and keep it at the top of the art area
		artView = lipgloss.Place(leftWidth, artHeight, lipgloss.Center, lipgloss.Top, artView)

		// Render info via the component method
		infoView := p.nowPlaying.RenderInfo(leftWidth, infoHeight)
		infoView = padOrCropLines(infoView, leftWidth, infoHeight)

		leftPane = styles.PaneStyle(leftWidth, leftContentHeight).
			Height(leftContentHeight).
			Render(lipgloss.JoinVertical(lipgloss.Center, artView, infoView))
	}

	// pos already set earlier

	var leftColumn, rightColumn string
	if pos == "right" {
		// Swap roles: left contains the Queue or content (if showingTracks/drawerOpen),
		// right contains the cover art + info.
		if p.drawerOpen || p.showingTracks {
			leftColumn = styles.PaneStyle(leftWidth, leftContentHeight).
				Height(leftContentHeight).
				Render(leftContent)
		} else {
			leftColumn = styles.PaneStyle(leftWidth, leftContentHeight).Height(leftContentHeight).Render(queueContent)
		}
		rightColumn = styles.PaneStyle(rightWidth, contentHeight).
			Height(contentHeight).
			Render(lipgloss.JoinVertical(lipgloss.Center, artView, infoView))
	} else {
		// Default: cover art left, queue right
		leftColumn = leftPane
		// When a drawer is open or the tracklist is active (and art is left),
		// render the active content in the right pane; otherwise show the queue.
		if p.drawerOpen || p.showingTracks {
			rightColumn = styles.PaneStyle(rightWidth, contentHeight).Height(contentHeight).Render(leftContent)
		} else {
			rightColumn = styles.PaneStyle(rightWidth, contentHeight).Height(contentHeight).Render(queueContent)
		}
	}

	// Compose left and right panes.
	panesRow := lipgloss.JoinHorizontal(lipgloss.Left, leftColumn, rightColumn)
	layout := panesRow

	// If Queue modal is visible, overlay it
	if p.coordinator.ShowQueueModal() {
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

	server = p.coordinator.GetCurrentServer()
	// Build status line with server/content counts.
	serverName := "none"
	if server != nil && server.Name != "" {
		serverName = server.Name
	}
	albumsCount := 0
	artistsCount := 0
	playlistsCount := 0
	tracksCount := 0
	if p.coordinator != nil {
		albumsCount = p.coordinator.AlbumsTotal()
		artistsCount = p.coordinator.ArtistsTotal()
		playlistsCount = p.coordinator.PlaylistsTotal()
		tracksCount = p.coordinator.TracksTotal()
	}

	var statusLine string
	if p.loadingStats {
		statusLine = styles.BlurredStyle.Render(
			fmt.Sprintf("Server: %s • %s Loading stats...", serverName, p.spinner.View()),
		)
	} else {
		statusLine = styles.BlurredStyle.Render(
			fmt.Sprintf("Server: %s • Artists: %s • Albums: %s • Playlists: %s • Tracks: %s",
				serverName,
				util.FormatNumber(artistsCount),
				util.FormatNumber(albumsCount),
				util.FormatNumber(playlistsCount),
				util.FormatNumber(tracksCount)))
	}

	// Render a transient top notification line if set on the coordinator.
	notifStr := ""
	if p.coordinator != nil && p.coordinator.NotificationActive() {
		msg, sev, _ := p.coordinator.Notification()
		switch sev {
		case "error":
			notifStr = styles.ErrorStyle.Render(fmt.Sprintf(" ⚠ %s", msg))
		case "success":
			notifStr = styles.SuccessStyle.Render(fmt.Sprintf(" ✓ %s", msg))
		default:
			notifStr = styles.InfoStyle.Render(fmt.Sprintf(" %s", msg))
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

	if notifStr != "" {
		mainLayout = lipgloss.JoinVertical(lipgloss.Left,
			notifStr,
			statusLine,
			layout,
		)
	}

	// Place the main layout in the available space
	finalView := lipgloss.JoinVertical(lipgloss.Left,
		mainLayout,
		helpView,
	)

	return lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Center,
		lipgloss.Center,
		finalView,
	)
}

// padOrCropLines ensures a string has exactly `height` lines, trimming or
// padding lines to stabilize block height and prevent layout jumps.
func padOrCropLines(s string, width, height int) string {
	if height <= 0 {
		return ""
	}
	// Normalize CRLF endings to LF
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	// Trim trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	// Pad with blank lines (each a space to maintain width in some renderers)
	blank := strings.Repeat(" ", width)
	for len(lines) < height {
		lines = append(lines, blank)
	}
	var b bytes.Buffer
	for i, l := range lines {
		b.WriteString(l)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderRecentlyAdded displays the current recently-added albums list.
func (p *LibraryPage) renderRecentlyAdded(width int) string {
	return p.recentlyAddedComponent.View()
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

// renderWithModal composes the base view layout with the queue modal overlay.
func (p *LibraryPage) renderWithModal(base string) string {
	return p.queueComponent.RenderWithModal(base, p.width, p.height)
}
