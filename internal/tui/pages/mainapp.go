package pages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	styles "plexmusic-tui/internal/tui/styles"
	views "plexmusic-tui/internal/ui"
)

// MainAppPage handles the main application UI with tab navigation,
// list rendering (Recently Added, Playlists), modal dialogs, and a simple
// "Now Playing" panel (cover art + controls).
type MainAppPage struct {
	coordinator *app.Coordinator

	width, height int

	// Services and subscriptions
	ctx       context.Context
	cancel    context.CancelFunc
	authSvc   service.AuthServicer
	authEvtCh <-chan pubsub.Event[service.AuthEvent]
	libSvc    *service.LibraryServiceWithEvents
	libEvtCh  <-chan pubsub.Event[service.LibraryEvent]
	pbSvc     *service.PlaybackService
	pbEvtCh   <-chan pubsub.Event[service.PlaybackEvent]

	// Lists
	recentlyAddedList list.Model
	playlistList      list.Model
	trackList         list.Model
	queueList         list.Model

	// Search and inline content UI
	searchInput  textinput.Model
	searchActive bool
	searchTerm   string

	// showingTracks indicates the left-pane is showing track list for a selected album/playlist.
	showingTracks bool

	focusedNowPlaying bool
	// Track last selected indices to detect selection changes and fetch tracks lazily
	lastSelectedAlbumIndex    int
	lastSelectedPlaylistIndex int

	help help.Model
	keys tui.MainAppKeyMap
}

// NewMainAppPage creates a new main application page and sets up a
// cancellable background context for event subscriptions.
func NewMainAppPage(coord *app.Coordinator) *MainAppPage {
	return NewMainAppPageWithAuth(coord, nil)
}

func NewMainAppPageWithAuth(coord *app.Coordinator, authSvc service.AuthServicer) *MainAppPage {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize lists
	delegate := list.NewDefaultDelegate()

	raList := list.New(nil, delegate, 0, 0)
	raList.Title = "Recently Added"
	raList.SetShowHelp(false)

	plList := list.New(nil, delegate, 0, 0)
	plList.Title = "Playlists"
	plList.SetShowHelp(false)

	trList := list.New(nil, delegate, 0, 0)
	trList.Title = "Tracks"
	trList.SetShowHelp(false)

	qList := list.New(nil, delegate, 0, 0)
	qList.Title = "Queue"
	qList.SetShowHelp(false)

	p := &MainAppPage{
		coordinator:   coord,
		ctx:           ctx,
		cancel:        cancel,
		authSvc:       authSvc,
		searchActive:  false,
		searchTerm:    "",
		showingTracks: false,
		// Track last selected indices so we can fetch tracks lazily when selection changes
		// without issuing repeated fetches.
		lastSelectedAlbumIndex:    -1,
		lastSelectedPlaylistIndex: -1,
		help:                      help.New(),
		keys:                      tui.DefaultMainAppKeyMap(),
		recentlyAddedList:         raList,
		playlistList:              plList,
		trackList:                 trList,
		queueList:                 qList,
	}

	// Initialize search input (unfocused by default)
	ti := textinput.New()
	ti.Placeholder = "Search albums, artists, playlists"
	ti.CharLimit = 120
	ti.Width = 48
	ti.Blur()
	p.searchInput = ti

	if authSvc != nil {
		p.authEvtCh = authSvc.Subscribe(p.ctx)
	}
	return p
}

// modalForActiveTab maps a TabType -> ModalType so that switching to a
// modal-associated tab can result in opening the appropriate modal.

// tabForModal maps a modal to the tab type (used for highlighting the nav
// row while the modal is active but without switching page content).
// Note: modal/tabForModal removed; tabs now control left-pane rendering directly.

// drawerTickMsg is an internal message used to animate the drawer slide.

// drawerTickCmd returns a command that sends a drawerTickMsg on a short interval.
// drawerTickMsg/drawerTickCmd removed; drawers are deprecated in favor of
// inline left-pane rendering managed by `showingTracks`.

// Init initializes the main app page. This attempts to set up library and
// playback services for the current server (if present) and kick off the
// initial fetches for Recently Added + Playlists.
func (p *MainAppPage) Init() tea.Cmd {
	server := p.coordinator.GetCurrentServer()
	// Prefer the server-specific access token (resource.AccessToken) when available;
	// otherwise fall back to the user auth token stored on the coordinator.
	token := ""
	if server != nil && server.AccessToken != "" {
		token = server.AccessToken
	} else {
		token = p.coordinator.GetToken()
	}

	// Only initialize services when a server is selected and we have an auth token.
	// When returning early here, the router/pages will handle transitions until
	// the token/server become available (so we avoid creating services with nil token).
	if server == nil || token == "" {
		// Nothing to fetch yet — render a prompt that a server must be selected
		return nil
	}

	// Build base URL for the server
	baseURL := fmt.Sprintf("%s://%s", server.Scheme, server.Host)
	if server.Port != "" {
		baseURL = fmt.Sprintf("%s:%s", baseURL, server.Port)
	}

	// Create (or reuse) library service and subscribe to events. This ensures we
	// only fetch library content when we have the necessary server + token.
	if p.libSvc == nil {
		p.libSvc = service.NewLibraryServiceWithEvents(baseURL, token)
		p.libEvtCh = p.libSvc.Subscribe(p.ctx)
	} else {
		// Update base URL and token to reflect current selected server.
		p.libSvc.SetBaseURL(baseURL)
		p.libSvc.SetToken(token)
	}

	// Create (or reuse) playback service and subscribe to events.
	if p.pbSvc == nil {
		p.pbSvc = service.NewPlaybackService()
		p.pbEvtCh = p.pbSvc.Subscribe(p.ctx)
	}

	// Default to the Home tab and ensure selection indices are initialized so
	// the initial view always starts with content focused when available.
	p.coordinator.SetActiveTab(app.HomeTab)
	p.coordinator.SetSelectedAlbum(0)
	p.coordinator.SetSelectedPlaylist(0)
	p.coordinator.SetSelectedTrack(0)

	// Kick off fetching of libraries, recently added and playlists, and begin subscriptions.
	return tea.Batch(
		p.subscribeToLibraryEvents(),
		p.subscribeToPlaybackEvents(),
		p.fetchLibraries(),
		p.fetchRecentlyAdded(),
		p.fetchPlaylists(),
	)
}

// Update processes messages for the main application page, including window
// size changes, library/playback events, and key events for navigation & actions.
func (p *MainAppPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height

		// Resize lists
		contentWidth := views.GetContentPaneWidth(p.width)
		listHeight := p.height - 6 // Approximate height minus header/footer
		if listHeight < 0 {
			listHeight = 0
		}

		p.recentlyAddedList.SetSize(contentWidth, listHeight)
		p.playlistList.SetSize(contentWidth, listHeight)
		p.trackList.SetSize(contentWidth, listHeight)
		p.queueList.SetSize(contentWidth, listHeight)

		return p, nil

	case service.LibraryEvent:
		// Update coordinator state based on library events
		switch msg.Type {
		case "libraries.loaded":
			appLibs := make([]app.MusicLibrary, len(msg.Libraries))
			for i, l := range msg.Libraries {
				appLibs[i] = app.MusicLibrary{Key: l.Key, Title: l.Title, Type: l.Type}
			}
			p.coordinator.SetLibraries(appLibs)
			if len(appLibs) > 0 {
				p.coordinator.SetSelectedLibrary(0)
			}
		case "recently_added.loaded":
			// Convert domain.Album to app.Album and update coordinator
			appAlbums := make([]app.Album, len(msg.Albums))
			items := make([]list.Item, len(msg.Albums))
			for i, a := range msg.Albums {
				appAlbums[i] = app.Album{
					Title:  a.Title,
					Artist: a.Artist,
					Year:   a.Year,
					Key:    a.Key,
					Thumb:  a.Thumb,
				}
				items[i] = AlbumItem{Album: a}
			}
			p.coordinator.SetAlbums(appAlbums)
			p.recentlyAddedList.SetItems(items)
			// Keep UI selection sane
			if len(appAlbums) > 0 {
				p.coordinator.SetSelectedAlbum(0)
				p.recentlyAddedList.Select(0)
				// Reset last selected album index so first selection triggers a fetch
				p.lastSelectedAlbumIndex = -1
			}

		case "playlists.loaded":
			appPlaylists := make([]app.Playlist, len(msg.Playlists))
			items := make([]list.Item, len(msg.Playlists))
			for i, pl := range msg.Playlists {
				appPlaylists[i] = app.Playlist{
					Title:        pl.Title,
					Key:          pl.Key,
					LeafCount:    pl.LeafCount,
					Duration:     pl.Duration,
					PlaylistType: pl.PlaylistType,
				}
				items[i] = PlaylistItem{Playlist: pl}
			}
			p.coordinator.SetPlaylists(appPlaylists)
			p.playlistList.SetItems(items)
			if len(appPlaylists) > 0 {
				p.coordinator.SetSelectedPlaylist(0)
				p.playlistList.Select(0)
				// Reset last selected playlist index so first selection triggers a fetch
				p.lastSelectedPlaylistIndex = -1
			}

		case "tracks.loaded":
			appTracks := make([]app.Track, len(msg.Tracks))
			items := make([]list.Item, len(msg.Tracks))
			for i, t := range msg.Tracks {
				appTracks[i] = app.Track{
					Title:       t.Title,
					Artist:      t.Artist,
					Album:       t.Album,
					Duration:    t.Duration,
					TrackNumber: t.TrackNumber,
					Key:         t.Key,
					RatingKey:   t.RatingKey,
					Thumb:       t.Thumb,
				}
				items[i] = TrackItem{Track: t}
			}
			p.coordinator.SetTracks(appTracks)
			p.trackList.SetItems(items)
			if len(appTracks) > 0 {
				p.coordinator.SetSelectedTrack(0)
				p.trackList.Select(0)
			}
		}
		// Re-subscribe to library and playback events so we continue receiving them
		return p, tea.Batch(p.subscribeToLibraryEvents(), p.subscribeToPlaybackEvents())

	case service.AuthEvent:
		// React to auth events (mainly servers.loaded) to create a library service
		// for the currently selected server and begin fetching library data.
		switch msg.Type {
		case "servers.loaded":
			// Convert to app-level servers and set them in the coordinator.
			if len(msg.Servers) > 0 {
				appServers := make([]app.PlexServer, len(msg.Servers))
				for i, s := range msg.Servers {
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
			}

			// If we do not already have a sub/service for the selected server
			// but the coordinator already has a selection and a token, create it.
			srv := p.coordinator.GetCurrentServer()
			if srv == nil && len(msg.Servers) > 0 {
				// If no selected server, default to the first server in the event
				// and mark it as selected.
				p.coordinator.SetSelectedServer(0)
				srv = p.coordinator.GetCurrentServer()
			}
			// Prefer server-specific access token if available, otherwise fall back to coordinator token
			token := ""
			if srv != nil && srv.AccessToken != "" {
				token = srv.AccessToken
			} else {
				token = p.coordinator.GetToken()
			}
			if p.libSvc == nil && srv != nil && token != "" {
				baseURL := fmt.Sprintf("%s://%s", srv.Scheme, srv.Host)
				if srv.Port != "" {
					baseURL = fmt.Sprintf("%s:%s", baseURL, srv.Port)
				}
				p.libSvc = service.NewLibraryServiceWithEvents(baseURL, token)
				p.libEvtCh = p.libSvc.Subscribe(p.ctx)

				// Kick off a library refresh and subscribe to library events
				return p, tea.Batch(p.subscribeToLibraryEvents(), p.fetchRecentlyAdded(), p.fetchPlaylists())
			}
		case "servers.fetch_failed":
			// No-op for now; other pages will surface the failure. We re-subscribe
			// to continue receiving future events.
		}
		// Re-subscribe to auth events to continue receiving them.
		return p, tea.Batch(p.subscribeToAuthEvents(), p.subscribeToLibraryEvents(), p.subscribeToPlaybackEvents())

	case service.PlaybackEvent:
		// Keep coordinator synchronized with playback events (UI-only reflection).
		switch msg.Type {
		case "playback.started":
			p.coordinator.SetPlaybackState(app.PlaybackPlaying)
			if msg.Track != nil {
				// Convert domain.Track -> app.Track and set on coordinator
				track := app.Track{
					Title:       msg.Track.Title,
					Artist:      msg.Track.Artist,
					Album:       msg.Track.Album,
					Duration:    msg.Track.Duration,
					TrackNumber: msg.Track.TrackNumber,
					Key:         msg.Track.Key,
					RatingKey:   msg.Track.RatingKey,
					Thumb:       msg.Track.Thumb,
				}
				p.coordinator.SetCurrentTrack(&track)
			}
		case "playback.paused":
			p.coordinator.SetPlaybackState(app.PlaybackPaused)
		case "playback.stopped":
			p.coordinator.SetPlaybackState(app.PlaybackStopped)
		case "playback.volume_changed":
			// Playback service publishes floats — we don't keep it in coordinator as a primitive.
		}
		// Re-subscribe to playback and library events so we continue receiving them
		return p, tea.Batch(p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
	// animation messages removed - no-op

	case tea.KeyMsg:
		// When the Search tab is active, let the text input widget handle keys.
		if p.coordinator.ActiveTab() == app.SearchTab {
			switch msg.String() {
			case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5", "ctrl+6":
				// Allow these to fall through into the main key handling below
			default:
				var cmd tea.Cmd
				p.searchInput, cmd = p.searchInput.Update(msg)
				// If user hits Enter or Esc, apply/abort and return
				switch msg.String() {
				case "enter":
					p.searchTerm = p.searchInput.Value()
					// TODO: apply search across collections (albums/playlists/tracks)
					p.searchInput.Blur()
					return p, cmd
				case "esc":
					p.searchInput.Blur()
					p.coordinator.SetActiveTab(app.HomeTab)
					return p, cmd
				default:
					return p, cmd
				}
			}
		}

		// Key handling for the main app.
		var cmd tea.Cmd

		// Handle global keys first
		switch {
		case key.Matches(msg, p.keys.Back):
			// If the queue modal is open, close it first; otherwise go back to server selection.
			if p.coordinator.ShowQueueModal() {
				p.coordinator.SetShowQueueModal(false)
				return p, nil
			}
			// Close left-pane tracklist if open instead of closing a drawer/modal.
			if p.showingTracks {
				p.showingTracks = false
				return p, nil
			}
			return p, func() tea.Msg {
				return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
			}

		case key.Matches(msg, p.keys.Play):
			// Attempt to fetch and play the first track from the selected album
			// or playlist, or play the selected queue item.
			if p.showingTracks {
				if p.libSvc != nil {
					if item, ok := p.trackList.SelectedItem().(TrackItem); ok {
						// Convert domain.Track -> app.Track
						at := app.Track{
							Title:       item.Track.Title,
							Artist:      item.Track.Artist,
							Album:       item.Track.Album,
							Duration:    item.Track.Duration,
							TrackNumber: item.Track.TrackNumber,
							Key:         item.Track.Key,
							RatingKey:   item.Track.RatingKey,
							Thumb:       item.Track.Thumb,
						}
						p.playAppTrack(&at)
						return p, nil
					}
				}
				return p, nil
			}

			active := p.coordinator.ActiveTab()
			if p.libSvc != nil && active == app.PlaylistsTab {
				if item, ok := p.playlistList.SelectedItem().(PlaylistItem); ok {
					reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
					defer cancel()
					tracks, _ := p.libSvc.FetchTracks(reqCtx, item.Playlist.Key)
					if len(tracks) > 0 {
						at := app.Track{
							Title:       tracks[0].Title,
							Artist:      tracks[0].Artist,
							Album:       tracks[0].Album,
							Duration:    tracks[0].Duration,
							TrackNumber: tracks[0].TrackNumber,
							Key:         tracks[0].Key,
							RatingKey:   tracks[0].RatingKey,
							Thumb:       tracks[0].Thumb,
						}
						p.playAppTrack(&at)
						return p, nil
					}
				}
				return p, nil
			}

			// Play/pause. Toggle playback for selected track (if available).
			if p.pbSvc == nil {
				return p, nil
			}
			if p.coordinator.HasCurrentTrack() && p.coordinator.IsPlaying() {
				_ = p.pbSvc.Pause()
			} else {
				// Determine candidate track:
				var tr *app.Track
				// Favor a selected track in the page / coordinator if present, else take first track
				tracks := p.coordinator.Tracks()
				if len(tracks) > 0 {
					idx := p.coordinator.SelectedTrack()
					if idx < 0 || idx >= len(tracks) {
						idx = 0
					}
					tr = &tracks[idx]
					// Play via helper to keep UI in sync
					p.playAppTrack(tr)
				}
			}
			return p, nil

		case key.Matches(msg, p.keys.Next):
			p.playNext()
			return p, nil
		case key.Matches(msg, p.keys.Prev):
			p.playPrev()
			return p, nil
		case key.Matches(msg, p.keys.VolumeUp):
			p.adjustVolume(0.1)
			return p, nil
		case key.Matches(msg, p.keys.VolumeDown):
			p.adjustVolume(-0.1)
			return p, nil
		case key.Matches(msg, p.keys.Queue):
			// Toggle queue modal
			p.coordinator.SetShowQueueModal(!p.coordinator.ShowQueueModal())
			return p, nil
		case key.Matches(msg, p.keys.Refresh):
			// Refresh library lists (recently added + playlists)
			if p.libSvc != nil {
				return p, tea.Batch(p.fetchRecentlyAdded(), p.fetchPlaylists())
			}
			return p, nil
		case key.Matches(msg, p.keys.Search):
			// Toggle Search tab and focus/blur the search input accordingly
			if p.coordinator.ActiveTab() == app.SearchTab {
				p.searchInput.Blur()
				p.coordinator.SetActiveTab(app.HomeTab)
			} else {
				p.coordinator.SetActiveTab(app.SearchTab)
				p.searchInput.Focus()
			}
			return p, nil
		case key.Matches(msg, p.keys.FocusNowPlaying):
			// Toggle pair of focused Now Playing view
			p.focusedNowPlaying = !p.focusedNowPlaying
			return p, nil
		case key.Matches(msg, p.keys.SwitchView):
			switch msg.String() {
			case "ctrl+1":
				p.coordinator.SetActiveTab(app.HomeTab)
			case "ctrl+2":
				p.coordinator.SetActiveTab(app.LibraryTab)
			case "ctrl+3":
				p.coordinator.SetActiveTab(app.PlaylistsTab)
			case "ctrl+4":
				p.coordinator.SetActiveTab(app.SearchTab)
			case "ctrl+5":
				p.coordinator.SetActiveTab(app.QueueTab)
			case "ctrl+6":
				p.coordinator.SetActiveTab(app.SettingsTab)
			}
			if p.showingTracks {
				p.showingTracks = false
			}
			return p, nil
		}

		// Delegate to active list
		active := p.coordinator.ActiveTab()

		if p.showingTracks {
			p.trackList, cmd = p.trackList.Update(msg)
			p.coordinator.SetSelectedTrack(p.trackList.Index())

			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.trackList.SelectedItem().(TrackItem); ok {
					at := app.Track{
						Title:       item.Track.Title,
						Artist:      item.Track.Artist,
						Album:       item.Track.Album,
						Duration:    item.Track.Duration,
						TrackNumber: item.Track.TrackNumber,
						Key:         item.Track.Key,
						RatingKey:   item.Track.RatingKey,
						Thumb:       item.Track.Thumb,
					}
					p.playAppTrack(&at)
				}
			}
			return p, cmd
		}

		switch active {
		case app.HomeTab, app.LibraryTab:
			p.recentlyAddedList, cmd = p.recentlyAddedList.Update(msg)
			p.coordinator.SetSelectedAlbum(p.recentlyAddedList.Index())

			// If selection changed, fetch tracks for the selected album in the background
			if item, ok := p.recentlyAddedList.SelectedItem().(AlbumItem); ok {
				newIdx := p.recentlyAddedList.Index()
				if newIdx != p.lastSelectedAlbumIndex && p.libSvc != nil {
					p.lastSelectedAlbumIndex = newIdx
					// On selection change prefetch tracks in the background.
					// The UI should only open the track list on Enter.
					cmd = tea.Batch(cmd, p.fetchTracksCmd(item.Album.Key))
				}
			}

			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.recentlyAddedList.SelectedItem().(AlbumItem); ok {
					if p.libSvc != nil {
						reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
						defer cancel()
						_, _ = p.libSvc.FetchTracks(reqCtx, item.Album.Key)
						p.showingTracks = true
					}
				}
			}

		case app.PlaylistsTab:
			p.playlistList, cmd = p.playlistList.Update(msg)
			p.coordinator.SetSelectedPlaylist(p.playlistList.Index())

			// If selection changed, fetch tracks for the selected playlist in the background
			if item, ok := p.playlistList.SelectedItem().(PlaylistItem); ok {
				newIdx := p.playlistList.Index()
				if newIdx != p.lastSelectedPlaylistIndex && p.libSvc != nil {
					p.lastSelectedPlaylistIndex = newIdx
					// On selection change prefetch tracks in the background.
					// The UI should only open the track list on Enter.
					cmd = tea.Batch(cmd, p.fetchTracksCmd(item.Playlist.Key))
				}
			}

			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.playlistList.SelectedItem().(PlaylistItem); ok {
					if p.libSvc != nil {
						reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
						defer cancel()
						_, _ = p.libSvc.FetchTracks(reqCtx, item.Playlist.Key)
						p.showingTracks = true
					}
				}
			}

		case app.QueueTab:
			p.queueList, cmd = p.queueList.Update(msg)
			p.coordinator.SetQueueIndex(p.queueList.Index())

			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.queueList.SelectedItem().(QueueItem); ok {
					at := app.Track{
						Title:       item.Track.Title,
						Artist:      item.Track.Artist,
						Album:       item.Track.Album,
						Duration:    item.Track.Duration,
						TrackNumber: item.Track.TrackNumber,
						Key:         item.Track.Key,
						RatingKey:   item.Track.RatingKey,
						Thumb:       item.Track.Thumb,
					}
					p.playAppTrack(&at)
				}
			}
		}

		return p, cmd
	}
	return p, nil
}

// View renders the main app page using a tabbed layout. It includes a nav pane,
// main content pane, and a detail/now-playing pane. When the queue is visible,
// a modal overlay is displayed.
func (p *MainAppPage) View() string {
	// Ensure we have width/height for layout calculations. If the page hasn't
	// received a WindowSizeMsg yet (width/height == 0) fall back to either the
	// coordinator-provided dimensions or to pragmatic defaults so we can render
	// the main UI immediately instead of returning an empty string.
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
			lipgloss.NewStyle().Padding(1, 2).Render(lipgloss.JoinVertical(lipgloss.Center, title, "", content, "", help)),
		)
	}

	contentWidth := views.GetContentPaneWidth(p.width)
	detailWidth := views.GetDetailPaneWidth(p.width)
	// Split the main layout into two panes: left controlled by tabs (contentWidth)
	// and right showing the Now Playing (detailWidth). Ensure both fit into the
	// available width, adjusting when necessary.
	leftWidth := contentWidth
	rightWidth := detailWidth
	usableWidth := p.width - 6
	if leftWidth+rightWidth > usableWidth {
		// Shrink right pane to fit while keeping left pane size readable.
		rightWidth = usableWidth - leftWidth
		if rightWidth < 20 {
			// Ensure a sensible minimum for the right pane.
			rightWidth = 20
			if leftWidth > usableWidth-rightWidth {
				leftWidth = usableWidth - rightWidth
			}
		}
	}

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
		leftContent = p.renderSearch(leftWidth)
	case app.SettingsTab:
		leftContent = p.renderSettings(leftWidth)
	default:
		leftContent = p.renderRecentlyAdded(leftWidth)
	}

	// Main content — Now Playing becomes the primary content area.
	mainContent := p.renderNowPlaying(rightWidth)
	if p.libSvc != nil && len(p.coordinator.Albums()) == 0 && len(p.coordinator.Playlists()) == 0 {
		// Show a friendly, centered loading placeholder in the content area when
		// the library service is active but no content has been loaded yet.
		mainContent = lipgloss.JoinVertical(lipgloss.Center, styles.BlurredStyle.Render("Loading library..."))
	}
	contentPane := styles.PaneStyle(rightWidth, p.height-6).Render(mainContent)

	leftContentHeight := p.height - 6
	if leftContentHeight < 6 {
		// Ensure a sensible minimum height for left content
		leftContentHeight = 6
	}

	leftPane := styles.PaneStyle(leftWidth, leftContentHeight).Render(leftContent)

	// Compose the two-pane layout: left column (content) and right pane (Now Playing)
	panesRow := lipgloss.JoinHorizontal(lipgloss.Left, leftPane, contentPane)
	layout := panesRow

	// If Queue modal is visible, overlay it
	if p.coordinator.ShowQueueModal() {
		return p.renderWithModal(layout)
	}

	// If the user has activated the focused Now Playing panel (full screen),
	// show it immediately.
	if p.focusedNowPlaying {
		// Leave the rest of the layout behind — a full-screen now-playing UI is focused.
		return lipgloss.Place(
			p.width,
			p.height,
			lipgloss.Center,
			lipgloss.Center,
			p.renderNowPlayingFull(p.width, p.height),
		)
	}

	// Drawers are removed; no overlay drawer behavior needed.
	// If the drawer is not present and the (ancillary) modal was requested but
	// the drawer offset is zero (maybe animation not yet started), fall back to the
	// previous centered modal behavior to prevent a confusing blank state.
	// No centered modal fallback - drawers were removed.

	server = p.coordinator.GetCurrentServer()
	pageTitle := "Plex Music"
	if server != nil && server.Name != "" {
		pageTitle = fmt.Sprintf("Plex Music — %s", server.Name)
	}
	// Build a compact status line showing server, auth, and counts of content.
	serverName := "none"
	if server != nil && server.Name != "" {
		serverName = server.Name
	}
	authStatus := "Signed Out"
	if p.coordinator != nil && p.coordinator.GetToken() != "" {
		authStatus = "Signed In"
	}
	albumsCount := 0
	playlistsCount := 0
	tracksCount := 0
	if p.coordinator != nil {
		albumsCount = len(p.coordinator.Albums())
		playlistsCount = len(p.coordinator.Playlists())
		tracksCount = len(p.coordinator.Tracks())
	}

	statusLine := styles.BlurredStyle.Render(fmt.Sprintf("Server: %s • %s • Albums: %d • Playlists: %d • Tracks: %d", serverName, authStatus, albumsCount, playlistsCount, tracksCount))

	centeredLayout := lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Top, layout)
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TitleStyle.Render(pageTitle),
		statusLine,
		centeredLayout,
	)
}

// Close cancels any subscriptions and releases resources used by the page.
func (p *MainAppPage) Close() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.libSvc != nil {
		_ = p.libSvc.Close()
	}
	if p.pbSvc != nil {
		_ = p.pbSvc.Close()
	}
}

// ---- Helpers ----

// subscribeToLibraryEvents returns a command that listens for library events
// and returns them as tea messages (LibraryEvent) for processing in Update.
func (p *MainAppPage) subscribeToLibraryEvents() tea.Cmd {
	if p.libEvtCh == nil {
		return nil
	}
	return func() tea.Msg {
		for ev := range p.libEvtCh {
			// Only forward useful events for this page
			switch ev.Type {
			case "recently_added.loaded", "playlists.loaded", "tracks.loaded", "albums.loaded":
				return ev.Payload
			default:
				continue
			}
		}
		return nil
	}
}

// subscribeToPlaybackEvents returns a command that listens for playback events
// and returns them as tea messages for processing in Update.
func (p *MainAppPage) subscribeToPlaybackEvents() tea.Cmd {
	if p.pbEvtCh == nil {
		return nil
	}
	return func() tea.Msg {
		for ev := range p.pbEvtCh {
			return ev.Payload
		}
		return nil
	}
}

// subscribeToAuthEvents listens for AuthService events and forwards ones
// relevant to server discovery to the page as tea messages.
func (p *MainAppPage) subscribeToAuthEvents() tea.Cmd {
	if p.authEvtCh == nil {
		if p.authSvc == nil {
			return nil
		}
		p.authEvtCh = p.authSvc.Subscribe(p.ctx)
	}
	return func() tea.Msg {
		for ev := range p.authEvtCh {
			// Only forward servers related events (servers.loaded, servers.fetch_failed)
			if ev.Type == "servers.loaded" || ev.Type == "servers.fetch_failed" {
				return ev.Payload
			}
		}
		return nil
	}
}

// fetchLibraries triggers the library service to fetch available libraries.
func (p *MainAppPage) fetchLibraries() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		_, _ = p.libSvc.FetchLibraries(ctx)
		return nil
	}
}

// fetchRecentlyAdded triggers the library service to fetch recently added.
func (p *MainAppPage) fetchRecentlyAdded() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		_, _ = p.libSvc.FetchRecentlyAdded(ctx)
		return nil
	}
}

// fetchPlaylists triggers the library service to fetch playlists.
func (p *MainAppPage) fetchPlaylists() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		_, _ = p.libSvc.FetchPlaylists(ctx)
		return nil
	}
}

// fetchTracksCmd returns a tea.Cmd that fetches tracks for the given key
// using the library service and returns nil as the tea.Message (we rely on
// library events to update the UI when the response arrives).
func (p *MainAppPage) fetchTracksCmd(key string) tea.Cmd {
	if p.libSvc == nil || key == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		_, _ = p.libSvc.FetchTracks(ctx, key)
		return nil
	}
}

// renderRecentlyAdded displays the current recently-added albums list.
func (p *MainAppPage) renderRecentlyAdded(width int) string {
	p.recentlyAddedList.SetWidth(width)
	return lipgloss.JoinVertical(lipgloss.Left, p.recentlyAddedList.View(), "", p.help.View(p.keys))
}

// renderPlaylists displays the playlists list.
func (p *MainAppPage) renderPlaylists(width int) string {
	p.playlistList.SetWidth(width)
	return lipgloss.JoinVertical(lipgloss.Left, p.playlistList.View(), "", p.help.View(p.keys))
}

// renderQueue displays the queued tracks list.
func (p *MainAppPage) renderQueue(width int) string {
	p.queueList.SetWidth(width)
	return lipgloss.JoinVertical(lipgloss.Left, p.queueList.View(), "", p.help.View(p.keys))
}

// renderSearch displays the search input and inline results in the left pane.
func (p *MainAppPage) renderSearch(width int) string {
	title := styles.TitleStyle.Render("Search")
	results := []string{}
	term := strings.TrimSpace(p.searchInput.Value())
	if term != "" {
		for _, a := range p.coordinator.Albums() {
			if strings.Contains(strings.ToLower(a.Title), strings.ToLower(term)) || strings.Contains(strings.ToLower(a.Artist), strings.ToLower(term)) {
				results = append(results, fmt.Sprintf("%s — %s", a.Title, a.Artist))
			}
		}
		for _, pl := range p.coordinator.Playlists() {
			if strings.Contains(strings.ToLower(pl.Title), strings.ToLower(term)) {
				results = append(results, fmt.Sprintf("%s (playlist)", pl.Title))
			}
		}
	}
	if len(results) == 0 {
		results = append(results, styles.BlurredStyle.Render("No matches"))
	}
	help := p.help.View(p.keys)
	return lipgloss.JoinVertical(lipgloss.Left, title, "", p.searchInput.View(), "", lipgloss.JoinVertical(lipgloss.Left, results...), "", help)
}

// renderSettings displays a simple settings placeholder in the left pane.
func (p *MainAppPage) renderSettings(width int) string {
	title := styles.TitleStyle.Render("Settings")
	lines := []string{
		styles.BlurredStyle.Render("No settings available yet."),
		styles.BlurredStyle.Render("Press Esc to close."),
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, "", lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// renderTracks displays the currently selected tracks in the left pane.
func (p *MainAppPage) renderTracks(width int) string {
	p.trackList.SetWidth(width)
	return lipgloss.JoinVertical(lipgloss.Left, p.trackList.View(), "", p.help.View(p.keys))
}

// renderNowPlaying shows the now playing details, a small cover-art placeholder,
// playback progress and volume controls.
func (p *MainAppPage) renderNowPlaying(width int) string {
	// Existing small/compact Right-hand Now Playing
	title := styles.TitleStyle.Render("Now Playing")

	// If no track is present, show a 'Nothing Playing' placeholder
	if !p.coordinator.HasCurrentTrack() {
		help := styles.NothingPlayingHintStyle()
		return lipgloss.JoinVertical(lipgloss.Center, title, "", styles.NothingPlayingStyle(), "", help)
	}

	tr := p.coordinator.CurrentTrack()
	trackTitle := styles.PrimaryTextStyle().Render(tr.Title)
	artist := styles.SecondaryTextStyle().Render(tr.Artist)
	album := styles.TertiaryTextStyle().Render(tr.Album)

	// Use sample pos/length and sample rate (if available) to compute a time-based position.
	posSamples := p.coordinator.StreamPosition()
	lengthSamples := p.coordinator.StreamLength()
	sr := int(p.coordinator.SampleRate())

	var posMs, lenMs int
	if sr > 0 && lengthSamples > 0 {
		posMs = posSamples * 1000 / sr
		lenMs = lengthSamples * 1000 / sr
	} else {
		// Fallback to former approach, but prefer track.Duration if available.
		posMs = 0
		lenMs = tr.Duration
	}

	// If lenMs still zero, fallback to no length display.
	if lenMs == 0 && tr != nil {
		lenMs = tr.Duration
	}

	posStr := views.FormatTrackDuration(posMs)
	lenStr := views.FormatTrackDuration(lenMs)

	// Build a progress bar roughly sized to the detail width
	barWidth := width - 12
	if barWidth < 8 {
		barWidth = 8
	}
	var pct float64
	if lenMs > 0 {
		pct = float64(posMs) / float64(lenMs)
		if pct < 0 {
			pct = 0
		} else if pct > 1 {
			pct = 1
		}
	} else {
		pct = 0
	}
	filled := int(pct * float64(barWidth))
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}

	barFill := strings.Repeat("█", filled)
	barEmpty := strings.Repeat(" ", barWidth-filled)
	progressBar := fmt.Sprintf("[%s%s] %s / %s",
		styles.FocusedStyle.Render(barFill),
		styles.BlurredStyle.Render(barEmpty),
		posStr,
		lenStr,
	)

	// Volume display: read from coordinator's volume effect if available
	volume := ""
	if vol := p.coordinator.Volume(); vol != nil {
		volume = fmt.Sprintf("Vol: %.2f", vol.Volume)
	}

	controls := p.help.View(p.keys)

	// Render album art using the playback renderer (if available). Fall back to a thumb/url line.
	art := p.coordinator.PlaybackAlbumArt()
	var artView string
	if art != nil && p.coordinator.PlaybackImgRenderer() != nil {
		// Give the art roughly 40-50% of the detail width with a lower bound
		artW := width * 45 / 100
		if artW < 6 {
			artW = 6
		}
		artH := artW / 2
		// Guard against zero size
		if artH < 3 {
			artH = 3
		}
		artView = p.coordinator.PlaybackImgRenderer().Render(art, artW, artH)
	} else {
		// Fallback to the thumbnail URL if image rendering is not available.
		thumb := p.coordinator.PlaybackAlbumArtThumb()
		if thumb != "" {
			artView = styles.PrimaryTextStyle().Render(fmt.Sprintf("Art: %s", thumb))
		} else {
			artView = styles.BlurredStyle.Render("(Album art)")
		}
	}

	rightColumn := lipgloss.JoinVertical(lipgloss.Left,
		trackTitle,
		artist,
		album,
		"",
		styles.BlurredStyle.Render(progressBar),
		styles.BlurredStyle.Render(volume),
		"",
		controls,
	)

	// If we have artView (likely multi-line), render art and info side-by-side.
	if artView != "" {
		return lipgloss.JoinHorizontal(lipgloss.Left,
			artView,
			lipgloss.NewStyle().Padding(0, 2).Render(rightColumn),
		)
	}

	// Fallback: render info block only
	return rightColumn
}

// renderWithModal composes the base view layout with the queue modal overlay.
func (p *MainAppPage) renderWithModal(base string) string {
	queue := p.coordinator.Queue()
	var lines []string
	lines = append(lines, styles.TitleStyle.Render("Queue"))
	lines = append(lines, "")
	if len(queue) == 0 {
		lines = append(lines, styles.BlurredStyle.Render("Queue is empty"))
	} else {
		for i, t := range queue {
			prefix := "  "
			if i == p.coordinator.QueueIndex() {
				prefix = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%s — %s", prefix, t.Title, t.Artist))
		}
	}

	modal := lipgloss.JoinVertical(lipgloss.Left, lines...)
	modalStyled := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(60).Render(modal)

	// Center overlay
	return lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, base, "", modalStyled),
	)
}

// renderNowPlayingFull renders a full-screen focused Now Playing page.
func (p *MainAppPage) renderNowPlayingFull(width int, height int) string {
	// Build a large art presentation area plus metadata and controls below
	if !p.coordinator.HasCurrentTrack() {
		help := styles.NothingPlayingHintStyle()
		return lipgloss.JoinVertical(lipgloss.Center, styles.TitleStyle.Render("Now Playing"), "", styles.NothingPlayingStyle(), "", help)
	}

	tr := p.coordinator.CurrentTrack()
	title := styles.PrimaryTextStyle().Render(tr.Title)
	artist := styles.SecondaryTextStyle().Render(tr.Artist)
	album := styles.TertiaryTextStyle().Render(tr.Album)

	art := p.coordinator.PlaybackAlbumArt()
	var artView string
	if art != nil && p.coordinator.PlaybackImgRenderer() != nil {
		// Compute a larger art size for full-screen mode
		artW := width * 60 / 100
		if artW < 20 {
			artW = 20
		}
		artH := height * 50 / 100
		if artH < 10 {
			artH = 10
		}
		artView = p.coordinator.PlaybackImgRenderer().Render(art, artW, artH)
	} else {
		thumb := p.coordinator.PlaybackAlbumArtThumb()
		if thumb != "" {
			artView = styles.PrimaryTextStyle().Render(fmt.Sprintf("Art: %s", thumb))
		} else {
			artView = styles.BlurredStyle.Render("(Album art)")
		}
	}

	// Playback info and controls
	posSamples := p.coordinator.StreamPosition()
	lengthSamples := p.coordinator.StreamLength()
	sr := int(p.coordinator.SampleRate())
	var posMs, lenMs int
	if sr > 0 && lengthSamples > 0 {
		posMs = posSamples * 1000 / sr
		lenMs = lengthSamples * 1000 / sr
	} else {
		posMs = 0
		lenMs = tr.Duration
	}
	posStr := views.FormatTrackDuration(posMs)
	lenStr := views.FormatTrackDuration(lenMs)

	// Simple, wide progress bar
	barWidth := width - 10
	if barWidth < 12 {
		barWidth = 12
	}
	var pct float64
	if lenMs > 0 {
		pct = float64(posMs) / float64(lenMs)
		if pct < 0 {
			pct = 0
		} else if pct > 1 {
			pct = 1
		}
	} else {
		pct = 0
	}
	filled := int(pct * float64(barWidth))
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}
	barFill := strings.Repeat("█", filled)
	barEmpty := strings.Repeat(" ", barWidth-filled)
	progressBar := fmt.Sprintf("[%s%s] %s / %s",
		styles.FocusedStyle.Render(barFill),
		styles.BlurredStyle.Render(barEmpty),
		posStr,
		lenStr,
	)

	controls := p.help.View(p.keys)

	// Compose a top-to-bottom full-screen now playing view
	info := lipgloss.JoinVertical(lipgloss.Left,
		styles.TitleStyle.Render("Now Playing"),
		"",
		title,
		artist,
		album,
		"",
		styles.BlurredStyle.Render(progressBar),
		"",
		styles.BlurredStyle.Render(fmt.Sprintf("Volume: %s", styles.BlurredStyle.Render(fmt.Sprintf("%.2f", func() float64 {
			if vol := p.coordinator.Volume(); vol != nil {
				return vol.Volume
			}
			return 1.0
		}())))),
		"",
		controls,
	)

	return lipgloss.JoinHorizontal(lipgloss.Center,
		lipgloss.NewStyle().Padding(1, 2).Render(artView),
		lipgloss.NewStyle().Padding(1, 2).Render(info),
	)
}

// Helper: convert an app.Track into a domain.Track for playback calls
func (p *MainAppPage) appTrackToDomain(at *app.Track) *domain.Track {
	if at == nil {
		return nil
	}
	return &domain.Track{
		Title:       at.Title,
		Artist:      at.Artist,
		Album:       at.Album,
		Duration:    at.Duration,
		TrackNumber: at.TrackNumber,
		Key:         at.Key,
		RatingKey:   at.RatingKey,
		Thumb:       at.Thumb,
	}
}

// Helper: play the provided app.Track (UI & playback service)
func (p *MainAppPage) playAppTrack(at *app.Track) {
	if at == nil {
		return
	}
	// Update UI coordinator state
	p.coordinator.SetCurrentTrack(at)
	p.coordinator.SetPlaybackState(app.PlaybackPlaying)

	// Delegate to playback service if available.
	if p.pbSvc != nil {
		dt := p.appTrackToDomain(at)
		_ = p.pbSvc.Play(dt)
	}
}

// Helper: play next track (queue preferred, otherwise tracklist)
func (p *MainAppPage) playNext() {
	// Prefer queue if present
	q := p.coordinator.Queue()
	if len(q) > 0 {
		idx := p.coordinator.QueueIndex()
		if idx < 0 {
			idx = 0
		} else {
			idx++
			if idx >= len(q) {
				idx = 0
			}
		}
		p.coordinator.SetQueueIndex(idx)
		p.playAppTrack(&q[idx])
		return
	}

	// Otherwise iterate through tracklist
	tracks := p.coordinator.Tracks()
	if len(tracks) == 0 {
		return
	}
	idx := p.coordinator.SelectedTrack()
	if idx < 0 || idx >= len(tracks)-1 {
		idx = 0
	} else {
		idx++
	}
	p.coordinator.SetSelectedTrack(idx)
	p.playAppTrack(&tracks[idx])
}

// Helper: play previous track (queue preferred, otherwise tracklist)
func (p *MainAppPage) playPrev() {
	// Prefer queue if present
	q := p.coordinator.Queue()
	if len(q) > 0 {
		idx := p.coordinator.QueueIndex()
		if idx <= 0 {
			idx = len(q) - 1
		} else {
			idx--
		}
		p.coordinator.SetQueueIndex(idx)
		p.playAppTrack(&q[idx])
		return
	}

	// Otherwise iterate through tracklist
	tracks := p.coordinator.Tracks()
	if len(tracks) == 0 {
		return
	}
	idx := p.coordinator.SelectedTrack()
	if idx <= 0 {
		idx = len(tracks) - 1
	} else {
		idx--
	}
	p.coordinator.SetSelectedTrack(idx)
	p.playAppTrack(&tracks[idx])
}

// Helper: adjust volume by delta (range 0.0..2.0)
func (p *MainAppPage) adjustVolume(delta float64) {
	if p.pbSvc != nil {
		v := p.pbSvc.GetVolume()
		v += delta
		if v < 0 {
			v = 0
		} else if v > 2 {
			v = 2
		}
		p.pbSvc.SetVolume(v)
		return
	}
	// Fallback: adjust coordinator volume effect directly if present
	if vol := p.coordinator.Volume(); vol != nil {
		vol.Volume += delta
		if vol.Volume < 0 {
			vol.Volume = 0
		} else if vol.Volume > 2 {
			vol.Volume = 2
		}
	}
}
