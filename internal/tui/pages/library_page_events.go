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

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (p *LibraryPage) handleLibraryEvent(msg domain.LibraryEvent) (tea.Model, tea.Cmd) {
	var postCmd tea.Cmd
	switch msg.Type {
	case "server.plexpass":
		// Plex Pass available on the server
		p.coordinator.SetPlexPass(true)
		p.coordinator.SetNotification("Plex Pass detected: sonic features may be available.", "info", 6*time.Second)
		return p, tea.Batch(p.subscribeToLibraryEvents(), p.subscribeToPlaybackEvents())
	case "library.sonic_analyzed":
		// Sonic analysis detected — enable sonic features and fetch related home content
		p.coordinator.SetSonicAvailable(true)
		// Trigger home content fetches for Mixes/OnThisDay/MoodStations
		return p, tea.Batch(
			p.fetchMixesForYou(),
			p.fetchOnThisDay(),
			p.fetchMoodStations(),
			p.subscribeToLibraryEvents(),
			p.subscribeToPlaybackEvents(),
		)
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

	case "mixes.loaded":
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
		p.coordinator.SetMixesForYou(appPlaylists)

	case "onthisday.loaded":
		appAlbums := make([]app.Album, len(msg.Albums))
		for i, a := range msg.Albums {
			appAlbums[i] = app.Album{Title: a.Title, Artist: a.Artist, Year: a.Year, Key: a.Key, Thumb: a.Thumb}
		}
		p.coordinator.SetOnThisDay(appAlbums)

	case "moodstation.loaded":
		appTracks := make([]app.Track, len(msg.Tracks))
		for i, t := range msg.Tracks {
			if at := util.DomainTrackToApp(&t); at != nil {
				appTracks[i] = *at
			}
		}
		p.coordinator.SetMoodStations(appTracks)

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

		// Attempt to fetch cover art for the first track when tracks are loaded
		if len(appTracks) > 0 {
			if appTracks[0].Thumb != "" && p.coordinator.PlaybackAlbumArtThumb() != appTracks[0].Thumb {
				postCmd = p.fetchCoverArtCmd(appTracks[0].Thumb)
			}
		}

		// If playback was requested immediately after a fetch, set queue and
		log.Debug(
			"tracks.loaded: payload",
			"autoPlayOnTracksLoaded",
			p.autoPlayOnTracksLoaded,
			"trackCount",
			len(appTracks),
		)
		// kick off playback of the first track.
		if p.autoPlayOnTracksLoaded {
			log.Info("tracks.loaded: autoPlayOnTracksLoaded triggered - this is the WRONG path for stations!")
			p.autoPlayOnTracksLoaded = false
			// Build queue and queue items
			q := make([]app.Track, len(appTracks))
			copy(q, appTracks)
			p.coordinator.SetQueue(q)
			p.coordinator.SetQueueIndex(0)
			p.queueComponent.UpdateListFromCoordinator()
			p.showingTracks = false
			p.coordinator.SetActiveTab(app.QueueTab)
			// Play first track asynchronously
			if len(q) > 0 {
				log.Info(
					"tracks.loaded: playing first track",
					"title",
					q[0].Title,
					"playQueueItemID",
					q[0].PlayQueueItemID,
				)
				// Include postCmd (fetch cover art) alongside subscription/setup
				if postCmd != nil {
					return p, tea.Batch(
						append([]tea.Cmd{postCmd}, p.playAppTrack(&q[0]), p.subscribeToPlaybackEvents())...)
				}
				return p, tea.Batch(p.playAppTrack(&q[0]), p.subscribeToPlaybackEvents())
			}
		}

	// Error cases: log and display notification
	case "libraries.fetch_failed",
		"recently_added.fetch_failed",
		"playlists.fetch_failed",
		"albums.fetch_failed",
		"tracks.fetch_failed":
	case "mixes.fetch_failed",
		"onthisday.fetch_failed",
		"moodstation.fetch_failed":
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
	if postCmd != nil {
		return p, tea.Batch(postCmd, p.subscribeToLibraryEvents(), p.subscribeToPlaybackEvents())
	}
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
	var postCmd tea.Cmd
	switch msg.Type {
	case "playback.load_failed":
		// Mark that the last load failed to prevent double-skip from playback.finished
		p.lastLoadFailed = true
		if msg.Error != nil {
			p.coordinator.SetNotification(fmt.Sprintf("Load failed: %v", msg.Error), "error", 10*time.Second)
			log.Debug("LibraryPage: set load_failed notification", "err", msg.Error)
		} else {
			p.coordinator.SetNotification("Load failed", "error", 10*time.Second)
			log.Debug("LibraryPage: set load_failed notification", "no err")
		}
	case "playback.play_failed":
		p.lastLoadFailed = true
		if msg.Error != nil {
			p.coordinator.SetNotification(fmt.Sprintf("Play failed: %v", msg.Error), "error", 10*time.Second)
		} else {
			p.coordinator.SetNotification("Play failed", "error", 10*time.Second)
		}
	case "playback.started":
		// Clear the load failed flag since playback started successfully
		p.lastLoadFailed = false
		// Record when this track started to help detect stale advance messages
		p.lastTrackStarted = time.Now()
		p.coordinator.SetPlaybackState(app.PlaybackPlaying)
		if msg.Track != nil {
			track := util.DomainTrackToApp(msg.Track)
			p.coordinator.SetCurrentTrack(track)
			// Fetch the album art for the track now that playback started
			if track.Thumb != "" && p.coordinator.PlaybackAlbumArtThumb() != track.Thumb {
				postCmd = p.fetchCoverArtCmd(track.Thumb)
			}
		}
		// Update queue UI to reflect the new playing track
		p.queueComponent.UpdateListFromCoordinator()
	case "playback.resumed":
		p.coordinator.SetPlaybackState(app.PlaybackPlaying)
	case "playback.paused":
		p.coordinator.SetPlaybackState(app.PlaybackPaused)
	case "playback.stopped":
		p.coordinator.SetPlaybackState(app.PlaybackStopped)
		// Clear the active playQueue when playback is explicitly stopped
		p.coordinator.ClearActivePlayQueue()
	case "playback.volume_changed":
		// Playback service publishes floats — we don't keep it in coordinator as a primitive.
	case "playback.finished":
		// Auto-advance to the next queued track.
		// We check IsPlaying to ensure we don't auto-advance if the user explicitly stopped playback,
		// although playback.finished usually implies the stream ran to completion.
		// Also skip if the last load failed - this prevents double-skip when a failed decode
		// causes both load_failed and finished events to fire in quick succession.
		if p.coordinator.IsPlaying() && !p.lastLoadFailed {
			// Schedule advance with timestamp so we can detect stale messages
			scheduledAt := time.Now()
			return p, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return playbackAdvanceMsg{scheduledAt: scheduledAt}
			})
		}
		// Reset the flag after checking so it doesn't affect future tracks
		p.lastLoadFailed = false
	case "playback.advance_next":
		// Legacy event type - redirect to playNext
		// This may still be used by external callers
		return p, p.playNext()
	case "playback.position":
		// Periodic position updates from the service.
		// Only update state if values actually changed to avoid unnecessary re-renders.
		stateChanged := false
		if msg.Position >= 0 {
			// Only update if position changed by at least 1 second worth of samples
			// to reduce re-render frequency while still showing progress.
			oldPos := p.coordinator.StreamPosition()
			sr := p.coordinator.SampleRate()
			if sr == 0 {
				sr = 44100 // default sample rate
			}
			// Threshold: 1 second of samples
			threshold := int(sr)
			if abs(msg.Position-oldPos) >= threshold {
				p.coordinator.SetStreamPosition(msg.Position)
				stateChanged = true
			}
		}
		if msg.Duration > 0 && msg.Duration != p.coordinator.StreamLength() {
			p.coordinator.SetStreamLength(msg.Duration)
			stateChanged = true
		}
		if msg.SampleRate > 0 && beep.SampleRate(msg.SampleRate) != p.coordinator.SampleRate() {
			p.coordinator.SetSampleRate(beep.SampleRate(msg.SampleRate))
			stateChanged = true
		}
		// Only trigger re-render if state actually changed
		if !stateChanged {
			// Still need to re-subscribe but skip triggering a view update
			return p, p.subscribeToPlaybackEvents()
		}
	}
	// Re-subscribe to continue receiving playback/library events
	if postCmd != nil {
		return p, tea.Batch(postCmd, p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
	}
	return p, tea.Batch(p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
}
