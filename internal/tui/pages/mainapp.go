package pages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	// ansi is used in Tabs component; not needed here

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	components "plexmusic-tui/internal/tui/components"
	styles "plexmusic-tui/internal/tui/styles"
	views "plexmusic-tui/internal/ui" // view helpers: GetContentPaneWidth, GetDetailPaneWidth, FormatTrackDuration, FormatTimeDuration
)

// MainAppPage handles the main application UI with tab navigation,
// list rendering (Recently Added, Playlists), modal dialogs, and a simple
// "Now Playing" panel (cover art + controls).
type ModalType int

const (
	ModalNone ModalType = iota
	ModalRecently
	ModalPlaylists
	ModalSearch
	ModalSettings
	ModalQueue
)

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

	// Local selection state (mirrors coordinator's selections but used
	// for per-page selection UI)
	selectedAlbumIndex    int
	selectedPlaylistIndex int
	selectedTrackIndex    int

	// Search and modal UI
	searchInput       textinput.Model
	searchActive      bool
	searchTerm        string
	modal             ModalType
	drawerOpen        bool
	drawerOffset      int
	drawerTarget      int
	drawerAnimating   bool
	drawerStep        int
	focusedNowPlaying bool
}

// NewMainAppPage creates a new main application page and sets up a
// cancellable background context for event subscriptions.
func NewMainAppPage(coord *app.Coordinator) *MainAppPage {
	return NewMainAppPageWithAuth(coord, nil)
}

func NewMainAppPageWithAuth(coord *app.Coordinator, authSvc service.AuthServicer) *MainAppPage {
	ctx, cancel := context.WithCancel(context.Background())

	p := &MainAppPage{
		coordinator:     coord,
		ctx:             ctx,
		cancel:          cancel,
		authSvc:         authSvc,
		modal:           ModalNone,
		drawerOpen:      false,
		drawerOffset:    0,
		drawerTarget:    0,
		drawerAnimating: false,
		drawerStep:      3,
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
func (p *MainAppPage) modalForActiveTab(active app.TabType) ModalType {
	switch active {
	case app.LibraryTab:
		return ModalRecently
	case app.PlaylistsTab:
		return ModalPlaylists
	case app.SearchTab:
		return ModalSearch
	case app.SettingsTab:
		return ModalSettings
	default:
		return ModalNone
	}
}

// tabForModal maps a modal to the tab type (used for highlighting the nav
// row while the modal is active but without switching page content).
func (p *MainAppPage) tabForModal(m ModalType) app.TabType {
	switch m {
	case ModalRecently:
		return app.LibraryTab
	case ModalPlaylists:
		return app.PlaylistsTab
	case ModalSearch:
		return app.SearchTab
	case ModalSettings:
		return app.SettingsTab
	case ModalQueue:
		return app.QueueTab
	default:
		return p.coordinator.ActiveTab()
	}
}

// drawerTickMsg is an internal message used to animate the drawer slide.
type drawerTickMsg time.Time

// drawerTickCmd returns a command that sends a drawerTickMsg on a short interval.
func drawerTickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*15, func(t time.Time) tea.Msg {
		return drawerTickMsg(t)
	})
}

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
	p.selectedAlbumIndex = 0
	p.coordinator.SetSelectedPlaylist(0)
	p.selectedPlaylistIndex = 0
	p.coordinator.SetSelectedTrack(0)
	p.selectedTrackIndex = 0

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
			for i, a := range msg.Albums {
				appAlbums[i] = app.Album{
					Title:  a.Title,
					Artist: a.Artist,
					Year:   a.Year,
					Key:    a.Key,
					Thumb:  a.Thumb,
				}
			}
			p.coordinator.SetAlbums(appAlbums)
			// Keep UI selection sane
			if len(appAlbums) > 0 {
				p.coordinator.SetSelectedAlbum(0)
				p.selectedAlbumIndex = 0
			}

		case "playlists.loaded":
			appPlaylists := make([]app.Playlist, len(msg.Playlists))
			for i, pl := range msg.Playlists {
				appPlaylists[i] = app.Playlist{
					Title:        pl.Title,
					Key:          pl.Key,
					LeafCount:    pl.LeafCount,
					Duration:     pl.Duration,
					PlaylistType: pl.PlaylistType,
				}
			}
			p.coordinator.SetPlaylists(appPlaylists)
			if len(appPlaylists) > 0 {
				p.coordinator.SetSelectedPlaylist(0)
				p.selectedPlaylistIndex = 0
			}

		case "tracks.loaded":
			appTracks := make([]app.Track, len(msg.Tracks))
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
			}
			p.coordinator.SetTracks(appTracks)
			if len(appTracks) > 0 {
				p.coordinator.SetSelectedTrack(0)
				p.selectedTrackIndex = 0
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
	case drawerTickMsg:
		// Animate the drawer open/close sliding.
		// Target height is a percent of the window height (drawer slides up).
		ph := p.height - 6
		var target int
		// Prefer a pre-computed drawer target if present.
		if p.drawerTarget > 0 {
			target = p.drawerTarget
		} else {
			target = ph * 45 / 100
			if target < 6 {
				target = 6
			}
		}
		step := p.drawerStep
		if p.drawerAnimating {
			if p.drawerOpen {
				// Opening: increase offset until target.
				if p.drawerOffset < target {
					p.drawerOffset += step
					if p.drawerOffset > target {
						p.drawerOffset = target
					}
					return p, drawerTickCmd()
				}
				// Reached target
				p.drawerOffset = target
				p.drawerAnimating = false
				return p, nil
			} else {
				// Closing: decrease offset until 0.
				if p.drawerOffset > 0 {
					p.drawerOffset -= step
					if p.drawerOffset < 0 {
						p.drawerOffset = 0
					}
					return p, drawerTickCmd()
				}
				// Fully closed: clear modal and target.
				p.drawerAnimating = false
				p.modal = ModalNone
				p.drawerTarget = 0
				return p, nil
			}
		}
		return p, nil

	case tea.KeyMsg:
		// When the search modal is active, let the text input widget handle keys.
		// This prioritizes search characters and allows Enter/Esc to confirm/abort.
		if p.modal == ModalSearch {
			var cmd tea.Cmd
			p.searchInput, cmd = p.searchInput.Update(msg)
			// If user hits Enter or Esc, close the search modal and apply/clear.
			switch msg.String() {
			case "enter":
				p.searchTerm = p.searchInput.Value()
				// TODO: apply search across collections (albums/playlists/tracks)
				p.modal = ModalNone
				p.searchInput.Blur()
				return p, cmd
			case "esc":
				p.modal = ModalNone
				p.searchInput.Blur()
				return p, cmd
			default:
				return p, cmd
			}
		}

		// Key handling for the main app. We map:
		// - Tab / Right: next tab
		// - Shift+Tab / Left: previous tab
		// - Up / Down: navigate lists
		// - Enter: select (fetch tracks / open playlist / play queue)
		// - Space or p: Play/Pause the selected track
		// - o: toggle queue modal
		// - s: toggle search modal
		// - r: refresh Recently Added + Playlists
		// - f: toggle focused Now Playing
		// - esc: go back to server selection (handled in outer router)
		var active app.TabType
		switch msg.String() {
		case "esc":
			// If the queue modal is open, close it first; otherwise go back to server selection.
			if p.coordinator.ShowQueueModal() {
				p.coordinator.SetShowQueueModal(false)
				return p, nil
			}
			// If another modal/drawer is open, close it first via animation.
			if p.modal != ModalNone {
				if p.drawerOffset > 0 || p.drawerAnimating {
					// Start the closing animation. Do not clear the Modal type until the animation completes.
					p.drawerOpen = false
					p.drawerAnimating = true
					return p, drawerTickCmd()
				}
				// If there is no open drawer or animation active, clear the modal immediately.
				p.modal = ModalNone
				return p, nil
			}
			return p, func() tea.Msg {
				return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
			}
		case "tab", "right":
			// Always move to the next tab on Tab/Right
			p.coordinator.NextTab()
			// If a drawer is open or animating, close it while keeping the newly selected tab active.
			if p.drawerOpen || p.drawerAnimating {
				p.drawerOpen = false
				p.drawerAnimating = true
				return p, drawerTickCmd()
			}
			return p, nil
		case "shift+tab", "left":
			// Always move to the previous tab on Shift+Tab/Left
			p.coordinator.PreviousTab()
			// Close the drawer if it is open or animating.
			if p.drawerOpen || p.drawerAnimating {
				p.drawerOpen = false
				p.drawerAnimating = true
				return p, drawerTickCmd()
			}
			return p, nil
		case "up", "k":
			// If a modal is open, navigate the modal's selection instead of the
			// main tabs. This keeps the "Now Playing" UI focused while letting the
			// modal lists be navigable with arrow keys.
			if p.modal == ModalRecently {
				if p.selectedAlbumIndex > 0 {
					p.selectedAlbumIndex--
					// Keep coordinator selection consistent when moving in modal.
					p.coordinator.SetSelectedAlbum(p.selectedAlbumIndex)
				}
				return p, nil
			}
			if p.modal == ModalPlaylists {
				if p.selectedPlaylistIndex > 0 {
					p.selectedPlaylistIndex--
					p.coordinator.SetSelectedPlaylist(p.selectedPlaylistIndex)
				}
				return p, nil
			}

			active = p.coordinator.ActiveTab()
			switch active {
			case app.HomeTab, app.LibraryTab: // Recently Added
				if p.selectedAlbumIndex > 0 {
					p.selectedAlbumIndex--
					p.coordinator.SetSelectedAlbum(p.selectedAlbumIndex)
				}
			case app.PlaylistsTab:
				if p.selectedPlaylistIndex > 0 {
					p.selectedPlaylistIndex--
					p.coordinator.SetSelectedPlaylist(p.selectedPlaylistIndex)
				}
			case app.QueueTab:
				if p.coordinator.QueueIndex() > 0 {
					p.coordinator.SetQueueIndex(p.coordinator.QueueIndex() - 1)
				}
			}
			return p, nil
		case "down", "j":
			// If a modal is open, navigate the modal's selection instead of the
			// main tabs.
			if p.modal == ModalRecently {
				if p.selectedAlbumIndex < len(p.coordinator.Albums())-1 {
					p.selectedAlbumIndex++
					p.coordinator.SetSelectedAlbum(p.selectedAlbumIndex)
				}
				return p, nil
			}
			if p.modal == ModalPlaylists {
				if p.selectedPlaylistIndex < len(p.coordinator.Playlists())-1 {
					p.selectedPlaylistIndex++
					p.coordinator.SetSelectedPlaylist(p.selectedPlaylistIndex)
				}
				return p, nil
			}

			active = p.coordinator.ActiveTab()
			switch active {
			case app.HomeTab, app.LibraryTab: // Recently Added list
				if p.selectedAlbumIndex < len(p.coordinator.Albums())-1 {
					p.selectedAlbumIndex++
					p.coordinator.SetSelectedAlbum(p.selectedAlbumIndex)
				}
			case app.PlaylistsTab:
				if p.selectedPlaylistIndex < len(p.coordinator.Playlists())-1 {
					p.selectedPlaylistIndex++
					p.coordinator.SetSelectedPlaylist(p.selectedPlaylistIndex)
				}
			case app.QueueTab:
				if p.coordinator.QueueIndex() < len(p.coordinator.Queue())-1 {
					p.coordinator.SetQueueIndex(p.coordinator.QueueIndex() + 1)
				}
			}
			return p, nil
		case "enter":
			// If no drawer is open, open the drawer for the currently selected tab.
			active := p.coordinator.ActiveTab()
			if !p.drawerOpen && !p.drawerAnimating && p.modal == ModalNone {
				switch active {
				case app.LibraryTab, app.HomeTab:
					p.modal = ModalRecently
					p.drawerOpen = true
					p.drawerTarget = p.height * 45 / 100
					p.drawerAnimating = true
					return p, drawerTickCmd()
				case app.PlaylistsTab:
					p.modal = ModalPlaylists
					p.drawerOpen = true
					p.drawerTarget = p.height * 45 / 100
					p.drawerAnimating = true
					return p, drawerTickCmd()
				case app.SearchTab:
					p.modal = ModalSearch
					p.drawerOpen = true
					p.drawerTarget = p.height * 45 / 100
					p.drawerAnimating = true
					return p, tea.Batch(drawerTickCmd(), p.searchInput.Focus())
				case app.SettingsTab:
					p.modal = ModalSettings
					p.drawerOpen = true
					p.drawerTarget = p.height * 45 / 100
					p.drawerAnimating = true
					return p, drawerTickCmd()
				}
			}
			// If a drawer is active, treat Enter as an action inside the drawer/modal.
			if p.modal == ModalRecently {
				if p.libSvc != nil {
					reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
					defer cancel()
					albums := p.coordinator.Albums()
					if p.selectedAlbumIndex >= 0 && p.selectedAlbumIndex < len(albums) {
						_, _ = p.libSvc.FetchTracks(reqCtx, albums[p.selectedAlbumIndex].Key)
						// Start closing animation: leave `p.modal` until animation completes.
						p.drawerOpen = false
						p.drawerAnimating = true
						return p, drawerTickCmd()
					}
				}
				return p, nil
			}
			if p.modal == ModalPlaylists {
				if p.libSvc != nil {
					reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
					defer cancel()
					playlists := p.coordinator.Playlists()
					if p.selectedPlaylistIndex >= 0 && p.selectedPlaylistIndex < len(playlists) {
						_, _ = p.libSvc.FetchTracks(reqCtx, playlists[p.selectedPlaylistIndex].Key)
						// Start closing animation
						p.drawerOpen = false
						p.drawerAnimating = true
						return p, drawerTickCmd()
					}
				}
				return p, nil
			}

			// Enter: fetch tracks for selected album or playlist, or play selected queue item
			active = p.coordinator.ActiveTab()
			if p.libSvc == nil {
				// For the queue page, pressing enter should play a queue item.
				if active == app.QueueTab && len(p.coordinator.Queue()) > 0 {
					q := p.coordinator.Queue()
					idx := p.coordinator.QueueIndex()
					if idx < 0 || idx >= len(q) {
						idx = 0
					}
					p.playAppTrack(&q[idx])
				}
				return p, nil
			}
			reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
			defer cancel()
			switch active {
			case app.HomeTab, app.LibraryTab:
				albums := p.coordinator.Albums()
				if p.selectedAlbumIndex >= 0 && p.selectedAlbumIndex < len(albums) {
					// Use library service to fetch tracks for the selected album
					_, _ = p.libSvc.FetchTracks(reqCtx, albums[p.selectedAlbumIndex].Key)
				}
			case app.PlaylistsTab:
				playlists := p.coordinator.Playlists()
				if p.selectedPlaylistIndex >= 0 && p.selectedPlaylistIndex < len(playlists) {
					_, _ = p.libSvc.FetchTracks(reqCtx, playlists[p.selectedPlaylistIndex].Key)
				}
			case app.QueueTab:
				q := p.coordinator.Queue()
				idx := p.coordinator.QueueIndex()
				if idx < 0 || idx >= len(q) {
					idx = 0
				}
				p.playAppTrack(&q[idx])
			}
			return p, nil
		case " ", "p":
			// If a modal is open, attempt to fetch and play the first track from the
			// selected album or playlist.
			if p.modal == ModalRecently {
				if p.libSvc != nil {
					reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
					defer cancel()
					albums := p.coordinator.Albums()
					if p.selectedAlbumIndex >= 0 && p.selectedAlbumIndex < len(albums) {
						tracks, _ := p.libSvc.FetchTracks(reqCtx, albums[p.selectedAlbumIndex].Key)
						if len(tracks) > 0 {
							// Convert the first domain.Track returned into an app.Track
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
							// Use the helper so UI & playback service stay in sync
							p.playAppTrack(&at)
							// Start closing animation
							p.drawerOpen = false
							p.drawerAnimating = true
							return p, drawerTickCmd()
						}
					}
				}
				return p, nil
			}
			if p.modal == ModalPlaylists {
				if p.libSvc != nil {
					reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
					defer cancel()
					playlists := p.coordinator.Playlists()
					if p.selectedPlaylistIndex >= 0 && p.selectedPlaylistIndex < len(playlists) {
						tracks, _ := p.libSvc.FetchTracks(reqCtx, playlists[p.selectedPlaylistIndex].Key)
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
							// Start closing animation
							p.drawerOpen = false
							p.drawerAnimating = true
							return p, drawerTickCmd()
						}
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
					// Use p.selectedTrackIndex if set
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
		case "n":
			p.playNext()
			return p, nil
		case "b":
			p.playPrev()
			return p, nil
		case "+", "=":
			p.adjustVolume(0.1)
			return p, nil
		case "-":
			p.adjustVolume(-0.1)
			return p, nil
		case "o":
			// Toggle queue modal
			p.coordinator.SetShowQueueModal(!p.coordinator.ShowQueueModal())
			return p, nil
		case "r":
			// Refresh library lists (recently added + playlists)
			if p.libSvc != nil {
				return p, tea.Batch(p.fetchRecentlyAdded(), p.fetchPlaylists())
			}
			return p, nil
		case "s":
			// Toggle search modal drawer
			if p.modal == ModalSearch {
				// Start closing animation (keep modal until fully closed)
				p.drawerOpen = false
				p.drawerAnimating = true
				p.searchInput.Blur()
				return p, drawerTickCmd()
			} else {
				p.modal = ModalSearch
				p.drawerOpen = true
				p.drawerTarget = p.height * 45 / 100
				p.drawerAnimating = true
				p.searchInput.Focus()
				return p, tea.Batch(drawerTickCmd(), p.searchInput.Focus())
			}
		case "f":
			// Toggle pair of focused Now Playing view
			p.focusedNowPlaying = !p.focusedNowPlaying
			return p, nil
		}
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
	// Compute the Now Playing width and use that width for tab alignment.
	// Start with the full available width and ensure it's at least wide enough
	// for the content + detail pane split.
	nowWidth := p.width - 6
	if nowWidth < contentWidth+detailWidth {
		nowWidth = contentWidth + detailWidth
	}

	// Build top tabs mapped by TabType to ensure consistent mapping
	tabNames := []string{"Home", "Recently Added", "Playlists", "Search", "Queue", "Settings"}
	active := p.coordinator.ActiveTab()
	// Ensure active tab is valid. If it's out of the expected range, set Home
	// as a safe default to ensure the UI renders content instead of an empty
	// fallback state.
	if active < app.HomeTab || active > app.SettingsTab {
		p.coordinator.SetActiveTab(app.HomeTab)
		active = app.HomeTab
	}
	// If a modal is open, highlight the tab corresponding to that modal rather
	// than the current active tab. This makes the nav reflect the modal that is
	// being shown (Recently/Playlists/Search/Settings), even while content remains
	// on the Now Playing centric UI.
	navActive := active
	if p.modal != ModalNone {
		navActive = p.tabForModal(p.modal)
	}

	// Move tab building into a component to keep rendering logic isolated.
	tabsComp := components.NewTabs(tabNames)
	tabsPane, _ := tabsComp.Render(nowWidth, int(navActive))

	// Tabs are now displayed below the Now Playing area (left navigation removed).

	// Main content — Now Playing becomes the primary content area.
	// The Now Playing view will take the primary area. Tab selection is below,
	// and pressing Enter opens the selected tab as an overlay drawer over Now Playing.
	// nowWidth was computed above — no need to recompute here. This ensures the
	// tabs were created using the final Now Playing width and remain aligned.

	mainContent := p.renderNowPlaying(nowWidth)
	if p.libSvc != nil && len(p.coordinator.Albums()) == 0 && len(p.coordinator.Playlists()) == 0 {
		// Show a friendly, centered loading placeholder in the content area when
		// the library service is active but no content has been loaded yet.
		mainContent = lipgloss.JoinVertical(lipgloss.Center, styles.BlurredStyle.Render("Loading library..."))
	}
	contentPane := styles.PaneStyle(nowWidth, p.height-6).Render(mainContent)

	// Compose the two-pane layout: main content (Now Playing) and tabs.
	// Center the main content and tabs horizontally so they appear visually centered
	// on the screen.
	layout := lipgloss.JoinVertical(lipgloss.Center, contentPane, tabsPane)

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

	// If a drawer/modal is active, render the slide-out drawer that slides up
	// from the bottom over the Now Playing content and dim the Now Playing area.
	drawerHeight := p.drawerOffset
	if drawerHeight > 0 {
		// Dim the Now Playing area to indicate modal focus using the Scrim style.
		dimNowPlaying := styles.ScrimStyle.Render(mainContent)
		contentPaneDim := styles.PaneStyle(nowWidth, p.height-6).Render(dimNowPlaying)

		// Render drawer content and anchor to the bottom.
		drawerContent := p.renderModalContent(nowWidth, "")
		// Append a small help hint to the drawer so users know available keys while the overlay is focused.
		drawerPane := styles.PaneStyle(nowWidth, drawerHeight).Render(lipgloss.JoinVertical(lipgloss.Left, drawerContent, styles.HelpStyle.Render("Enter: open • Space: play • Esc: close")))

		baseWithTabs := lipgloss.JoinVertical(lipgloss.Center, contentPaneDim, tabsPane)
		overlayBottom := lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Bottom, drawerPane)

		// If Queue modal is visible, overlay it on top of the drawer layout as before.
		if p.coordinator.ShowQueueModal() {
			return p.renderWithModal(baseWithTabs)
		}
		return lipgloss.JoinVertical(lipgloss.Center, baseWithTabs, overlayBottom)
	}
	// If the drawer is not present and the (ancillary) modal was requested but
	// the drawer offset is zero (maybe animation not yet started), fall back to the
	// previous centered modal behavior to prevent a confusing blank state.
	if p.modal != ModalNone && p.drawerOffset == 0 {
		content := p.renderModalContent(contentWidth, contentPane)
		return p.renderWithCustomModal(layout, content)
	}

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
	quitHint := styles.HelpStyle.Render("Ctrl+C: Quit")

	centeredLayout := lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Top, layout)
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TitleStyle.Render(pageTitle),
		lipgloss.JoinHorizontal(lipgloss.Left, statusLine, "  ", quitHint),
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

// renderHome composes a small home view with the recently added list and
// the Now Playing panel side-by-side.
func (p *MainAppPage) renderHome(width int) string {
	// Simple two-column split: left (recently added) and right (now playing)
	leftWidth := width * 60 / 100
	rightWidth := width - leftWidth - 2

	left := p.renderRecentlyAdded(leftWidth)
	right := p.renderNowPlaying(rightWidth)

	return lipgloss.JoinHorizontal(lipgloss.Left,
		styles.PaneStyle(leftWidth, p.height-2).Render(left),
		styles.PaneStyle(rightWidth, p.height-2).Render(right),
	)
}

// renderRecentlyAdded displays the current recently-added albums list.
func (p *MainAppPage) renderRecentlyAdded(width int) string {
	title := styles.TitleStyle.Render("Recently Added")

	var lines []string
	albums := p.coordinator.Albums()
	if len(albums) == 0 {
		lines = append(lines, styles.BlurredStyle.Render("No recently added albums"))
	} else {
		for i, a := range albums {
			prefix := "  "
			style := styles.BlurredStyle
			if i == p.selectedAlbumIndex || i == p.coordinator.SelectedAlbum() {
				prefix = "> "
				style = styles.FocusedStyle
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s%s • %s (%d)", prefix, a.Title, a.Artist, a.Year)))
		}
	}

	help := styles.HelpStyle.Render("↑/↓: navigate • enter: view • p/space: play • n: next • b: prev • +/-: volume • o: queue")

	return lipgloss.JoinVertical(lipgloss.Left, title, "", lipgloss.JoinVertical(lipgloss.Left, lines...), "", help)
}

// renderPlaylists displays the playlists list.
func (p *MainAppPage) renderPlaylists(width int) string {
	title := styles.TitleStyle.Render("Playlists")

	var lines []string
	playlists := p.coordinator.Playlists()
	if len(playlists) == 0 {
		lines = append(lines, styles.BlurredStyle.Render("No playlists"))
	} else {
		for i, pl := range playlists {
			prefix := "  "
			style := styles.BlurredStyle
			if i == p.selectedPlaylistIndex || i == p.coordinator.SelectedPlaylist() {
				prefix = "> "
				style = styles.FocusedStyle
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s%s (%d)", prefix, pl.Title, pl.LeafCount)))
		}
	}

	help := styles.HelpStyle.Render("↑/↓: navigate • enter: open • p/space: play selected • n: next • b: prev • +/-: volume")

	return lipgloss.JoinVertical(lipgloss.Left, title, "", lipgloss.JoinVertical(lipgloss.Left, lines...), "", help)
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

	controls := styles.HelpStyle.Render("Space/p: Play/Pause • n: Next • b: Prev • +/-: Volume • o: Queue")

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

	controls := styles.HelpStyle.Render("Space/p: Play/Pause • n: Next • b: Prev • +/-: Volume • o: Queue • f: Toggle Focus")

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

// renderWithCustomModal overlays any content (string) as a centered modal
// and returns the combined view for use in View()
func (p *MainAppPage) renderWithCustomModal(base, content string) string {
	modalStyled := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(60).Render(content)

	// Center overlay
	return lipgloss.Place(
		p.width,
		p.height,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, base, "", modalStyled),
	)
}

// renderModalContent returns content for the current active modal type.
func (p *MainAppPage) renderModalContent(width int, baseContent string) string {
	switch p.modal {
	case ModalSearch:
		title := styles.TitleStyle.Render("Search")
		// Display input and search results
		results := []string{}
		term := strings.TrimSpace(p.searchInput.Value())
		if term != "" {
			// Very simple matching against album title and artist
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
		return lipgloss.JoinVertical(lipgloss.Left, title, "", p.searchInput.View(), "", lipgloss.JoinVertical(lipgloss.Left, results...), "")
	case ModalRecently:
		title := styles.TitleStyle.Render("Recently Added")
		var lines []string
		for i, a := range p.coordinator.Albums() {
			prefix := "  "
			style := styles.BlurredStyle
			if i == p.selectedAlbumIndex {
				prefix = "> "
				style = styles.FocusedStyle
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s%s • %s", prefix, a.Title, a.Artist)))
		}
		if len(lines) == 0 {
			lines = append(lines, styles.BlurredStyle.Render("No recently added albums"))
		}
		help := styles.HelpStyle.Render("Enter: open • Space: play • Esc: close")
		return lipgloss.JoinVertical(lipgloss.Left, title, "", lipgloss.JoinVertical(lipgloss.Left, lines...), "", help)

	case ModalPlaylists:
		title := styles.TitleStyle.Render("Playlists")
		var lines []string
		for i, pl := range p.coordinator.Playlists() {
			prefix := "  "
			style := styles.BlurredStyle
			if i == p.selectedPlaylistIndex {
				prefix = "> "
				style = styles.FocusedStyle
			}
			lines = append(lines, style.Render(fmt.Sprintf("%s%s (%d)", prefix, pl.Title, pl.LeafCount)))
		}
		if len(lines) == 0 {
			lines = append(lines, styles.BlurredStyle.Render("No playlists"))
		}
		help := styles.HelpStyle.Render("Enter: open • Space: play • Esc: close")
		return lipgloss.JoinVertical(lipgloss.Left, title, "", lipgloss.JoinVertical(lipgloss.Left, lines...), "", help)

	case ModalSettings:
		title := styles.TitleStyle.Render("Settings")
		var lines []string
		lines = append(lines, styles.BlurredStyle.Render("Settings are not yet available in a modal."))
		lines = append(lines, styles.BlurredStyle.Render("Press Esc to close."))
		return lipgloss.JoinVertical(lipgloss.Left, title, "", lipgloss.JoinVertical(lipgloss.Left, lines...))
	default:
		return styles.BlurredStyle.Render("Unknown modal")
	}
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
