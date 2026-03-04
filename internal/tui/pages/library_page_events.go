package pages

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
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
		p.appCtx.Content.SetPlexPass(true)
		p.appCtx.View.SetNotification("Plex Pass detected: sonic features may be available.", "info", 6*time.Second)
		return p, tea.Batch(p.subscribeToLibraryEvents(), p.subscribeToPlaybackEvents())
	case "library.sonic_analyzed":
		// Sonic analysis detected — enable sonic features and fetch related home content
		p.appCtx.Content.SetSonicAvailable(true)
		// Trigger home content fetches for Mixes/OnThisDay/MoodStations
		return p, tea.Batch(
			p.fetchMixesForYou(),
			p.fetchOnThisDay(),
			p.fetchMoodStations(),
			p.subscribeToLibraryEvents(),
			p.subscribeToPlaybackEvents(),
		)
	case "libraries.loaded":
		p.appCtx.Session.SetLibraries(msg.Libraries)
		if len(msg.Libraries) > 0 {
			p.appCtx.Session.SetSelectedLibrary(0)
			// Stats will be fetched on-demand when settings tab is viewed
			return p, tea.Batch(
				p.subscribeToLibraryEvents(),
				p.subscribeToPlaybackEvents(),
			)
		}
		// If no libraries, just subscribe to events
		return p, tea.Batch(
			p.subscribeToLibraryEvents(),
			p.subscribeToPlaybackEvents(),
		)
	case "recently_added.loaded":
		// Update the coordinator with domain albums directly
		p.appCtx.Content.SetAlbums(msg.Albums)

		items := make([]list.Item, len(msg.Albums))
		for i, a := range msg.Albums {
			items[i] = util.AlbumItem{Album: a}
		}
		// Note: msg.TotalSize here is the count of recently added items (e.g. 50),
		// not the total albums in the library. We should not overwrite the library stats.
		p.recentlyAddedComponent.SetItems(items)
		// Keep UI selection sane
		if len(msg.Albums) > 0 {
			p.appCtx.View.SetSelectedAlbum(0)
			p.recentlyAddedComponent.Select(0)
			// Reset last selected album index so first selection triggers a fetch
			p.lastSelectedAlbumIndex = -1
		}

	case "playlists.loaded":
		p.appCtx.Content.SetPlaylists(msg.Playlists)

		items := make([]list.Item, len(msg.Playlists))
		for i, pl := range msg.Playlists {
			items[i] = util.PlaylistItem{Playlist: pl}
		}

		if msg.TotalSize > 0 {
			p.appCtx.Content.SetPlaylistsTotal(msg.TotalSize)
		} else {
			p.appCtx.Content.SetPlaylistsTotal(len(msg.Playlists))
		}
		p.playlistComponent.SetItems(items)
		if len(msg.Playlists) > 0 {
			p.appCtx.View.SetSelectedPlaylist(0)
			p.playlistComponent.Select(0)
			// Reset last selected playlist index so first selection triggers a fetch
			p.lastSelectedPlaylistIndex = -1
		}

	case "mixes.loaded":
		p.appCtx.Content.SetMixesForYou(msg.Playlists)

	case "onthisday.loaded":
		p.appCtx.Content.SetOnThisDay(msg.Albums)

	case "moodstation.loaded":
		p.appCtx.Content.SetMoodStations(msg.Tracks)

	case "tracks.loaded":
		p.appCtx.Content.SetTracks(msg.Tracks)

		items := make([]list.Item, len(msg.Tracks))
		for i, t := range msg.Tracks {
			items[i] = util.TrackItem{Track: t}
		}

		if msg.TotalSize > 0 {
			p.appCtx.Content.SetTracksTotal(msg.TotalSize)
		} else {
			p.appCtx.Content.SetTracksTotal(len(msg.Tracks))
		}
		p.trackComponent.SetItems(items)
		if len(msg.Tracks) > 0 {
			p.appCtx.View.SetSelectedTrack(0)
			p.trackComponent.Select(0)
		}

		// Attempt to fetch cover art for the first track when tracks are loaded
		if len(msg.Tracks) > 0 {
			if msg.Tracks[0].Thumb != "" && p.appCtx.Playback.AlbumArtThumb() != msg.Tracks[0].Thumb {
				postCmd = p.fetchCoverArtCmd(msg.Tracks[0].Thumb)
			}
		}

		// If playback was requested immediately after a fetch, set queue and
		// kick off playback of the first track.
		if p.autoPlayOnTracksLoaded {
			p.autoPlayOnTracksLoaded = false
			// Build queue and queue items
			q := make([]domain.Track, len(msg.Tracks))
			copy(q, msg.Tracks)
			p.appCtx.Content.SetQueue(q)
			p.appCtx.Content.SetQueueIndex(0)
			p.queueComponent.UpdateListFromCoordinator()
			p.showingTracks = false
			p.appCtx.View.SetActiveTab(app.QueueTab)
			// Play first track asynchronously
			if len(q) > 0 {
				// Include postCmd (fetch cover art) alongside subscription/setup
				if postCmd != nil {
					return p, tea.Batch(
						append([]tea.Cmd{postCmd}, p.playTrack(&q[0]), p.subscribeToPlaybackEvents())...)
				}
				return p, tea.Batch(p.playTrack(&q[0]), p.subscribeToPlaybackEvents())
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
			p.appCtx.View.SetNotification(fmt.Sprintf("Library fetch error: %s", errMsg), "error", 10*time.Second)
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
			p.appCtx.Session.SetServers(msg.Servers)
		}

		// If we do not already have a sub/service for the selected server
		// but the coordinator already has a selection and a token, create it.
		srv := p.appCtx.Session.GetCurrentServer()
		if srv == nil && len(msg.Servers) > 0 {
			// If no selected server, default to the first server in the event
			// and mark it as selected.
			p.appCtx.Session.SetSelectedServer(0)
			srv = p.appCtx.Session.GetCurrentServer()
		}
		// Prefer server-specific access token if available, otherwise fall back to coordinator token
		token := ""
		if srv != nil && srv.AccessToken != "" {
			token = srv.AccessToken
		} else {
			token = p.appCtx.Session.Token()
		}
		if p.libSvc == nil && srv != nil && token != "" {
			baseURL := fmt.Sprintf("%s://%s", srv.Scheme, srv.Host)
			if srv.Port != "" {
				baseURL = fmt.Sprintf("%s:%s", baseURL, srv.Port)
			}
			p.libSvc = service.NewLibraryServiceWithEvents(baseURL, token, http.NewFactory())
			p.libEvtCh = p.libSvc.Subscribe(p.ctx)
			// Store in coordinator so media control wrapper can access it
			if p.appCtx != nil {
				p.appCtx.Services.SetLibraryService(p.libSvc)
			}

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
			p.appCtx.View.SetNotification(fmt.Sprintf("Load failed: %v", msg.Error), "error", 10*time.Second)
		} else {
			p.appCtx.View.SetNotification("Load failed", "error", 10*time.Second)
		}
	case "playback.play_failed":
		p.lastLoadFailed = true
		if msg.Error != nil {
			p.appCtx.View.SetNotification(fmt.Sprintf("Play failed: %v", msg.Error), "error", 10*time.Second)
		} else {
			p.appCtx.View.SetNotification("Play failed", "error", 10*time.Second)
		}
	case "playback.started":
		// Clear the load failed flag since playback started successfully
		p.lastLoadFailed = false
		// Record when this track started to help detect stale advance messages
		p.lastTrackStarted = time.Now()
		p.appCtx.Playback.SetState(app.PlaybackPlaying)
		p.appCtx.Playback.SetStreamPosition(0)
		if msg.Track != nil {
			track := util.DomainTrackToApp(msg.Track)
			p.appCtx.Playback.SetCurrentTrack(track)
			// Fetch the album art for the track now that playback started
			if track.Thumb != "" && p.appCtx.Playback.AlbumArtThumb() != track.Thumb {
				postCmd = p.fetchCoverArtCmd(track.Thumb)
			}
		}
		// Update queue UI to reflect the new playing track
		p.queueComponent.UpdateListFromCoordinator()
		// Start progress bar ticker for display updates
		return p, tea.Batch(
			postCmd,
			p.subscribeToPlaybackEvents(),
			p.subscribeToLibraryEvents(),
			p.startProgressTick(),
		)
	case "playback.resumed":
		p.appCtx.Playback.SetState(app.PlaybackPlaying)
		// Start progress bar ticker for display updates
		return p, tea.Batch(
			p.subscribeToPlaybackEvents(),
			p.subscribeToLibraryEvents(),
			p.startProgressTick(),
		)
	case "playback.paused":
		p.appCtx.Playback.SetState(app.PlaybackPaused)
	case "playback.stopped":
		p.appCtx.Playback.SetState(app.PlaybackStopped)
		// Clear the active playQueue when playback is explicitly stopped
		p.appCtx.Content.ClearActivePlayQueue()
	case "playback.volume_changed":
		// Playback service publishes floats — we don't keep it in coordinator as a primitive.
	case "playback.finished":
		// Auto-advance to the next queued track.
		// We check IsPlaying to ensure we don't auto-advance if the user explicitly stopped playback,
		// although playback.finished usually implies the stream ran to completion.
		// Also skip if the last load failed - this prevents double-skip when a failed decode
		// causes both load_failed and finished events to fire in quick succession.
		if p.appCtx.Playback.IsPlaying() && !p.lastLoadFailed {
			// Schedule advance with timestamp so we can detect stale messages
			scheduledAt := time.Now()
			// Re-subscribe to events so we're listening when playbackAdvanceMsg triggers
			// Return only a tick so that the test harness can call cmd() without
			// blocking on subscribe commands that read from channels.
			// Re-subscribe will be done when handling playbackAdvanceMsg.
			return p, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return playbackAdvanceMsg{scheduledAt: scheduledAt}
			})
		}
		// Reset the flag after checking so it doesn't affect future tracks
		p.lastLoadFailed = false
	case "playback.advance_next":
		// Legacy event type - redirect to playNext
		// This may still be used by external callers
		return p, tea.Batch(p.playNext(), p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
	case "playback.position":
		// Periodic position updates from the service.
		// Update stream position directly - TUI displays this value.
		if msg.Position >= 0 {
			p.appCtx.Playback.SetStreamPosition(msg.Position)
		}
		if msg.Duration > 0 {
			p.appCtx.Playback.SetStreamLength(msg.Duration)
		}
		if msg.SampleRate > 0 {
			p.appCtx.Playback.SetSampleRate(beep.SampleRate(msg.SampleRate))
		}
	case "playback.seeked":
		// Update position on seek
		if msg.Position >= 0 {
			p.appCtx.Playback.SetStreamPosition(msg.Position)
		}
	}
	// Re-subscribe to continue receiving playback/library events
	if postCmd != nil {
		return p, tea.Batch(postCmd, p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
	}
	return p, tea.Batch(p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
}
