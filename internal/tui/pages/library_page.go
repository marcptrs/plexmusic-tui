package pages

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
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

const retroLogo = `
██████╗ ██╗     ███████╗██╗  ██╗    ███╗   ███╗██╗   ██╗███████╗██╗ ██████╗ 
██╔══██╗██║     ██╔════╝╚██╗██╔╝    ████╗ ████║██║   ██║██╔════╝██║██╔════╝ 
██████╔╝██║     █████╗   ╚███╔╝     ██╔████╔██║██║   ██║███████╗██║██║      
██╔═══╝ ██║     ██╔══╝   ██╔██╗     ██║╚██╔╝██║██║   ██║╚════██║██║██║      
██║     ███████╗███████╗██╔╝ ██╗    ██║ ╚═╝ ██║╚██████╔╝███████║██║╚██████╗ 
╚═╝     ╚══════╝╚══════╝╚═╝  ╚═╝    ╚═╝     ╚═╝ ╚═════╝ ╚══════╝╚═╝ ╚═════╝ 
`

const retroLogoVertical = `
██████╗ ██╗     ███████╗██╗  ██╗
██╔══██╗██║     ██╔════╝╚██╗██╔╝
██████╔╝██║     █████╗   ╚███╔╝ 
██╔═══╝ ██║     ██╔══╝   ██╔██╗ 
██║     ███████╗███████╗██╔╝ ██╗
╚═╝     ╚══════╝╚══════╝╚═╝  ╚═╝

███╗   ███╗██╗   ██╗███████╗██╗ ██████╗ 
████╗ ████║██║   ██║██╔════╝██║██╔════╝ 
██╔████╔██║██║   ██║███████╗██║██║      
██║╚██╔╝██║██║   ██║╚════██║██║██║      
██║ ╚═╝ ██║╚██████╔╝███████║██║╚██████╗ 
╚═╝     ╚═╝ ╚═════╝ ╚══════╝╚═╝ ╚═════╝ 
`

// Helper to detect rune keypresses in a KeyMsg when the bubbles/list key mapping
// doesn't directly express a specific rune. Useful for supporting both "k"/"j"
// navigation as rune fallbacks where clients send runes instead of key events.
func isRuneKey(msg tea.KeyMsg, r rune) bool {
	if len(msg.Runes) == 0 {
		return false
	}
	// Only accept a single rune to prevent conflicts with multi-rune sequences
	return len(msg.Runes) == 1 && msg.Runes[0] == r
}

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
	settingsList      list.Model

	// Search and inline content UI
	searchInput  textinput.Model
	searchActive bool
	searchTerm   string

	// showingTracks indicates the left-pane is showing track list for a selected album/playlist.
	showingTracks bool
	// drawerOpen indicates whether an overlay drawer (for library/search/settings) is open
	drawerOpen bool

	// Stats loading state
	loadingStats bool
	spinner      spinner.Model

	// Now Playing component
	nowPlaying   *components.NowPlayingComponent
	orchestrator *tui.Orchestrator

	// State tracking for selection changes
	lastSelectedAlbumIndex    int
	lastSelectedPlaylistIndex int

	// Help model
	help help.Model

	// Key bindings
	keys tui.LibraryKeyMap

	// Playback finished trigger
	finishedTriggered bool

	focusedNowPlaying bool
	focusedQueue      bool
}

// NewLibraryPage creates a library page and its cancellable event context.
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

	// Create a separate delegate for the settings list so we can style it.
	settingsDelegate := list.NewDefaultDelegate()
	settingsList := list.New(nil, settingsDelegate, 0, 0)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	p := &LibraryPage{
		coordinator:   coord,
		ctx:           ctx,
		cancel:        cancel,
		authSvc:       authSvc,
		searchActive:  false,
		searchTerm:    "",
		showingTracks: false,
		drawerOpen:    false,
		loadingStats:  false,
		spinner:       s,
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
		settingsList:              settingsList,
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
	// Initialize settings list items from available config
	if p.coordinator != nil && p.coordinator.ConfigManager() != nil {
		pos := p.coordinator.ConfigManager().GetCoverArtPosition()
		// Build the settings list items (grouped)
		items := []list.Item{}
		items = append(items, util.SettingsItem{Group: "Layout", Name: "Cover art position", Key: "coverArtPos", Kind: "choice", Value: pos})
		p.settingsList.SetItems(items)
	}
	return p
}

// Init sets up services and triggers initial data fetches for the page.
func (p *LibraryPage) Init() tea.Cmd {
	server := p.coordinator.GetCurrentServer()
	// Prefer server-specific token; otherwise fall back to coordinator token.
	token := ""
	if server != nil && server.AccessToken != "" {
		token = server.AccessToken
	} else {
		token = p.coordinator.GetToken()
	}

	// Only initialize services when a server and token exist.
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
	// Initialize settings list items based on current config.
	if p.coordinator != nil && p.coordinator.ConfigManager() != nil {
		pos := p.coordinator.ConfigManager().GetCoverArtPosition()
		items := []list.Item{}
		items = append(items, util.SettingsItem{Group: "Layout", Name: "Cover art position", Key: "coverArtPos", Kind: "choice", Value: pos, DescriptionText: "Position of cover art within the layout: left or right"})
		p.settingsList.SetItems(items)
	}
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

		// Resize lists and compute heights consistent with View().
		usableWidth := p.width - 4
		leftWidth := usableWidth * 40 / 100
		if leftWidth < 30 {
			leftWidth = 30
		}

		// Compute content height considering header/footer/help and pane borders.

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
		// Update coordinator based on library events
		switch msg.Type {
		case "libraries.loaded":
			log.Debug("LibraryPage: libraries.loaded", "count", len(msg.Libraries))
			appLibs := make([]app.MusicLibrary, len(msg.Libraries))
			for i, l := range msg.Libraries {
				appLibs[i] = app.MusicLibrary{Key: l.Key, Title: l.Title, Type: l.Type}
			}
			p.coordinator.SetLibraries(appLibs)
			if len(appLibs) > 0 {
				p.coordinator.SetSelectedLibrary(0)
				// Trigger stats fetch now that we have libraries
				p.loadingStats = true
				return p, tea.Batch(p.subscribeToLibraryEvents(), p.subscribeToPlaybackEvents(), p.fetchLibraryStats(), p.spinner.Tick)
			} else {
				log.Warn("LibraryPage: No libraries found, stats fetch skipped")
			}
		case "recently_added.loaded":
			// Convert domain.Album to app.Album and update the coordinator
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
			// Note: msg.TotalSize here is the count of recently added items (e.g. 50),
			// not the total albums in the library. We should not overwrite the library stats.
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
			if msg.TotalSize > 0 {
				p.coordinator.SetPlaylistsTotal(msg.TotalSize)
			} else {
				p.coordinator.SetPlaylistsTotal(len(appPlaylists))
			}
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
			if msg.TotalSize > 0 {
				p.coordinator.SetTracksTotal(msg.TotalSize)
			} else {
				p.coordinator.SetTracksTotal(len(appTracks))
			}
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
		// Re-subscribe to continue receiving library/playback events
		return p, tea.Batch(p.subscribeToLibraryEvents(), p.subscribeToPlaybackEvents())

	case service.AuthEvent:
		// Handle auth events (servers.loaded) to create libSvc and fetch libraries.
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
		// Update coordinator from playback events for UI reflection.
		switch msg.Type {
		case "playback.load_failed":
			if msg.Error != nil {
				p.coordinator.SetNotification(fmt.Sprintf("Load failed: %v", msg.Error), "error", 10*time.Second)
				log.Debug("LibraryPage: set load_failed notification", "err", msg.Error)
			} else {
				p.coordinator.SetNotification("Load failed", "error", 10*time.Second)
				log.Debug("LibraryPage: set load_failed notification", "no err")
			}
		case "playback.play_failed":
			if msg.Error != nil {
				p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", msg.Error), "error", 10*time.Second)
			} else {
				p.coordinator.SetNotification("Play failed", "error", 10*time.Second)
			}
		case "playback.started":
			p.coordinator.SetPlaybackState(app.PlaybackPlaying)
			// Reset our finished-triggered debounce to allow auto-advance for the next
			// track when it completes.
			p.finishedTriggered = false
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

			// Detect end-of-track and auto-advance to the next queued track if applicable.
			// We debounce using `finishedTriggered` to avoid issuing multiple commands for
			// the same track-end event as position updates can be frequent.
			if msg.Length > 0 && msg.Position >= msg.Length {
				if p.coordinator.IsPlaying() && !p.finishedTriggered {
					p.finishedTriggered = true
					// Trigger next; playNext handles the queue logic and will stop playback
					// when the queue is complete.
					return p, p.playNext()
				}
			} else {
				// Reset the debounce when position is before the end (new track started, resumed, or seeked).
				p.finishedTriggered = false
			}
		}
		// Re-subscribe to continue receiving playback/library events
		return p, tea.Batch(p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
	// animation messages removed - no-op

	case spinner.TickMsg:
		if p.loadingStats {
			var cmd tea.Cmd
			p.spinner, cmd = p.spinner.Update(msg)
			return p, cmd
		}

	case LibraryStatsMsg:
		p.loadingStats = false
		p.coordinator.SetArtistsTotal(msg.Artists)
		p.coordinator.SetAlbumsTotal(msg.Albums)
		p.coordinator.SetTracksTotal(msg.Tracks)
		return p, nil

	case CoverArtLoadedMsg:
		// Dump before/after views to assist in debugging VSCode terminal rendering
		p.dumpPageView("before_art_load")
		p.coordinator.SetPlaybackAlbumArt(msg.Image, msg.Path)
		p.dumpPageView("after_art_load")
		return p, nil

	case tea.KeyMsg:
		// Early interception: route Up/Down (including k/j) to the queue handler if
		// the queue is explicitly in scope (modal visible OR explicit queue focus OR visible in layout).
		// This takes precedence over other list handlers so Up/Down will scroll the queue.
		if (key.Matches(msg, p.keys.Up) || key.Matches(msg, p.keys.Down) || isRuneKey(msg, 'k') || isRuneKey(msg, 'j')) && p.coordinator != nil {
			if p.coordinator.ShowQueueModal() || p.IsFocusedQueue() || p.isQueueVisible() {
				if len(p.coordinator.Queue()) > 0 {
					// If we are now routing Up/Down to the queue, prefer to mark the queue
					// as focused so subsequent logic skips updating the left-side lists.
					p.SetFocusedQueue(true)
					return p, p.handleQueueKeyMsg(msg)
				}
			}
		}

		// Let the search input handle keys while on Search tab.
		if p.coordinator.ActiveTab() == app.SearchTab {
			switch msg.String() {
			case "1", "2", "3", "4", "5", "6":
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

		// Page-level key handling.
		var cmd tea.Cmd

		// Check global keys first
		switch {
		case p.coordinator != nil && (p.coordinator.ShowQueueModal() || p.IsFocusedQueue() || p.isQueueVisible()) && len(p.coordinator.Queue()) > 0 && (key.Matches(msg, p.keys.Up) || key.Matches(msg, p.keys.Down) || key.Matches(msg, p.keys.PlaySelected) || key.Matches(msg, p.keys.QueueMoveUp) || key.Matches(msg, p.keys.QueueMoveDown) || key.Matches(msg, p.keys.QueueRemove) || key.Matches(msg, p.keys.Enter)):
			// When the queue modal is visible, or when the queue has explicit keyboard focus,
			// or when the queue is visible as a pane, route queue-specific keys to the unified handler.
			// This overrides other lists when the queue has explicit focus or is visible to the user.
			return p, p.handleQueueKeyMsg(msg)
		case p.coordinator != nil && p.coordinator.ShowQueueModal() && key.Matches(msg, p.keys.Back):
			// Close queue modal on 'Back' key when visible
			p.coordinator.SetShowQueueModal(false)
			p.SetFocusedQueue(false)
			return p, nil
		case key.Matches(msg, p.keys.Back):
			// If the queue modal is open, close it first; otherwise go back to server selection.
			if p.coordinator.ShowQueueModal() {
				p.coordinator.SetShowQueueModal(false)
				return p, nil
			}
			// Close left-pane drawer or tracklist if open instead of closing the page.
			if p.drawerOpen {
				p.drawerOpen = false
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

			// Toggle playback if already active.
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

			// If the Queue tab is active and the user presses space, treat it as
			// a "play selected" from the queue: remove previous entries up to the
			// selected item and begin playing that track.
			active := p.coordinator.ActiveTab()
			if active == app.QueueTab {
				sel := p.queueList.Index()
				q := p.coordinator.Queue()
				if len(q) == 0 || sel < 0 || sel >= len(q) {
					return p, nil
				}
				// If selected is not the first item, trim the queue to the selected item and onward
				if sel > 0 {
					newQueue := make([]app.Track, len(q)-sel)
					copy(newQueue, q[sel:])
					p.coordinator.SetQueue(newQueue)
					p.coordinator.SetQueueIndex(0)
					p.updateQueueListFromCoordinator()
					p.queueList.Select(0)
					return p, p.playAppTrack(&newQueue[0])
				}
				// Selected is the first item — just play it
				if item, ok := p.queueList.SelectedItem().(util.QueueItem); ok {
					at := util.DomainTrackToApp(&item.Track)
					return p, p.playAppTrack(at)
				}
				return p, nil
			}

			// Try to play the selected/first track from album, playlist or queue.
			if p.showingTracks {
				if item, ok := p.trackList.SelectedItem().(util.TrackItem); ok {
					// Convert domain.Track -> app.Track defensively using the selected item
					at := util.DomainTrackToApp(&item.Track)

					// If the user chooses to play a track while viewing an album/playlist,
					// build a queue from the selected track to the end of the list so the
					// subsequent tracks will be played automatically.
					tracks := p.coordinator.Tracks()
					selIdx := p.trackList.Index()
					if len(tracks) == 0 {
						return p, nil
					}
					if selIdx < 0 || selIdx >= len(tracks) {
						selIdx = 0
					}
					newQueue := make([]app.Track, len(tracks)-selIdx)
					copy(newQueue, tracks[selIdx:])

					// Ensure the first queued track matches the selected item (if convert succeeded)
					if at != nil && len(newQueue) > 0 {
						newQueue[0] = *at
					}

					// Persist queue & selection to coordinator
					p.coordinator.SetQueue(newQueue)
					p.coordinator.SetQueueIndex(0)
					p.updateQueueListFromCoordinator()

					// Make sure the underlying tracklist selection is reflected in coordinator
					p.coordinator.SetSelectedTrack(selIdx)
					p.showingTracks = false
					p.coordinator.SetActiveTab(app.QueueTab)

					// Play the first track in the newly created queue (the selected track)
					if len(newQueue) > 0 {
						return p, p.playAppTrack(&newQueue[0])
					}
				}
				return p, nil
			}

			active = p.coordinator.ActiveTab()
			if p.libSvc != nil && (active == app.PlaylistsTab || active == app.HomeTab || active == app.LibraryTab) {
				if active == app.PlaylistsTab {
					if item, ok := p.playlistList.SelectedItem().(util.PlaylistItem); ok {
						reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
						defer cancel()
						// Fetch tracks for the playlist and set them as the queue.
						tracks, _, _ := p.libSvc.FetchTracks(reqCtx, item.Playlist.Key)
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
							p.updateQueueListFromCoordinator()
							p.coordinator.SetTracks(appTracks)
							p.trackList.SetItems(items)
							p.trackList.Select(0)
							p.coordinator.SetSelectedTrack(0)
							p.showingTracks = false
							p.coordinator.SetActiveTab(app.QueueTab)
							return p, p.playAppTrack(&appTracks[0])
						}
					}
				} else {
					// Home or Library tab: use selected album
					if item, ok := p.recentlyAddedList.SelectedItem().(util.AlbumItem); ok {
						reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
						defer cancel()
						tracks, _, _ := p.libSvc.FetchTracks(reqCtx, item.Album.Key)
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
							p.updateQueueListFromCoordinator()
							p.coordinator.SetTracks(appTracks)
							p.trackList.SetItems(items)
							p.trackList.Select(0)
							p.coordinator.SetSelectedTrack(0)
							p.showingTracks = false
							p.coordinator.SetActiveTab(app.QueueTab)
							return p, p.playAppTrack(&appTracks[0])
						}
					}
				}
				return p, nil
			}

			// Fallback: play selected track from Tracks or Queue.
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
		case key.Matches(msg, p.keys.Queue) || isRuneKey(msg, 'o') || isRuneKey(msg, 'O'):
			// If the modal is visible, close it and clear focus
			if p.coordinator.ShowQueueModal() {
				p.coordinator.SetShowQueueModal(false)
				p.SetFocusedQueue(false)
				return p, nil
			}
			// If the active tab is the Queue tab, toggle queue focus instead of opening modal.
			// This allows Up/Down to be routed to the queue lists when focused.
			if p.coordinator.ActiveTab() == app.QueueTab {
				p.SetFocusedQueue(!p.IsFocusedQueue())
				if p.IsFocusedQueue() {
					p.focusedNowPlaying = false // don't keep both focused states set
				}
				return p, nil
			}
			// Otherwise, open/close the queue modal and set focus in a consistent manner.
			showing := !p.coordinator.ShowQueueModal()
			p.coordinator.SetShowQueueModal(showing)
			p.SetFocusedQueue(showing)
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
			sw := msg.String()
			switch sw {
			case "1":
				p.coordinator.SetActiveTab(app.HomeTab)
			case "2":
				p.coordinator.SetActiveTab(app.LibraryTab)
			case "3":
				p.coordinator.SetActiveTab(app.PlaylistsTab)
			case "4":
				p.coordinator.SetActiveTab(app.SearchTab)
			case "5":
				p.coordinator.SetActiveTab(app.QueueTab)
			case "6":
				p.coordinator.SetActiveTab(app.SettingsTab)
			}
			// For browsing-focused tabs, open the drawer; queue tab should keep drawer closed
			switch sw {
			case "1", "2", "3", "4", "6":
				p.drawerOpen = true
			case "5":
				p.drawerOpen = false
			}
			// Do not automatically change keyboard focus when switching tabs.
			// Explicit 'o' keypress or the queue modal should be used to toggle focus.
			if p.showingTracks {
				p.showingTracks = false
			}
			return p, nil
		}

		// Delegate to the active list
		active := p.coordinator.ActiveTab()

		// In Settings tab, handle quick shortcuts (e.g., 'c' toggles cover-art side).
		if active == app.SettingsTab {
			if km := msg; true {
				switch km.String() {
				case "c":
					if p.coordinator != nil && p.coordinator.ConfigManager() != nil {
						cur := p.coordinator.ConfigManager().GetCoverArtPosition()
						if cur == "left" {
							p.coordinator.ConfigManager().SetCoverArtPosition("right")
						} else {
							p.coordinator.ConfigManager().SetCoverArtPosition("left")
						}
						_ = p.coordinator.ConfigManager().Save()
						p.coordinator.SetNotification("Cover art position updated", "success", 3*time.Second)
					}
					return p, nil
				}
			}
		}

		if p.showingTracks {
			if !p.IsFocusedQueue() && !p.coordinator.ShowQueueModal() {
				p.trackList, cmd = p.trackList.Update(msg)
				p.coordinator.SetSelectedTrack(p.trackList.Index())

				// Enter on track list does nothing (dedicated to menu operation elsewhere).
				// Playback is triggered via Space/P.
				return p, cmd
			}
			// Queue is focused, ignore tracklist nav keys
			return p, nil
		}

		switch active {
		case app.HomeTab, app.LibraryTab:
			if !p.IsFocusedQueue() && !p.coordinator.ShowQueueModal() {
				p.recentlyAddedList, cmd = p.recentlyAddedList.Update(msg)
				p.coordinator.SetSelectedAlbum(p.recentlyAddedList.Index())

				// If selection changed, fetch tracks for the selected album in the background
				if item, ok := p.recentlyAddedList.SelectedItem().(util.AlbumItem); ok {
					newIdx := p.recentlyAddedList.Index()
					if newIdx != p.lastSelectedAlbumIndex && p.libSvc != nil {
						p.lastSelectedAlbumIndex = newIdx
						cmd = tea.Batch(cmd, p.fetchTracksCmd(item.Album.Key))
					}
				}
			}

			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.recentlyAddedList.SelectedItem().(util.AlbumItem); ok {
					if p.libSvc != nil {
						reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
						defer cancel()
						_, _, _ = p.libSvc.FetchTracks(reqCtx, item.Album.Key)
						p.showingTracks = true
					}
				}
			}

		case app.PlaylistsTab:
			if !p.IsFocusedQueue() && !p.coordinator.ShowQueueModal() {
				p.playlistList, cmd = p.playlistList.Update(msg)
				p.coordinator.SetSelectedPlaylist(p.playlistList.Index())

				if item, ok := p.playlistList.SelectedItem().(util.PlaylistItem); ok {
					newIdx := p.playlistList.Index()
					if newIdx != p.lastSelectedPlaylistIndex && p.libSvc != nil {
						p.lastSelectedPlaylistIndex = newIdx
						cmd = tea.Batch(cmd, p.fetchTracksCmd(item.Playlist.Key))
					}
				}
			}

			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.playlistList.SelectedItem().(util.PlaylistItem); ok {
					if p.libSvc != nil {
						reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
						defer cancel()
						_, _, _ = p.libSvc.FetchTracks(reqCtx, item.Playlist.Key)
						p.showingTracks = true
					}
				}
			}

		case app.QueueTab:
			p.queueList, cmd = p.queueList.Update(msg)
			p.coordinator.SetQueueIndex(p.queueList.Index())

			// Move selected queue item up (swap it with previous item)
			if key.Matches(msg, p.keys.QueueMoveUp) {
				sel := p.queueList.Index()
				q := p.coordinator.Queue()
				if sel > 0 && sel < len(q) {
					playIdx := p.coordinator.QueueIndex()
					// Swap the queue elements
					q[sel-1], q[sel] = q[sel], q[sel-1]
					// Update playing index mapping if necessary
					switch playIdx {
					case sel:
						playIdx = sel - 1
					case sel - 1:
						playIdx = sel
					}
					p.coordinator.SetQueue(q)
					p.coordinator.SetQueueIndex(playIdx)
					p.updateQueueListFromCoordinator()
					if playIdx >= 0 && playIdx < len(q) {
						p.queueList.Select(playIdx)
					} else {
						p.queueList.Select(sel - 1)
					}
				}
				return p, nil
			}

			// Move selected queue item down (swap it with next item)
			if key.Matches(msg, p.keys.QueueMoveDown) {
				sel := p.queueList.Index()
				q := p.coordinator.Queue()
				if sel >= 0 && sel < len(q)-1 {
					playIdx := p.coordinator.QueueIndex()
					// Swap the queue elements
					q[sel], q[sel+1] = q[sel+1], q[sel]
					// Update playing index mapping if necessary
					switch playIdx {
					case sel:
						playIdx = sel + 1
					case sel + 1:
						playIdx = sel
					}
					p.coordinator.SetQueue(q)
					p.coordinator.SetQueueIndex(playIdx)
					p.updateQueueListFromCoordinator()
					if playIdx >= 0 && playIdx < len(q) {
						p.queueList.Select(playIdx)
					} else {
						p.queueList.Select(sel + 1)
					}
				}
				return p, nil
			}

			// Remove selected queue item
			if key.Matches(msg, p.keys.QueueRemove) {
				sel := p.queueList.Index()
				q := p.coordinator.Queue()
				if sel >= 0 && sel < len(q) {
					playIdx := p.coordinator.QueueIndex()
					newQ := make([]app.Track, 0, len(q)-1)
					newQ = append(newQ, q[:sel]...)
					newQ = append(newQ, q[sel+1:]...)
					// Adjust playing index mapping
					if playIdx == sel {
						// We removed the currently playing queued element; stop playback and clear current track/index
						if p.orchestrator != nil {
							_ = p.orchestrator.Stop()
						} else {
							p.coordinator.SetPlaybackState(app.PlaybackStopped)
						}
						p.coordinator.SetCurrentTrack(nil)
						playIdx = -1
					} else if playIdx > sel {
						playIdx = playIdx - 1
					}
					p.coordinator.SetQueue(newQ)
					p.coordinator.SetQueueIndex(playIdx)
					p.updateQueueListFromCoordinator()
					// Adjust list selection to a valid index
					if playIdx >= 0 && playIdx < len(newQ) {
						p.queueList.Select(playIdx)
					} else if sel < len(newQ) {
						p.queueList.Select(sel)
					} else if len(newQ) > 0 {
						p.queueList.Select(len(newQ) - 1)
					}
				}
				return p, nil
			}

			// Play selected queue item — when playing a selected track within the queue,
			// remove all previous tracks so the new selected track becomes the first queued item.
			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.queueList.SelectedItem().(util.QueueItem); ok {
					sel := p.queueList.Index()
					q := p.coordinator.Queue()
					if len(q) == 0 || sel < 0 || sel >= len(q) {
						return p, nil
					}
					if sel > 0 {
						newQ := make([]app.Track, len(q)-sel)
						copy(newQ, q[sel:])
						p.coordinator.SetQueue(newQ)
						p.coordinator.SetQueueIndex(0)
						p.updateQueueListFromCoordinator()
						p.queueList.Select(0)
						return p, p.playAppTrack(&newQ[0])
					}
					at := util.DomainTrackToApp(&item.Track)
					return p, p.playAppTrack(at)
				}
			}
		case app.SettingsTab:
			p.settingsList, cmd = p.settingsList.Update(msg)
			// Handle Enter to toggle boolean/choice settings
			if key.Matches(msg, p.keys.Enter) {
				if item, ok := p.settingsList.SelectedItem().(util.SettingsItem); ok {
					switch item.Key {
					case "coverArtPos":
						// Toggle left/right choice
						cur := "left"
						if p.coordinator != nil && p.coordinator.ConfigManager() != nil {
							cur = p.coordinator.ConfigManager().GetCoverArtPosition()
						}
						newVal := "left"
						if cur == "left" {
							newVal = "right"
						}
						if p.coordinator != nil && p.coordinator.ConfigManager() != nil {
							p.coordinator.ConfigManager().SetCoverArtPosition(newVal)
							// persist the change if possible
							_ = p.coordinator.ConfigManager().Save()
						}
						// update the item in the settings list
						items := make([]list.Item, len(p.settingsList.Items()))
						for i, it := range p.settingsList.Items() {
							if s, ok2 := it.(util.SettingsItem); ok2 {
								if s.Key == "coverArtPos" {
									s.Value = newVal
								}
								items[i] = s
							} else {
								items[i] = it
							}
						}
						p.settingsList.SetItems(items)
						p.coordinator.SetNotification("Cover art position updated", "success", 2*time.Second)
					}
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
	p.queueList.SetSize(rightWidth, listHeight)
	var queueContent string
	if len(p.coordinator.Queue()) == 0 {
		// Show retro logo if queue is empty
		// Use vertical logo if space is tight (approx 80 chars needed for full logo)
		logoStr := retroLogo
		if rightWidth < 80 {
			logoStr = retroLogoVertical
		}
		logo := styles.PrimaryTextStyle().Render(logoStr)
		queueContent = lipgloss.Place(rightWidth, listHeight, lipgloss.Center, lipgloss.Center, logo)
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
		leftPane = styles.PaneStyle(leftWidth, leftContentHeight).Height(leftContentHeight).Render(leftContent)
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

		leftPane = styles.PaneStyle(leftWidth, leftContentHeight).Height(leftContentHeight).Render(lipgloss.JoinVertical(lipgloss.Center, artView, infoView))
	}

	// pos already set earlier

	var leftColumn, rightColumn string
	if pos == "right" {
		// Swap roles: left contains the Queue or content (if showingTracks/drawerOpen),
		// right contains the cover art + info.
		if p.drawerOpen || p.showingTracks {
			leftColumn = styles.PaneStyle(leftWidth, leftContentHeight).Height(leftContentHeight).Render(leftContent)
		} else {
			leftColumn = styles.PaneStyle(leftWidth, leftContentHeight).Height(leftContentHeight).Render(queueContent)
		}
		rightColumn = styles.PaneStyle(rightWidth, contentHeight).Height(contentHeight).Render(lipgloss.JoinVertical(lipgloss.Center, artView, infoView))
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
		statusLine = styles.BlurredStyle.Render(fmt.Sprintf("Server: %s • %s Loading stats...", serverName, p.spinner.View()))
	} else {
		statusLine = styles.BlurredStyle.Render(fmt.Sprintf("Server: %s • Artists: %s • Albums: %s • Playlists: %s • Tracks: %s",
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

// dumpPageView writes the raw Page.View to /tmp/plexmusic_view_debug.txt when
// coordinator.DumpView() is enabled; useful for reproducing terminal render quirks.
func (p *LibraryPage) dumpPageView(label string) {
	if !p.coordinator.DumpView() {
		return
	}
	f, err := os.OpenFile("/tmp/plexmusic_view_debug.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "=== %s (%s) ===\n", label, time.Now().Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "%s\n\n", p.View())
}

// Close cancels subscriptions and releases resources.
func (p *LibraryPage) Close() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.libSvc != nil {
		_ = p.libSvc.Close()
	}
}

// ---- Helpers ----

// subscribeToLibraryEvents forwards library events as Tea messages to Update.
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

// and returns them as Tea messages for Update.
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

// subscribeToAuthEvents forwards auth events relevant to server discovery.
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

// fetchLibraries triggers fetching available libraries.
func (p *LibraryPage) fetchLibraries() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		_, _, _ = p.libSvc.FetchLibraries(ctx)
		return nil
	}
}

// fetchRecentlyAdded triggers fetching recently added albums.
func (p *LibraryPage) fetchRecentlyAdded() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		_, _, _ = p.libSvc.FetchRecentlyAdded(ctx)
		return nil
	}
}

// Note: playback orchestration is done via the orchestrator; helper removed.

// fetchPlaylists triggers fetching playlists.
func (p *LibraryPage) fetchPlaylists() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		_, _, _ = p.libSvc.FetchPlaylists(ctx)
		return nil
	}
}

// fetchTracksCmd returns a command to fetch tracks for the given key; UI
// updates are delivered via library events.
func (p *LibraryPage) fetchTracksCmd(key string) tea.Cmd {
	if p.libSvc == nil || key == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		_, _, _ = p.libSvc.FetchTracks(ctx, key)
		return nil
	}
}

// fetchLibraryStats triggers fetching statistics for the current library.
func (p *LibraryPage) fetchLibraryStats() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	// We need to know which library to query.
	// If coordinator has libraries, use the selected one.
	// If not, we might need to wait for libraries.loaded.
	// For now, let's assume we query the first music library if available.

	// Set loading state immediately so UI updates
	p.loadingStats = true

	return tea.Batch(
		p.spinner.Tick,
		func() tea.Msg {
			// This is a bit tricky because we need the library key.
			// We can access coordinator state here safely as it's read-only or thread-safe enough for this.
			// But better to pass the key if we knew it.
			// Let's try to get libraries from coordinator.
			libs := p.coordinator.Libraries()
			if len(libs) == 0 {
				log.Warn("fetchLibraryStats: No libraries available in coordinator")
				return nil
			}
			// Use the first library for now (or selected if we had that concept fully wired)
			// The coordinator sets selectedLibrary to 0 by default.
			idx := p.coordinator.SelectedLibrary()
			if idx < 0 || idx >= len(libs) {
				idx = 0
			}
			key := libs[idx].Key
			log.Debug("fetchLibraryStats: starting", "libraryKey", key)

			ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
			defer cancel()

			artists, albums, tracks, err := p.libSvc.FetchSectionCounts(ctx, key)
			if err != nil {
				log.Error("Failed to fetch library stats", "err", err)
				// Return zero stats to clear loading state
				return LibraryStatsMsg{Artists: 0, Albums: 0, Tracks: 0}
			}
			log.Debug("fetchLibraryStats: success", "artists", artists, "albums", albums, "tracks", tracks)
			return LibraryStatsMsg{Artists: artists, Albums: albums, Tracks: tracks}
		},
	)
}

type LibraryStatsMsg struct {
	Artists int
	Albums  int
	Tracks  int
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
	// Configure the settings list size based on current layout
	contentHeight := p.height - 8
	if contentHeight < 6 {
		contentHeight = 6
	}
	listHeight := contentHeight - 2
	if listHeight < 0 {
		listHeight = 0
	}
	p.settingsList.SetSize(width, listHeight)
	return lipgloss.JoinVertical(lipgloss.Left, title, "", p.settingsList.View())
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

	// Center overlay without extra spacer line
	return lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, base, modalStyled),
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
func (p *LibraryPage) updateQueueListFromCoordinator() {
	q := p.coordinator.Queue()
	qItems := make([]list.Item, len(q))
	for i, t := range q {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			qi := util.QueueItem{Track: *dt, Index: i}
			if p.coordinator.QueueIndex() == i {
				qi.Playing = true
			}
			qItems[i] = qi
		} else {
			qi := util.QueueItem{Track: domain.Track{Title: t.Title, Artist: t.Artist, Album: t.Album, Duration: t.Duration}, Index: i}
			if p.coordinator.QueueIndex() == i {
				qi.Playing = true
			}
			qItems[i] = qi
		}
	}
	p.queueList.SetItems(qItems)
	sel := p.coordinator.QueueIndex()
	if len(q) == 0 {
		// No queue: clear selection
		p.queueList.Select(0)
		return
	}
	if sel < 0 {
		sel = 0
	}
	if sel >= len(q) {
		sel = len(q) - 1
	}
	p.queueList.Select(sel)
}

// IsFocusedQueue returns true if the queue has keyboard focus.
func (p *LibraryPage) IsFocusedQueue() bool {
	return p.focusedQueue
}

// SetFocusedQueue sets whether the queue has keyboard focus.
func (p *LibraryPage) SetFocusedQueue(v bool) {
	p.focusedQueue = v
}

// isQueueVisible returns true if the queue pane or modal is visible to the user.
// It accounts for the modal state, the active Queue tab, and whether the UI
// is showing a queue pane (i.e., no left-pane tracklist or drawer).
func (p *LibraryPage) isQueueVisible() bool {
	if p.coordinator == nil {
		return false
	}
	// Queue modal takes precedence
	if p.coordinator.ShowQueueModal() {
		return true
	}
	// Queue tab active is treated as visible
	if p.coordinator.ActiveTab() == app.QueueTab {
		return true
	}
	// Compute cover art position from config manager and determine whether the
	// queue content is shown as a secondary pane (right or left) depending on the
	// configured layout and whether drawers/tracklist are displayed.
	pos := "left"
	if p.coordinator.ConfigManager() != nil {
		pos = p.coordinator.ConfigManager().GetCoverArtPosition()
	}
	// When there is no drawer open and no album/playlist tracklist showing,
	// the queue is shown in the opposite column from the cover art (so it is visible).
	if (pos == "left" || pos == "right") && !p.drawerOpen && !p.showingTracks {
		return true
	}
	return false
}

func (p *LibraryPage) handleQueueKeyMsg(msg tea.KeyMsg) tea.Cmd {
	// Ensure the queue becomes focused when we start handling queue keys so
	// other lists do not steal the input.
	p.SetFocusedQueue(true)

	// Navigation keys (up/down/k/j or rune fallback) are handled by the list component.
	if key.Matches(msg, p.keys.Up) || key.Matches(msg, p.keys.Down) || isRuneKey(msg, 'k') || isRuneKey(msg, 'j') {
		var cmd tea.Cmd
		p.queueList, cmd = p.queueList.Update(msg)
		p.coordinator.SetQueueIndex(p.queueList.Index())
		return cmd
	}

	// Move the selected queue item up
	if key.Matches(msg, p.keys.QueueMoveUp) {
		sel := p.queueList.Index()
		q := p.coordinator.Queue()
		if sel > 0 && sel < len(q) {
			playIdx := p.coordinator.QueueIndex()
			q[sel-1], q[sel] = q[sel], q[sel-1]
			// Remap playing index if needed
			switch playIdx {
			case sel:
				playIdx = sel - 1
			case sel - 1:
				playIdx = sel
			}
			p.coordinator.SetQueue(q)
			p.coordinator.SetQueueIndex(playIdx)
			p.updateQueueListFromCoordinator()
			if playIdx >= 0 && playIdx < len(q) {
				p.queueList.Select(playIdx)
			} else {
				p.queueList.Select(sel - 1)
			}
		}
		return nil
	}

	// Move the selected queue item down
	if key.Matches(msg, p.keys.QueueMoveDown) {
		sel := p.queueList.Index()
		q := p.coordinator.Queue()
		if sel >= 0 && sel < len(q)-1 {
			playIdx := p.coordinator.QueueIndex()
			q[sel], q[sel+1] = q[sel+1], q[sel]
			// Remap playing index if needed
			switch playIdx {
			case sel:
				playIdx = sel + 1
			case sel + 1:
				playIdx = sel
			}
			p.coordinator.SetQueue(q)
			p.coordinator.SetQueueIndex(playIdx)
			p.updateQueueListFromCoordinator()
			if playIdx >= 0 && playIdx < len(q) {
				p.queueList.Select(playIdx)
			} else {
				p.queueList.Select(sel + 1)
			}
		}
		return nil
	}

	// Remove the selected queue item
	if key.Matches(msg, p.keys.QueueRemove) {
		sel := p.queueList.Index()
		q := p.coordinator.Queue()
		if sel >= 0 && sel < len(q) {
			playIdx := p.coordinator.QueueIndex()
			newQ := make([]app.Track, 0, len(q)-1)
			newQ = append(newQ, q[:sel]...)
			newQ = append(newQ, q[sel+1:]...)
			// If we're removing the currently playing queued element, stop and clear it.
			if playIdx == sel {
				if p.orchestrator != nil {
					_ = p.orchestrator.Stop()
				} else {
					p.coordinator.SetPlaybackState(app.PlaybackStopped)
				}
				p.coordinator.SetCurrentTrack(nil)
				playIdx = -1
			} else if playIdx > sel {
				playIdx = playIdx - 1
			}
			p.coordinator.SetQueue(newQ)
			p.coordinator.SetQueueIndex(playIdx)
			p.updateQueueListFromCoordinator()
			// Adjust selection to a valid index
			if playIdx >= 0 && playIdx < len(newQ) {
				p.queueList.Select(playIdx)
			} else if sel < len(newQ) {
				p.queueList.Select(sel)
			} else if len(newQ) > 0 {
				p.queueList.Select(len(newQ) - 1)
			}
		}
		return nil
	}

	// Play selected queue item:
	// Enter or Space in modal should trim the queue before the selected index and
	// start playback at the now-first queued element.
	if key.Matches(msg, p.keys.PlaySelected) || key.Matches(msg, p.keys.Enter) {
		sel := p.queueList.Index()
		q := p.coordinator.Queue()
		if len(q) == 0 || sel < 0 || sel >= len(q) {
			return nil
		}
		// Trim previous tracks when selecting an item inside the queue. After trimming,
		// the selected item becomes the first queued track.
		if sel > 0 {
			newQ := make([]app.Track, len(q)-sel)
			copy(newQ, q[sel:])
			p.coordinator.SetQueue(newQ)
			p.coordinator.SetQueueIndex(0)
			p.updateQueueListFromCoordinator()
			p.queueList.Select(0)
			// Close the modal and play the selected-first track.
			p.coordinator.SetShowQueueModal(false)
			return p.playAppTrack(&newQ[0])
		}
		// Selected item is already first — close modal and play it.
		at := &q[sel]
		p.coordinator.SetShowQueueModal(false)
		return p.playAppTrack(at)
	}

	// No special key matched, forward navigation to the queue list component.
	var cmd tea.Cmd
	p.queueList, cmd = p.queueList.Update(msg)
	p.coordinator.SetQueueIndex(p.queueList.Index())
	return cmd
}

func (p *LibraryPage) playNext() tea.Cmd {
	q := p.coordinator.Queue()
	tracks := p.coordinator.Tracks()

	// If there is an active queue and we're at the last queued item, stop playback
	// instead of wrapping. The queue semantics: play all remaining items and stop.
	if len(q) > 0 {
		idx := p.coordinator.QueueIndex()
		if idx < 0 {
			idx = 0
		}
		if idx >= len(q)-1 {
			// Queue is complete; stop playback and clear selection.
			if p.orchestrator != nil {
				_ = p.orchestrator.Stop()
			} else {
				p.coordinator.SetPlaybackState(app.PlaybackStopped)
			}
			p.coordinator.SetQueueIndex(-1)
			// Clear current track for UI clarity; playback finished
			p.coordinator.SetCurrentTrack(nil)
			return nil
		}
	}

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
	var cmds []tea.Cmd
	if err := p.orchestrator.PlayNext(p.ctx, pc, q, p.coordinator.QueueIndex(), tracks, p.coordinator.SelectedTrack()); err != nil {
		log.Error("playback play failed", "err", err)
		p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", err), "error", 10*time.Second)
	}
	// After orchestrator starts the next track, fetch cover art for UI playback pane
	if ct := p.coordinator.CurrentTrack(); ct != nil && ct.Thumb != "" && p.libSvc != nil {
		cmds = append(cmds, p.fetchCoverArtCmd(ct.Thumb))
	}
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
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
	var cmds []tea.Cmd
	if err := p.orchestrator.PlayPrev(p.ctx, pc, q, p.coordinator.QueueIndex(), tracks, p.coordinator.SelectedTrack()); err != nil {
		p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", err), "error", 10*time.Second)
	}
	if ct := p.coordinator.CurrentTrack(); ct != nil && ct.Thumb != "" && p.libSvc != nil {
		cmds = append(cmds, p.fetchCoverArtCmd(ct.Thumb))
	}
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
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
