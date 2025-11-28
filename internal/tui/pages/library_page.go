package pages

import (
	"context"
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log/v2"

	"plexmusic-tui/internal/app"
	domain "plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/http"
	"plexmusic-tui/internal/logging"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	components "plexmusic-tui/internal/tui/components"
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
	authEvtCh <-chan pubsub.Event[domain.AuthEvent]
	libSvc    service.LibraryServicer
	libEvtCh  <-chan pubsub.Event[domain.LibraryEvent]
	// playback service is handled by the orchestrator (no pbSvc field on page)
	pbEvtCh <-chan pubsub.Event[domain.PlaybackEvent]

	// Lists
	homeComponent          *components.HomeComponent
	recentlyAddedComponent *components.RecentlyAddedComponent
	playlistComponent      *components.PlaylistsComponent
	trackComponent         *components.TracksComponent
	queueComponent         *components.QueueComponent
	searchComponent        *components.SearchComponent
	settingsComponent      *components.SettingsComponent

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
	lastSelectedQueueIndex    int
	lastSelectedTrackIndex    int

	// Help model
	help help.Model

	// Key bindings
	keys tui.LibraryKeyMap

	focusedNowPlaying bool
	focusedQueue      bool
	// Playback initialization state signals that we are in the process of
	// starting playback (e.g., fetching stream, creating PlayQueue). While
	// true, show a spinner and prevent the UI from indicating playback started.
	playbackInitializing bool
	playbackInitCancel   context.CancelFunc
	// Debounce Play key events (to avoid terminal auto-repeat toggles)
	lastPlayKey time.Time
	// autoPlayOnTracksLoaded signals that after tracks are fetched, we should
	// automatically begin playback of the first track (useful for async fetches).
	autoPlayOnTracksLoaded bool
}

// sonicDetectResultMsg is an internal message indicating the result of a
// manual sonic analysis detection run triggered by a keybinding or command.
type sonicDetectResultMsg struct{ ok bool }

// playResultMsg is returned from background playback startup commands to
// signal success or error to the UI loop.
type playResultMsg struct{ Err error }

// NewLibraryPage creates a library page and its cancellable event context.
func NewLibraryPage(coord app.Coordinatorer) *LibraryPage {
	return NewLibraryPageWithAuth(coord, nil)
}

func NewLibraryPageWithAuth(coord app.Coordinatorer, authSvc service.AuthServicer) *LibraryPage {
	ctx, cancel := context.WithCancel(context.Background())

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	orch := tui.NewOrchestrator(coord, nil, nil)

	p := &LibraryPage{
		coordinator:   coord,
		ctx:           ctx,
		cancel:        cancel,
		authSvc:       authSvc,
		showingTracks: false,
		drawerOpen:    false,
		loadingStats:  false,
		spinner:       s,
		// Track last selected indices so we can fetch tracks lazily when selection changes
		// without issuing repeated fetches.
		lastSelectedAlbumIndex:    -1,
		lastSelectedPlaylistIndex: -1,
		lastSelectedQueueIndex:    -1,
		lastSelectedTrackIndex:    -1,
		help:                      help.New(),
		keys:                      tui.DefaultLibraryKeyMap(),
		homeComponent:             components.NewHomeComponent(coord),
		recentlyAddedComponent:    components.NewRecentlyAddedComponent(coord),
		playlistComponent:         components.NewPlaylistsComponent(coord),
		trackComponent:            components.NewTracksComponent(coord),
		queueComponent:            components.NewQueueComponent(coord, orch),
		searchComponent:           components.NewSearchComponent(coord),
		settingsComponent:         components.NewSettingsComponent(coord),
		nowPlaying:                components.NewNowPlayingComponent(coord, nil),
		orchestrator:              orch,
	}

	if authSvc != nil {
		p.authEvtCh = authSvc.Subscribe(p.ctx)
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
	log.Debug(
		"LibraryPage: connecting to server",
		"name",
		server.Name,
		"baseURL",
		baseURL,
		"scheme",
		server.Scheme,
		"host",
		server.Host,
		"port",
		server.Port,
	)

	// Create (or reuse) library service and subscribe to events. This ensures we
	// only fetch library content when we have the necessary server + token.
	if p.libSvc == nil {
		p.libSvc = service.NewLibraryServiceWithEvents(baseURL, token, http.NewFactory())
		p.libEvtCh = p.libSvc.Subscribe(p.ctx)
	} else {
		// Update base URL and token to reflect current selected server.
		if err := p.libSvc.SetBaseURL(baseURL); err != nil {
			log.Warn("Failed to set base URL", "error", err)
		}
		if err := p.libSvc.SetToken(token); err != nil {
			log.Warn("Failed to set authentication token", "error", err)
		}
	}
	// Detect Plex Pass and sonic analysis availability and cache in coordinator
	if p.coordinator != nil {
		go func() {
			ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
			defer cancel()
			if ok, _ := p.libSvc.HasPlexPass(ctx); ok {
				p.coordinator.SetPlexPass(true)
			}
			if ok, _ := p.libSvc.HasSonicAnalysis(ctx); ok {
				p.coordinator.SetSonicAvailable(true)
			}
		}()
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
	p.queueComponent.SetOrchestrator(p.orchestrator)
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
		p.fetchLibraryHubs(),
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

		p.recentlyAddedComponent.SetSize(leftWidth, listHeight)
		p.playlistComponent.SetSize(leftWidth, listHeight)
		p.trackComponent.SetSize(leftWidth, listHeight)
		p.queueComponent.SetSize(leftWidth, listHeight)

		return p, nil

	case domain.LibraryEvent:
		return p.handleLibraryEvent(msg)

	case domain.AuthEvent:
		return p.handleAuthEvent(msg)

	case domain.PlaybackEvent:
		return p.handlePlaybackEvent(msg)
	case components.PlayResultMsg:
		// From queue component asynchronous play command
		p.playbackInitializing = false
		if p.playbackInitCancel != nil {
			p.playbackInitCancel = nil
		}
		if msg.Err != nil {
			p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", msg.Err), "error", 10*time.Second)
			p.coordinator.SetPlaybackState(app.PlaybackStopped)
		}
		return p, nil

	case spinner.TickMsg:
		if p.loadingStats || p.playbackInitializing {
			var cmd tea.Cmd
			p.spinner, cmd = p.spinner.Update(msg)
			log.Debug("spinner: tick", "loadingStats", p.loadingStats, "playbackInitializing", p.playbackInitializing)
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

	case playResultMsg:
		// Playback initialization finished (success or error)
		p.playbackInitializing = false
		if p.playbackInitCancel != nil {
			p.playbackInitCancel = nil
		}
		if msg.Err != nil {
			// Show error
			p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", msg.Err), "error", 10*time.Second)
			p.coordinator.SetPlaybackState(app.PlaybackStopped)
		}
		return p, nil

	case tea.KeyMsg:
		return p.handleKeyMsg(msg)
	case sonicDetectResultMsg:
		if msg.ok {
			// Fetch sonic enhanced content when analysis is present
			return p, tea.Batch(p.fetchMixesForYou(), p.fetchOnThisDay(), p.fetchMoodStations())
		}
		// Not found: notify user
		if p.coordinator != nil {
			p.coordinator.SetNotification("No sonic analysis detected", "error", 5*time.Second)
		}
		return p, nil
	}
	return p, nil
}

// dumpPageView writes the raw Page.View to the debug dump file when
// coordinator.DumpView() is enabled; useful for reproducing terminal render quirks.
func (p *LibraryPage) dumpPageView(label string) {
	if !p.coordinator.DumpView() {
		return
	}
	f, err := os.OpenFile(
		logging.GetDebugDumpFilePath(),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o600,
	)
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

// fetchLibraryHubs fetches all library hubs and stores them in coordinator for UI consumption.
func (p *LibraryPage) fetchLibraryHubs() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 15*time.Second)
		defer cancel()

		// Get the library section key
		libs, _, err := p.libSvc.FetchLibraries(ctx)
		if err != nil || len(libs) == 0 {
			log.Debug("fetchLibraryHubs: no libraries available", "err", err)
			return nil
		}
		sectionKey := libs[0].Key

		// Fetch all hubs for this library section
		hubs, err := p.libSvc.FetchLibraryHubs(ctx, sectionKey)
		if err != nil {
			log.Debug("fetchLibraryHubs failed", "err", err)
			return nil
		}

		log.Debug("fetchLibraryHubs success", "count", len(hubs))

		// Convert and store in coordinator
		if p.coordinator != nil {
			var appHubs []app.Hub
			for _, h := range hubs {
				// Convert playlists
				playlists := make([]app.Playlist, len(h.Playlists))
				for j, pl := range h.Playlists {
					playlists[j] = app.Playlist{
						Title:        pl.Title,
						Key:          pl.Key,
						LeafCount:    pl.LeafCount,
						Duration:     pl.Duration,
						PlaylistType: pl.PlaylistType,
					}
				}
				// Convert albums
				albums := make([]app.Album, len(h.Albums))
				for j, a := range h.Albums {
					albums[j] = app.Album{Title: a.Title, Artist: a.Artist, Year: a.Year, Key: a.Key, Thumb: a.Thumb}
				}
				appHubs = append(appHubs, app.Hub{
					HubIdentifier: h.HubIdentifier,
					Title:         h.Title,
					Type:          h.Type,
					Context:       h.Context,
					Size:          h.Size,
					Playlists:     playlists,
					Albums:        albums,
				})
			}
			p.coordinator.SetLibraryHubs(appHubs)

			// Also extract stations into MixesForYou for backward compatibility
			var mixes []app.Playlist
			for _, h := range appHubs {
				if strings.Contains(strings.ToLower(h.Context), "station") {
					mixes = append(mixes, h.Playlists...)
				}
			}
			if len(mixes) > 0 {
				p.coordinator.SetMixesForYou(mixes)
			}
		}
		return nil
	}
}

// fetchMixesForYou triggers fetching personalized mixes via library service.
func (p *LibraryPage) fetchMixesForYou() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		playlists, _, err := p.libSvc.FetchMixesForYou(ctx)
		if err != nil {
			log.Debug("fetchMixesForYou failed", "err", err)
		} else {
			log.Debug("fetchMixesForYou success", "count", len(playlists))
		}
		// store results in coordinator for UI consumption
		if p.coordinator != nil {
			var out []app.Playlist
			for _, pl := range playlists {
				out = append(
					out,
					app.Playlist{
						Title:        pl.Title,
						Key:          pl.Key,
						LeafCount:    pl.LeafCount,
						Duration:     pl.Duration,
						PlaylistType: pl.PlaylistType,
					},
				)
			}
			p.coordinator.SetMixesForYou(out)
		}
		return nil
	}
}

// fetchOnThisDay triggers fetching the on-this-day albums.
func (p *LibraryPage) fetchOnThisDay() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		albums, _, err := p.libSvc.FetchOnThisDay(ctx)
		if err != nil {
			log.Debug("fetchOnThisDay failed", "err", err)
		} else {
			log.Debug("fetchOnThisDay success", "count", len(albums))
		}
		if p.coordinator != nil {
			var out []app.Album
			for _, a := range albums {
				out = append(out, app.Album{Title: a.Title, Artist: a.Artist, Year: a.Year, Key: a.Key, Thumb: a.Thumb})
			}
			p.coordinator.SetOnThisDay(out)
		}
		return nil
	}
}

// fetchMoodStations triggers fetching mood-based station tracks.
// Instead of using hardcoded station names, we now fetch all available
// station-like hubs and display them.
func (p *LibraryPage) fetchMoodStations() tea.Cmd {
	if p.libSvc == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
		defer cancel()
		// FetchMoodStation with empty station name will return all mood-like content
		tracks, _, err := p.libSvc.FetchMoodStation(ctx, "", 20)
		if err != nil {
			log.Debug("fetchMoodStations failed", "err", err)
		} else {
			log.Debug("fetchMoodStations success", "count", len(tracks))
		}
		if p.coordinator != nil && len(tracks) > 0 {
			var out []app.Track
			for _, t := range tracks {
				out = append(
					out,
					app.Track{
						Title:       t.Title,
						Artist:      t.Artist,
						Album:       t.Album,
						Duration:    t.Duration,
						TrackNumber: t.TrackNumber,
						Key:         t.Key,
						RatingKey:   t.RatingKey,
						Thumb:       t.Thumb,
					},
				)
			}
			p.coordinator.SetMoodStations(out)
		}
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
			log.Debug(
				"fetchLibraryStats: success",
				"artists",
				artists,
				"albums",
				albums,
				"tracks",
				tracks,
			)
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
