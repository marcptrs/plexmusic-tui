package pages

import (
	"context"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log/v2"
	"github.com/faiface/beep"

	"plexmusic-tui/internal/app"
	domain "plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	components "plexmusic-tui/internal/tui/components"
	styles "plexmusic-tui/internal/tui/styles"
	"plexmusic-tui/internal/tui/util"
)

// LibraryPage handles the library browsing UI with tab navigation,
// list rendering (Recently Added, Playlists), modal dialogs, and the
// "Now Playing" panel (cover art + controls).
type LibraryPage struct {
	coordinator app.Coordinatorer

	width, height int

	// Services and subscriptions
	ctx       context.Context
	cancel    context.CancelFunc
	authSvc   service.AuthServicer
	authEvtCh <-chan pubsub.Event[service.AuthEvent]
	libSvc    *service.LibraryServiceWithEvents
	libEvtCh  <-chan pubsub.Event[service.LibraryEvent]
	// playback service is handled by the orchestrator (no pbSvc field on page)
	pbEvtCh <-chan pubsub.Event[service.PlaybackEvent]

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

	help         help.Model
	keys         tui.LibraryKeyMap
	nowPlaying   *components.NowPlayingComponent
	orchestrator *tui.Orchestrator
}

// NewLibraryPage creates a new library browsing page and sets up a
// cancellable background context for event subscriptions.
func NewLibraryPage(coord app.Coordinatorer) *LibraryPage {
	return NewLibraryPageWithAuth(coord, nil)
}

func NewLibraryPageWithAuth(coord app.Coordinatorer, authSvc service.AuthServicer) *LibraryPage {
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

	p := &LibraryPage{
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
		keys:                      tui.DefaultLibraryKeyMap(),
		recentlyAddedList:         raList,
		playlistList:              plList,
		trackList:                 trList,
		queueList:                 qList,
		nowPlaying:                components.NewNowPlayingComponent(coord, nil),
		orchestrator:              tui.NewOrchestrator(coord, nil, nil),
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

// Init initializes the library page. This attempts to set up library and
// playback services for the current server (if present) and kick off the
// initial fetches for Recently Added + Playlists.
func (p *LibraryPage) Init() tea.Cmd {
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
	log.Debug("LibraryPage: connecting to server", "name", server.Name, "baseURL", baseURL, "scheme", server.Scheme, "host", server.Host, "port", server.Port)

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
	// Initialize playback orchestrator using coordinator-provided service or creating a new one.
	var pbSvc service.PlaybackServicer
	if p.coordinator != nil && p.coordinator.PlaybackService() != nil {
		pbSvc = p.coordinator.PlaybackService()
	} else {
		pbSvcLocal := service.NewPlaybackService()
		pbSvc = pbSvcLocal
		if p.coordinator != nil {
			p.coordinator.SetPlaybackService(pbSvcLocal)
		}
	}
	// create or update orchestrator and nowplaying component (pass orchestrator as PlaybackServicer)
	p.orchestrator = tui.NewOrchestrator(p.coordinator, p.libSvc, pbSvc)
	p.pbEvtCh = p.orchestrator.Subscribe(p.ctx)
	if p.nowPlaying == nil {
		p.nowPlaying = components.NewNowPlayingComponent(p.coordinator, p.orchestrator)
	} else {
		p.nowPlaying = components.NewNowPlayingComponent(p.coordinator, p.orchestrator)
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

// Update processes messages for the library page, including window
// size changes, library/playback events, and key events for navigation & actions.
func (p *LibraryPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		log.Debug("LibraryPage: WindowSizeMsg", "width", p.width, "height", p.height)

		// Resize lists
		// We use the same calculation as in View() to ensure consistency
		usableWidth := p.width - 4
		leftWidth := usableWidth * 40 / 100
		if leftWidth < 30 {
			leftWidth = 30
		}

		// Adjust height to accommodate help view at the bottom (approx 2 lines)
		// and header/footer/borders.
		// In View(), contentHeight := p.height - 8
		// The list is inside the pane, which has borders (2 lines).
		// So list height should be contentHeight - 2?
		// Or does SetSize include borders? bubbles/list usually handles its own sizing.
		// If we put the list inside a pane, the pane adds borders.
		// So the list should be sized to fit INSIDE the pane.

		contentHeight := p.height - 8
		if contentHeight < 6 {
			contentHeight = 6
		}
		// Pane borders take 2 lines (top+bottom)
		listHeight := contentHeight - 2
		if listHeight < 0 {
			listHeight = 0
		}

		p.recentlyAddedList.SetSize(leftWidth, listHeight)
		p.playlistList.SetSize(leftWidth, listHeight)
		p.trackList.SetSize(leftWidth, listHeight)
		p.queueList.SetSize(leftWidth, listHeight)

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
				items[i] = util.AlbumItem{Album: a}
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
				items[i] = util.PlaylistItem{Playlist: pl}
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
				if at := util.DomainTrackToApp(&t); at != nil {
					appTracks[i] = *at
				}
				items[i] = util.TrackItem{Track: t}
			}
			p.coordinator.SetTracks(appTracks)
			p.trackList.SetItems(items)
			if len(appTracks) > 0 {
				p.coordinator.SetSelectedTrack(0)
				p.trackList.Select(0)
			}

		// Error cases: log and display notification
		case "libraries.fetch_failed", "recently_added.fetch_failed", "playlists.fetch_failed", "albums.fetch_failed", "tracks.fetch_failed":
			if msg.Error != nil {
				// Extract just the error message without full error chain for display
				errMsg := msg.Error.Error()
				// Truncate if too long for UI
				if len(errMsg) > 60 {
					errMsg = errMsg[:60] + "..."
				}
				// Show truncated message in UI
				p.coordinator.SetNotification(fmt.Sprintf("Library fetch error: %s", errMsg), "error", 10*time.Second)
				// Log full details including event type for debugging
				log.Error("Library fetch failed", "event_type", msg.Type, "full_error", msg.Error.Error())
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
		case "playback.load_failed":
			if msg.Error != nil {
				p.coordinator.SetNotification(fmt.Sprintf("Load failed: %v", msg.Error), "error", 10*time.Second)
				log.Debug("LibraryPage: set load_failed notification", "err", msg.Error)
			} else {
				p.coordinator.SetNotification("Load failed", "error", 10*time.Second)
				log.Debug("LibraryPage: set load_failed notification (no err)")
			}
		case "playback.play_failed":
			if msg.Error != nil {
				p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", msg.Error), "error", 10*time.Second)
			} else {
				p.coordinator.SetNotification("Play failed", "error", 10*time.Second)
			}
		case "playback.started":
			p.coordinator.SetPlaybackState(app.PlaybackPlaying)
			if msg.Track != nil {
				track := util.DomainTrackToApp(msg.Track)
				p.coordinator.SetCurrentTrack(track)
			}
		case "playback.resumed":
			p.coordinator.SetPlaybackState(app.PlaybackPlaying)
		case "playback.paused":
			p.coordinator.SetPlaybackState(app.PlaybackPaused)
		case "playback.stopped":
			p.coordinator.SetPlaybackState(app.PlaybackStopped)
		case "playback.volume_changed":
			// Playback service publishes floats — we don't keep it in coordinator as a primitive.
		case "playback.position":
			// Periodic position updates from the service.
			if msg.Position >= 0 {
				p.coordinator.SetStreamPosition(msg.Position)
			}
			if msg.Length > 0 {
				p.coordinator.SetStreamLength(msg.Length)
			}
			if msg.SampleRate > 0 {
				p.coordinator.SetSampleRate(beep.SampleRate(msg.SampleRate))
			}
		}
		// Re-subscribe to playback and library events so we continue receiving them
		return p, tea.Batch(p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
	// animation messages removed - no-op

	case CoverArtLoadedMsg:
		p.coordinator.SetPlaybackAlbumArt(msg.Image, msg.Path)
		return p, nil

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

		// Key handling for the library page.
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
			log.Debug("Play key matched", "msg", msg)

			if p.orchestrator == nil {
				p.coordinator.SetNotification("Playback unavailable: orchestrator is not initialized", "error", 5*time.Second)
				return p, nil
			}

			// Priority: Toggle playback if active (Playing or Paused)
			if p.coordinator.IsPlaying() {
				if p.orchestrator != nil {
					_ = p.orchestrator.Pause()
				} else {
					p.coordinator.SetNotification("Pause failed: playback orchestrator unavailable", "error", 5*time.Second)
				}
				return p, nil
			}
			if p.coordinator.IsPaused() {
				if p.orchestrator != nil {
					_ = p.orchestrator.Resume()
				} else {
					p.coordinator.SetNotification("Resume failed: playback orchestrator unavailable", "error", 5*time.Second)
				}
				return p, nil
			}

			// If we are Stopped, try to restart the current track if available.
			if p.coordinator.HasCurrentTrack() {
				if tr := p.coordinator.CurrentTrack(); tr != nil {
					return p, p.playAppTrack(tr)
				}
			}
			return p, nil

		case key.Matches(msg, p.keys.PlaySelected):
			log.Debug("PlaySelected key matched", "msg", msg)

			// Attempt to fetch and play the first track from the selected album
			// or playlist, or play the selected queue item.
			if p.showingTracks {
				if p.libSvc != nil {
					if item, ok := p.trackList.SelectedItem().(util.TrackItem); ok {
						// Convert domain.Track -> app.Track
						at := util.DomainTrackToApp(&item.Track)
						return p, p.playAppTrack(at)
					}
				}
				return p, nil
			}

			active := p.coordinator.ActiveTab()
			if p.libSvc != nil && (active == app.PlaylistsTab || active == app.HomeTab || active == app.LibraryTab) {
				if active == app.PlaylistsTab {
					if item, ok := p.playlistList.SelectedItem().(util.PlaylistItem); ok {
						reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
						defer cancel()
						// Fetch tracks for the playlist and set them as the queue.
						tracks, _ := p.libSvc.FetchTracks(reqCtx, item.Playlist.Key)
						if len(tracks) > 0 {
							// Build app.Track list and list items
							appTracks := make([]app.Track, len(tracks))
							items := make([]list.Item, len(tracks))
							for i, t := range tracks {
								if at := util.DomainTrackToApp(&t); at != nil {
									appTracks[i] = *at
								}
								items[i] = util.TrackItem{Track: t}
							}
							// Update coordinator & list state
							p.coordinator.SetQueue(appTracks)
							p.coordinator.SetQueueIndex(0)
							p.coordinator.SetTracks(appTracks)
							p.trackList.SetItems(items)
							p.trackList.Select(0)
							p.coordinator.SetSelectedTrack(0)
							p.showingTracks = true
							// Play the first track of the queue
							return p, p.playAppTrack(&appTracks[0])
						}
					}
				} else {
					// Home or Library tab: use selected album
					if item, ok := p.recentlyAddedList.SelectedItem().(util.AlbumItem); ok {
						reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
						defer cancel()
						tracks, _ := p.libSvc.FetchTracks(reqCtx, item.Album.Key)
						if len(tracks) > 0 {
							// Build app.Track list and list items
							appTracks := make([]app.Track, len(tracks))
							items := make([]list.Item, len(tracks))
							for i, t := range tracks {
								if at := util.DomainTrackToApp(&t); at != nil {
									appTracks[i] = *at
								}
								items[i] = util.TrackItem{Track: t}
							}
							p.coordinator.SetQueue(appTracks)
							p.coordinator.SetQueueIndex(0)
							p.coordinator.SetTracks(appTracks)
							p.trackList.SetItems(items)
							p.trackList.Select(0)
							p.coordinator.SetSelectedTrack(0)
							p.showingTracks = true
							return p, p.playAppTrack(&appTracks[0])
						}
					}
				}
				return p, nil
			}

			// Fallback: if we have tracks loaded in the coordinator but not showingTracks (e.g. queue view?)
			// or if we just want to play the selected track from the current list if nothing else matched.
			// The original code had a fallback here.
			tracks := p.coordinator.Tracks()
			if len(tracks) > 0 {
				idx := p.coordinator.SelectedTrack()
				if idx < 0 || idx >= len(tracks) {
					idx = 0
				}
				tr := &tracks[idx]
				return p, p.playAppTrack(tr)
			}
			return p, nil

		case key.Matches(msg, p.keys.Next):
			return p, p.playNext()
		case key.Matches(msg, p.keys.Prev):
			return p, p.playPrev()
		case key.Matches(msg, p.keys.VolumeUp):
			p.adjustVolumeByPercent(5)
			return p, nil
		case key.Matches(msg, p.keys.VolumeDown):
			p.adjustVolumeByPercent(-5)
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

			// Enter on track list does nothing (dedicated to menu operation elsewhere).
			// Playback is triggered via Space/P.
			return p, cmd
		}

		switch active {
		case app.HomeTab, app.LibraryTab:
			p.recentlyAddedList, cmd = p.recentlyAddedList.Update(msg)
			p.coordinator.SetSelectedAlbum(p.recentlyAddedList.Index())

			// If selection changed, fetch tracks for the selected album in the background
			if item, ok := p.recentlyAddedList.SelectedItem().(util.AlbumItem); ok {
				newIdx := p.recentlyAddedList.Index()
				if newIdx != p.lastSelectedAlbumIndex && p.libSvc != nil {
					p.lastSelectedAlbumIndex = newIdx
					// On selection change prefetch tracks in the background.
					// The UI should only open the track list on Enter.
					cmd = tea.Batch(cmd, p.fetchTracksCmd(item.Album.Key))
				}
			}

			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.recentlyAddedList.SelectedItem().(util.AlbumItem); ok {
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
			if item, ok := p.playlistList.SelectedItem().(util.PlaylistItem); ok {
				newIdx := p.playlistList.Index()
				if newIdx != p.lastSelectedPlaylistIndex && p.libSvc != nil {
					p.lastSelectedPlaylistIndex = newIdx
					// On selection change prefetch tracks in the background.
					// The UI should only open the track list on Enter.
					cmd = tea.Batch(cmd, p.fetchTracksCmd(item.Playlist.Key))
				}
			}

			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.playlistList.SelectedItem().(util.PlaylistItem); ok {
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
				if item, ok := p.queueList.SelectedItem().(util.QueueItem); ok {
					at := util.DomainTrackToApp(&item.Track)
					return p, p.playAppTrack(at)
				}
			}
		}

		return p, cmd
	}
	return p, nil
}

// View renders the library page using a tabbed layout. It includes a nav pane,
// main content pane, and a detail/now-playing pane. When the queue is visible,
// a modal overlay is displayed.
func (p *LibraryPage) View() string {
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

	// Split the main layout into two panes: left controlled by tabs (contentWidth)
	// and right showing the Now Playing (detailWidth). Ensure both fit into the
	// available width, adjusting when necessary.

	// Calculate widths to fill available space
	// We want roughly 40% left, 60% right, or 50/50?
	// The previous logic used fixed percentages in views.go which might leave gaps.
	// Let's use a simpler split: 40% left, 60% right of usable width.
	usableWidth := p.width - 4 // 2 chars padding/border on each side roughly
	leftWidth := usableWidth * 40 / 100
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
	p.recentlyAddedList.SetSize(leftWidth, listHeight)
	p.playlistList.SetSize(leftWidth, listHeight)
	p.trackList.SetSize(leftWidth, listHeight)
	p.queueList.SetSize(leftWidth, listHeight)

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
	var mainContent string
	mainContent = p.nowPlaying.Render(rightWidth, contentHeight)
	if p.libSvc != nil && len(p.coordinator.Albums()) == 0 && len(p.coordinator.Playlists()) == 0 {
		// Show a friendly, centered loading placeholder in the content area when
		// the library service is active but no content has been loaded yet.
		mainContent = lipgloss.JoinVertical(lipgloss.Center, styles.BlurredStyle.Render("Loading library..."))
	}
	// Adjust height to accommodate help view at the bottom (approx 2 lines)
	// contentHeight calculated above

	// Force height on the pane style to ensure borders extend fully
	contentPane := styles.PaneStyle(rightWidth, contentHeight).Height(contentHeight).Render(mainContent)

	leftContentHeight := contentHeight
	leftPane := styles.PaneStyle(leftWidth, leftContentHeight).Height(leftContentHeight).Render(leftContent)

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
			func() string {
				return p.nowPlaying.RenderFull(p.width, p.height)
			}(),
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
		styles.TitleStyle.Render(pageTitle),
		statusLine,
		layout, // The panes
	)

	if notifStr != "" {
		mainLayout = lipgloss.JoinVertical(lipgloss.Left,
			styles.TitleStyle.Render(pageTitle),
			notifStr,
			statusLine,
			layout,
		)
	}

	// Place the main layout in the available space
	finalView := lipgloss.JoinVertical(lipgloss.Left,
		mainLayout,
		"",
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

// Close cancels any subscriptions and releases resources used by the page.
func (p *LibraryPage) Close() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.libSvc != nil {
		_ = p.libSvc.Close()
	}
}

// ---- Helpers ----

// subscribeToLibraryEvents returns a command that listens for library events
// and returns them as tea messages (LibraryEvent) for processing in Update.
func (p *LibraryPage) subscribeToLibraryEvents() tea.Cmd {
	if p.libEvtCh == nil {
		return nil
	}
	return func() tea.Msg {
		for ev := range p.libEvtCh {
			// Forward both success and error events for this page
			switch ev.Type {
			case "recently_added.loaded", "playlists.loaded", "tracks.loaded", "albums.loaded",
				"recently_added.fetch_failed", "playlists.fetch_failed", "tracks.fetch_failed", "albums.fetch_failed",
				"libraries.loaded", "libraries.fetch_failed":
				return ev.Payload
			default:
				continue
			}
		}
		return nil
	}
}

// and returns them as tea messages for processing in Update.
func (p *LibraryPage) subscribeToPlaybackEvents() tea.Cmd {
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
func (p *LibraryPage) subscribeToAuthEvents() tea.Cmd {
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
func (p *LibraryPage) fetchLibraries() tea.Cmd {
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
func (p *LibraryPage) fetchRecentlyAdded() tea.Cmd {
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

// Note: playback orchestration is done via the orchestrator; helper removed.

// fetchPlaylists triggers the library service to fetch playlists.
func (p *LibraryPage) fetchPlaylists() tea.Cmd {
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
func (p *LibraryPage) fetchTracksCmd(key string) tea.Cmd {
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

type CoverArtLoadedMsg struct {
	Image image.Image
	Path  string
}

func (p *LibraryPage) fetchCoverArtCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if p.libSvc == nil {
			return nil
		}
		img, err := p.libSvc.FetchImage(p.ctx, path)
		if err != nil {
			log.Error("failed to fetch cover art", "path", path, "err", err)
			return nil
		}
		return CoverArtLoadedMsg{Image: img, Path: path}
	}
}

// renderRecentlyAdded displays the current recently-added albums list.
func (p *LibraryPage) renderRecentlyAdded(width int) string {
	p.recentlyAddedList.SetWidth(width)
	return p.recentlyAddedList.View()
}

// renderPlaylists displays the playlists list.
func (p *LibraryPage) renderPlaylists(width int) string {
	p.playlistList.SetWidth(width)
	return p.playlistList.View()
}

// renderQueue displays the queued tracks list.
func (p *LibraryPage) renderQueue(width int) string {
	p.queueList.SetWidth(width)
	return p.queueList.View()
}

// renderSearch displays the search input and inline results in the left pane.
func (p *LibraryPage) renderSearch(width int) string {
	title := styles.TitleStyle.Render("Search")
	results := []string{}
	term := strings.TrimSpace(p.searchInput.Value())
	if term != "" {
		q := strings.ToLower(term)
		seen := make(map[string]bool)
		// Search albums
		for _, a := range p.coordinator.Albums() {
			if strings.Contains(strings.ToLower(a.Title), q) || strings.Contains(strings.ToLower(a.Artist), q) {
				s := fmt.Sprintf("%s — %s", a.Title, a.Artist)
				if !seen[s] {
					results = append(results, s)
					seen[s] = true
				}
			}
		}
		// Search playlists
		for _, pl := range p.coordinator.Playlists() {
			if strings.Contains(strings.ToLower(pl.Title), q) {
				s := fmt.Sprintf("%s (playlist)", pl.Title)
				if !seen[s] {
					results = append(results, s)
					seen[s] = true
				}
			}
		}
		// Search tracks
		for _, t := range p.coordinator.Tracks() {
			if strings.Contains(strings.ToLower(t.Title), q) || strings.Contains(strings.ToLower(t.Artist), q) || strings.Contains(strings.ToLower(t.Album), q) {
				s := fmt.Sprintf("%s — %s (track)", t.Title, t.Artist)
				if !seen[s] {
					results = append(results, s)
					seen[s] = true
				}
			}
		}
	}
	if len(results) == 0 {
		results = append(results, styles.BlurredStyle.Render("No matches"))
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, "", p.searchInput.View(), "", lipgloss.JoinVertical(lipgloss.Left, results...))
}

// renderSettings displays a simple settings placeholder in the left pane.
func (p *LibraryPage) renderSettings(width int) string {
	title := styles.TitleStyle.Render("Settings")
	lines := []string{
		styles.BlurredStyle.Render("No settings available yet."),
		styles.BlurredStyle.Render("Press Esc to close."),
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, "", lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// renderTracks displays the currently selected tracks in the left pane.
func (p *LibraryPage) renderTracks(width int) string {
	p.trackList.SetWidth(width)
	return p.trackList.View()
}

// renderNowPlaying shows the now playing details, a small cover-art placeholder,
// playback progress and volume controls.

// renderWithModal composes the base view layout with the queue modal overlay.
func (p *LibraryPage) renderWithModal(base string) string {
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

// Helper: play the provided app.Track (UI & playback service)
func (p *LibraryPage) playAppTrack(at *app.Track) tea.Cmd {
	if at == nil {
		return nil
	}
	log.Debug("playAppTrack.called", "title", at.Title)
	// Update UI coordinator state preemptively for immediate UI feedback — playback.started will reconcile state later
	p.coordinator.SetCurrentTrack(at)
	p.coordinator.SetPlaybackState(app.PlaybackPlaying)

	var cmds []tea.Cmd

	// Fetch cover art if available
	if at.Thumb != "" {
		cmds = append(cmds, p.fetchCoverArtCmd(at.Thumb))
	}

	// Orchestrator is required to perform playback orchestration
	if p.orchestrator != nil {
		if err := p.orchestrator.PlayAppTrack(p.ctx, at); err != nil {
			p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", err), "error", 10*time.Second)
			return nil
		}
	} else {
		// Orchestrator missing: notify user
		p.coordinator.SetNotification("Play failed: playback orchestrator unavailable", "error", 10*time.Second)
		return nil
	}
	return tea.Batch(cmds...)
}

// Helper: play next track (queue preferred, otherwise tracklist)
func (p *LibraryPage) playNext() tea.Cmd {
	q := p.coordinator.Queue()
	tracks := p.coordinator.Tracks()
	// Convert app.Track slices into domain.Track slices for the playback controller
	dq := make([]domain.Track, len(q))
	for i, t := range q {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			dq[i] = *dt
		}
	}
	dtracks := make([]domain.Track, len(tracks))
	for i, t := range tracks {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			dtracks[i] = *dt
		}
	}
	pc := service.NewPlaybackController(p.orchestrator)
	if p.orchestrator == nil {
		p.coordinator.SetNotification("Play failed: playback orchestrator unavailable", "error", 10*time.Second)
		return nil
	}
	if err := p.orchestrator.PlayNext(p.ctx, pc, q, p.coordinator.QueueIndex(), tracks, p.coordinator.SelectedTrack()); err != nil {
		log.Error("playback play failed", "err", err)
		p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", err), "error", 10*time.Second)
	}
	return nil
}

// Helper: play previous track (queue preferred, otherwise tracklist)
func (p *LibraryPage) playPrev() tea.Cmd {
	q := p.coordinator.Queue()
	tracks := p.coordinator.Tracks()
	dq := make([]domain.Track, len(q))
	for i, t := range q {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			dq[i] = *dt
		}
	}
	dtracks := make([]domain.Track, len(tracks))
	for i, t := range tracks {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			dtracks[i] = *dt
		}
	}
	pc := service.NewPlaybackController(p.orchestrator)
	if p.orchestrator == nil {
		p.coordinator.SetNotification("Play failed: playback orchestrator unavailable", "error", 10*time.Second)
		return nil
	}
	if err := p.orchestrator.PlayPrev(p.ctx, pc, q, p.coordinator.QueueIndex(), tracks, p.coordinator.SelectedTrack()); err != nil {
		p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", err), "error", 10*time.Second)
	}
	return nil
}

// Helper: adjust volume by percentage (linear stepping for consistent feel)
// percentageDelta is in percentage points (e.g., 5 means +5% or -5%)
func (p *LibraryPage) adjustVolumeByPercent(percentageDelta int) {
	// Prefer orchestrator where available to centralize pb operations
	if p.orchestrator != nil {
		_ = p.orchestrator.AdjustVolumeByPercent(percentageDelta)
		return
	}
	// Orchestrator missing: notify user of missing playback orchestrator
	p.coordinator.SetNotification("Volume adjust failed: playback orchestrator unavailable", "error", 5*time.Second)
	// No coordinator fallback in orchestrator-only mode
}
