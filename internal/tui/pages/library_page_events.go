package pages

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	log "github.com/charmbracelet/log/v2"
	"github.com/faiface/beep"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/http"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui/util"
)

func (p *LibraryPage) handleLibraryEvent(msg domain.LibraryEvent) (tea.Model, tea.Cmd) {
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
			return p, tea.Batch(
				p.subscribeToLibraryEvents(),
				p.subscribeToPlaybackEvents(),
				p.fetchLibraryStats(),
				p.spinner.Tick,
			)
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
		p.recentlyAddedComponent.SetItems(items)
		// Keep UI selection sane
		if len(appAlbums) > 0 {
			p.coordinator.SetSelectedAlbum(0)
			p.recentlyAddedComponent.Select(0)
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
		p.playlistComponent.SetItems(items)
		if len(appPlaylists) > 0 {
			p.coordinator.SetSelectedPlaylist(0)
			p.playlistComponent.Select(0)
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
		p.trackComponent.SetItems(items)
		if len(appTracks) > 0 {
			p.coordinator.SetSelectedTrack(0)
			p.trackComponent.Select(0)
		}

	// Error cases: log and display notification
	case "libraries.fetch_failed",
		"recently_added.fetch_failed",
		"playlists.fetch_failed",
		"albums.fetch_failed",
		"tracks.fetch_failed":
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
}

func (p *LibraryPage) handleAuthEvent(msg domain.AuthEvent) (tea.Model, tea.Cmd) {
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
			p.libSvc = service.NewLibraryServiceWithEvents(baseURL, token, http.NewFactory())
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
}

func (p *LibraryPage) handlePlaybackEvent(msg domain.PlaybackEvent) (tea.Model, tea.Cmd) {
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
		if msg.Duration > 0 {
			p.coordinator.SetStreamLength(msg.Duration)
		}
		if msg.SampleRate > 0 {
			p.coordinator.SetSampleRate(beep.SampleRate(msg.SampleRate))
		}

		// Detect end-of-track and auto-advance to the next queued track if applicable.
		// We debounce using `finishedTriggered` to avoid issuing multiple commands for
		// the same track-end event as position updates can be frequent.
		if msg.Duration > 0 && msg.Position >= msg.Duration {
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
}
