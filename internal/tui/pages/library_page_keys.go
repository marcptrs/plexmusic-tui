package pages

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"charm.land/bubbles/v2/key"

	// "charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"plexmusic-tui/internal/app"
	domain "plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	"plexmusic-tui/internal/tui/util"
)

func (p *LibraryPage) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Early interception: route Up/Down (including k/j) to the queue handler if
	// the queue is explicitly in scope (modal visible OR explicit queue focus OR visible in layout).
	// This takes precedence over other list handlers so Up/Down will scroll the queue.
	if (key.Matches(msg, p.keys.Up) ||
		key.Matches(msg, p.keys.Down) ||
		isRuneKey(msg, 'k') ||
		isRuneKey(msg, 'j')) && p.appCtx != nil {
		if p.appCtx.View.ShowQueueModal() || p.IsFocusedQueue() || p.isQueueVisible() {
			if len(p.appCtx.Content.Queue()) > 0 {
				// If we are now routing Up/Down to the queue, prefer to mark the queue
				// as focused so subsequent logic skips updating the left-side lists.
				p.SetFocusedQueue(true)
				return p, p.handleQueueKeyMsg(msg)
			}
		}
	}

	// Let the search component handle keys while on Search tab.
	if p.appCtx.View.ActiveTab() == app.SearchTab {
		switch msg.String() {
		case "1", "2", "3", "4", "5", "6":
			// Allow these to fall through into the main key handling below
		default:
			var cmd tea.Cmd
			_, cmd = p.searchComponent.Update(msg)

			// If user hits Esc, abort and return to Home
			if msg.String() == "esc" {
				p.searchComponent.Blur()
				p.appCtx.View.SetActiveTab(app.HomeTab)
			}
			return p, cmd
		}
	}

	// Page-level key handling.
	var cmd tea.Cmd

	// Check global keys first
	switch {
	case p.appCtx != nil &&
		(p.appCtx.View.ShowQueueModal() || p.IsFocusedQueue() || p.isQueueVisible()) &&
		len(p.appCtx.Content.Queue()) > 0 &&
		(key.Matches(msg, p.keys.Up) ||
			key.Matches(msg, p.keys.Down) ||
			key.Matches(msg, p.keys.PlaySelected) ||
			key.Matches(msg, p.keys.QueueMoveUp) ||
			key.Matches(msg, p.keys.QueueMoveDown) ||
			key.Matches(msg, p.keys.QueueRemove) ||
			key.Matches(msg, p.keys.Enter)):
		// When the queue modal is visible, or when the queue has explicit keyboard focus,
		// or when the queue is visible as a pane, route queue-specific keys to the unified handler.
		// This overrides other lists when the queue has explicit focus or is visible to the user.
		return p, p.handleQueueKeyMsg(msg)
	case p.appCtx != nil && p.appCtx.View.ShowQueueModal() && key.Matches(msg, p.keys.Back):
		// Close queue modal on 'Back' key when visible
		p.appCtx.View.SetShowQueueModal(false)
		p.SetFocusedQueue(false)
		// Continue to fallback: no selection handled by libSvc branch.
	case key.Matches(msg, p.keys.Back):
		// If the queue modal is open, close it first; otherwise go back to server selection.
		if p.appCtx.View.ShowQueueModal() {
			p.appCtx.View.SetShowQueueModal(false)
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

		// If playback is initializing, cancel it instead of exiting to previous page.
		if p.playbackInitializing {
			if p.playbackInitCancel != nil {
				p.playbackInitCancel()
			}
			p.playbackInitializing = false
			p.appCtx.View.SetNotification("Playback initialization canceled", "info", 3*time.Second)
			return p, nil
		}
		return p, func() tea.Msg {
			return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
		}

	case key.Matches(msg, p.keys.Play):
		// Debounce Play key events to avoid auto-repeat toggles (e.g., if key is held).
		if !p.lastPlayKey.IsZero() && time.Since(p.lastPlayKey) < 250*time.Millisecond {
			return p, nil
		}
		p.lastPlayKey = time.Now()

		if p.orchestrator == nil {
			p.appCtx.View.SetNotification(
				"Playback unavailable: orchestrator is not initialized",
				"error",
				5*time.Second,
			)
			return p, nil
		}

		if p.appCtx.Playback.IsPlaying() {
			if p.orchestrator != nil {
				err := p.orchestrator.Pause()
				if err != nil {
					p.appCtx.View.SetNotification(fmt.Sprintf("Pause failed: %v", err), "error", 5*time.Second)
				}
				// Regardless of error state, update coordinator playback state so UI reflects intended pause
				p.appCtx.Playback.SetState(app.PlaybackPaused)
			} else {
				p.appCtx.View.SetNotification("Pause failed: playback orchestrator unavailable", "error", 5*time.Second)
			}
			return p, nil
		}
		if p.appCtx.Playback.IsPaused() {
			if p.orchestrator != nil {
				err := p.orchestrator.Resume()
				if err != nil {
					p.appCtx.View.SetNotification(fmt.Sprintf("Resume failed: %v", err), "error", 5*time.Second)
				} else {
					// If resume didn't lead to a playing state, try to restart current track
					if p.orchestrator.GetState() != domain.PlaybackPlaying {
						if tr := p.appCtx.Playback.CurrentTrack(); tr != nil {
							// Convert app.Track to domain.Track for playTrack
							dt := util.AppTrackToDomain(tr)
							return p, func() tea.Msg {
								err := p.orchestrator.PlayDomainTrack(context.Background(), p.libSvc, dt)
								return playResultMsg{Err: err}
							}
						}
					}
					// Ensure UI shows playing state after resume attempt
					p.appCtx.Playback.SetState(app.PlaybackPlaying)
				}
			} else {
				p.appCtx.View.SetNotification("Resume failed: playback orchestrator unavailable", "error", 5*time.Second)
			}
			return p, nil
		}
		// Stopped: try to start playback. First prefer current track, else try to play
		// based on the currently selected item (mirroring Space behavior)
		if !p.appCtx.Playback.IsPlaying() && !p.appCtx.Playback.IsPaused() {
			// Try to restart current track if present
			if p.appCtx.Playback.HasCurrentTrack() {
				if tr := p.appCtx.Playback.CurrentTrack(); tr != nil {
					// Convert app.Track to domain.Track for playTrack
					dt := util.AppTrackToDomain(tr)
					return p, p.playTrack(dt)
				}
			}
			active := p.appCtx.View.ActiveTab()
			// If Showing Tracks, play selected track
			if p.showingTracks {
				if item, ok := p.trackComponent.SelectedItem().(util.TrackItem); ok {
					// Use the selected item directly
					dt := &item.Track
					tracks := p.appCtx.Content.Tracks()
					selIdx := p.trackComponent.Index()
					if len(tracks) == 0 {
						return p, nil
					}
					if selIdx < 0 || selIdx >= len(tracks) {
						selIdx = 0
					}
					newQueue := make([]domain.Track, len(tracks)-selIdx)
					copy(newQueue, tracks[selIdx:])
					if dt != nil && len(newQueue) > 0 {
						newQueue[0] = *dt
					}
					p.appCtx.Content.SetQueue(newQueue)
					p.appCtx.Content.SetQueueIndex(0)
					p.queueComponent.UpdateListFromCoordinator()
					p.appCtx.View.SetSelectedTrack(selIdx)
					p.showingTracks = false
					p.appCtx.View.SetActiveTab(app.QueueTab)
					if len(newQueue) > 0 {
						return p, p.playTrack(&newQueue[0])
					}
					return p, nil
				}
			}
			// Else, if home/playlist/library selected, trigger fetch-and-play (async)
			if p.libSvc != nil && (active == app.PlaylistsTab || active == app.HomeTab || active == app.LibraryTab) {
				switch active {
				case app.PlaylistsTab:
					if item, ok := p.playlistComponent.SelectedItem().(util.PlaylistItem); ok {
						p.playbackInitializing = true
						p.autoPlayOnTracksLoaded = true
						return p, tea.Batch(p.fetchTracksCmd(item.Playlist.Key), p.spinner.Tick)
					} else if p.appCtx != nil {
						// Fallback to the coordinator's selected playlist.
						sel := p.appCtx.View.SelectedPlaylist()
						if sel >= 0 && sel < len(p.appCtx.Content.Playlists()) {
							key := p.appCtx.Content.Playlists()[sel].Key
							p.playbackInitializing = true
							p.autoPlayOnTracksLoaded = true
							return p, tea.Batch(p.fetchTracksCmd(key), p.spinner.Tick)
						}
					}
				case app.HomeTab:
					if item := p.homeComponent.SelectedItem(); item != nil {
						// Check if this is a station - use StartStationPlayback for continuous playback
						if item.Type == "station" {
							return p, p.startStationPlaybackCmd(item.Key)
						}
						// For albums and other items, use regular fetch
						p.playbackInitializing = true
						p.autoPlayOnTracksLoaded = true
						return p, tea.Batch(p.fetchTracksCmd(item.Key), p.spinner.Tick)
					}
				case app.LibraryTab:
					if item, ok := p.recentlyAddedComponent.SelectedItem().(util.AlbumItem); ok {
						p.playbackInitializing = true
						p.autoPlayOnTracksLoaded = true
						return p, tea.Batch(p.fetchTracksCmd(item.Album.Key), p.spinner.Tick)
					}
				}
			}
		}
	case key.Matches(msg, p.keys.PlaySelected):
		// If the Queue tab is active and the user presses space, treat it as
		// a "play selected" from the queue: remove previous entries up to the
		// selected item and begin playing that track.
		active := p.appCtx.View.ActiveTab()
		if active == app.QueueTab {
			sel := p.queueComponent.Index()
			q := p.appCtx.Content.Queue()
			if len(q) == 0 || sel < 0 || sel >= len(q) {
				return p, nil
			}
			// If selected is not the first item, trim the queue to the selected item and onward
			if sel > 0 {
				newQueue := make([]domain.Track, len(q)-sel)
				copy(newQueue, q[sel:])
				p.appCtx.Content.SetQueue(newQueue)
				p.appCtx.Content.SetQueueIndex(0)
				p.queueComponent.UpdateListFromCoordinator()
				p.queueComponent.Select(0)
				return p, p.playTrack(&newQueue[0])
			}
			// Selected is the first item — just play it
			if item, ok := p.queueComponent.SelectedItem().(util.QueueItem); ok {
				return p, p.playTrack(&item.Track)
			}
			return p, nil
		}

		// Try to play the selected/first track from album, playlist or queue.
		if p.showingTracks {
			if item, ok := p.trackComponent.SelectedItem().(util.TrackItem); ok {
				// Use the selected item directly
				dt := &item.Track

				// If the user chooses to play a track while viewing an album/playlist,
				// build a queue from the selected track to the end of the list so the
				// subsequent tracks will be played automatically.
				tracks := p.appCtx.Content.Tracks()
				selIdx := p.trackComponent.Index()
				if len(tracks) == 0 {
					return p, nil
				}
				if selIdx < 0 || selIdx >= len(tracks) {
					selIdx = 0
				}
				newQueue := make([]domain.Track, len(tracks)-selIdx)
				copy(newQueue, tracks[selIdx:])

				// Ensure the first queued track matches the selected item
				if dt != nil && len(newQueue) > 0 {
					newQueue[0] = *dt
				}

				// Persist queue & selection to coordinator
				p.appCtx.Content.SetQueue(newQueue)
				p.appCtx.Content.SetQueueIndex(0)
				p.queueComponent.UpdateListFromCoordinator()

				// Make sure the underlying tracklist selection is reflected in coordinator
				p.appCtx.View.SetSelectedTrack(selIdx)
				p.showingTracks = false
				p.appCtx.View.SetActiveTab(app.QueueTab)

				// Play the first track in the newly created queue (the selected track)
				if len(newQueue) > 0 {
					return p, p.playTrack(&newQueue[0])
				}
			}
			return p, nil
		}

		active = p.appCtx.View.ActiveTab()
		if p.libSvc != nil && (active == app.PlaylistsTab || active == app.HomeTab || active == app.LibraryTab) {
			switch active {
			case app.PlaylistsTab:
				if item, ok := p.playlistComponent.SelectedItem().(util.PlaylistItem); ok {
					// Start async fetch and auto-play when tracks are loaded
					p.playbackInitializing = true
					p.autoPlayOnTracksLoaded = true
					return p, tea.Batch(p.fetchTracksCmd(item.Playlist.Key), p.spinner.Tick)
				}
			case app.HomeTab:
				// Home tab: use homeComponent.SelectedItem()
				if item := p.homeComponent.SelectedItem(); item != nil {
					// Check if this is a station - use StartStationPlayback for continuous playback
					if item.Type == "station" {
						return p, p.startStationPlaybackCmd(item.Key)
					}
					// Start async fetch and auto-play when tracks are loaded
					p.playbackInitializing = true
					p.autoPlayOnTracksLoaded = true
					return p, tea.Batch(p.fetchTracksCmd(item.Key), p.spinner.Tick)
				}
				// Fallback to coordinator selection if homeComponent doesn't have an item
				if p.appCtx != nil {
					sel := p.appCtx.View.SelectedAlbum()
					if sel >= 0 && sel < len(p.appCtx.Content.Albums()) {
						key := p.appCtx.Content.Albums()[sel].Key
						p.playbackInitializing = true
						p.autoPlayOnTracksLoaded = true
						return p, tea.Batch(p.fetchTracksCmd(key), p.spinner.Tick)
					}
				}
			case app.LibraryTab:
				// Library tab: use selected album from recentlyAddedComponent
				if item, ok := p.recentlyAddedComponent.SelectedItem().(util.AlbumItem); ok {
					// Start async fetch and auto-play when tracks are loaded
					p.playbackInitializing = true
					p.autoPlayOnTracksLoaded = true
					return p, tea.Batch(p.fetchTracksCmd(item.Album.Key), p.spinner.Tick)
				}
			}
		}

		// Fallback: play selected track from Tracks or Queue.
		tracks := p.appCtx.Content.Tracks()
		if len(tracks) > 0 {
			idx := p.appCtx.View.SelectedTrack()
			if idx < 0 || idx >= len(tracks) {
				idx = 0
			}
			tr := &tracks[idx]
			return p, p.playTrack(tr)
		}
		return p, nil

	case key.Matches(msg, p.keys.Next):
		return p, tea.Batch(p.playNext(), p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
	case key.Matches(msg, p.keys.Prev):
		return p, tea.Batch(p.playPrev(), p.subscribeToPlaybackEvents(), p.subscribeToLibraryEvents())
	case key.Matches(msg, p.keys.SeekForward):
		if p.appCtx.Services.PlaybackService() != nil && p.orchestrator != nil {
			svc := p.appCtx.Services.PlaybackService()
			// Guard against nil pointer wrapped in interface
			if reflect.ValueOf(svc).Kind() == reflect.Ptr && reflect.ValueOf(svc).IsNil() {
				return p, nil
			}

			pos := svc.GetPosition()
			duration := svc.GetDuration()
			sr := svc.SampleRate()
			if sr == 0 {
				sr = 44100
			}

			// Seek +10 seconds (in samples)
			newPos := pos + (10 * sr)
			if newPos > duration {
				newPos = duration
			}

			return p, func() tea.Msg {
				defer func() {
					if r := recover(); r != nil {
						// TODO: Add logging - intentionally empty for now
						_ = r // satisfy linter
					}
				}()
				if err := p.orchestrator.Seek(newPos); err != nil {
					// TODO: Add logging - intentionally empty for now
					_ = err // satisfy linter
				}

				return nil
			}
		}
		return p, nil
	case key.Matches(msg, p.keys.SeekBackward):
		if p.appCtx.Services.PlaybackService() != nil && p.orchestrator != nil {
			svc := p.appCtx.Services.PlaybackService()
			// Guard against nil pointer wrapped in interface
			if reflect.ValueOf(svc).Kind() == reflect.Ptr && reflect.ValueOf(svc).IsNil() {
				return p, nil
			}

			pos := svc.GetPosition()
			sr := svc.SampleRate()
			if sr == 0 {
				sr = 44100
			}

			// Seek -10 seconds (in samples)
			newPos := pos - (10 * sr)
			if newPos < 0 {
				newPos = 0
			}

			return p, func() tea.Msg {
				defer func() {
					if r := recover(); r != nil {
						// TODO: Add logging - intentionally empty for now
						_ = r // satisfy linter
					}
				}()
				if err := p.orchestrator.Seek(newPos); err != nil {
					// TODO: Add logging - intentionally empty for now
					_ = err // satisfy linter
				}

				return nil
			}
		}
	case key.Matches(msg, p.keys.VolumeUp):
		p.adjustVolumeByPercent(5)
		return p, nil
	case key.Matches(msg, p.keys.VolumeDown):
		p.adjustVolumeByPercent(-5)
		return p, nil
	case key.Matches(msg, p.keys.Queue) || isRuneKey(msg, 'o') || isRuneKey(msg, 'O'):
		// If the modal is visible, close it and clear focus
		if p.appCtx.View.ShowQueueModal() {
			p.appCtx.View.SetShowQueueModal(false)
			p.SetFocusedQueue(false)
			return p, nil
		}
		// If the active tab is the Queue tab, toggle queue focus instead of opening modal.
		// This allows Up/Down to be routed to the queue lists when focused.
		if p.appCtx.View.ActiveTab() == app.QueueTab {
			p.SetFocusedQueue(!p.IsFocusedQueue())
			if p.IsFocusedQueue() {
				p.focusedNowPlaying = false // don't keep both focused states set
			}
			return p, nil
		}
		// Otherwise, open/close the queue modal and set focus in a consistent manner.
		showing := !p.appCtx.View.ShowQueueModal()
		p.appCtx.View.SetShowQueueModal(showing)
		p.SetFocusedQueue(showing)
		return p, nil
	case key.Matches(msg, p.keys.Refresh):
		// Refresh library lists (recently added + playlists)
		if p.libSvc != nil {
			return p, tea.Batch(p.fetchRecentlyAdded(), p.fetchPlaylists())
		}
		return p, nil
	case key.Matches(msg, p.keys.Detect):
		// Re-run sonic detection and return a `sonicDetectResultMsg` indicating
		// whether sonic analysis was detected. The Update handler will translate
		// the result into follow-up actions (fetch content or show a notification).
		if p.libSvc == nil {
			return p, nil
		}
		return p, func() tea.Msg {
			ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
			defer cancel()
			ok, _ := p.libSvc.HasSonicAnalysis(ctx)
			if p.appCtx != nil {
				p.appCtx.Content.SetSonicAvailable(ok)
			}
			return sonicDetectResultMsg{ok: ok}
		}

	case key.Matches(msg, p.keys.Search):
		// Toggle Search tab and focus/blur the search input accordingly
		if p.appCtx.View.ActiveTab() == app.SearchTab {
			p.searchComponent.Blur()
			p.appCtx.View.SetActiveTab(app.HomeTab)
		} else {
			p.appCtx.View.SetActiveTab(app.SearchTab)
			p.searchComponent.Focus()
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
			p.appCtx.View.SetActiveTab(app.HomeTab)
		case "2":
			p.appCtx.View.SetActiveTab(app.LibraryTab)
		case "3":
			p.appCtx.View.SetActiveTab(app.PlaylistsTab)
		case "4":
			p.appCtx.View.SetActiveTab(app.SearchTab)
		case "5":
			p.appCtx.View.SetActiveTab(app.QueueTab)
		case "6":
			p.appCtx.View.SetActiveTab(app.SettingsTab)
		}
		// For browsing-focused tabs, open the drawer; queue tab should keep drawer closed
		switch sw {
		case "1", "2", "3", "4", "6":
			p.drawerOpen = true
			// Clear queue focus when switching to non-queue tabs
			p.SetFocusedQueue(false)
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
	active := p.appCtx.View.ActiveTab()

	// In Settings tab, handle quick shortcuts (e.g., 'c' toggles cover-art side).
	if active == app.SettingsTab {
		if km := msg; true {
			switch km.String() {
			case "c":
				if p.appCtx != nil && p.appCtx.Services.ConfigManager() != nil {
					cur := p.appCtx.Services.ConfigManager().GetCoverArtPosition()
					if cur == "left" {
						p.appCtx.Services.ConfigManager().SetCoverArtPosition("right")
					} else {
						p.appCtx.Services.ConfigManager().SetCoverArtPosition("left")
					}
					_ = p.appCtx.Services.ConfigManager().Save()
					p.appCtx.View.SetNotification("Cover art position updated", "success", 3*time.Second)
				}
				return p, nil
			}
		}
	}

	if p.showingTracks {
		if !p.IsFocusedQueue() && !p.appCtx.View.ShowQueueModal() {
			// Ensure track component is focused to receive key events
			p.trackComponent.SetFocused(true)
			oldSel := p.trackComponent.Index()
			_, cmd = p.trackComponent.Update(msg)
			newSel := p.trackComponent.Index()
			p.appCtx.View.SetSelectedTrack(newSel)

			// If selection changed, update the last selected index
			if newSel != oldSel {
				p.lastSelectedTrackIndex = newSel
				// Note: We don't fetch cover art here as it would interfere with
				// the currently playing track's art. Art is fetched on playback.started.
			}

			// Enter on track list does nothing (dedicated to menu operation elsewhere).
			// Playback is triggered via Space/P.
			return p, cmd
		}
		// Queue is focused, ignore tracklist nav keys
		return p, nil
	}

	switch active {
	case app.HomeTab:
		if !p.IsFocusedQueue() && !p.appCtx.View.ShowQueueModal() {
			p.homeComponent.SetFocused(true)
			_, cmd = p.homeComponent.Update(msg)

			// Handle Enter key for home items
			if key.Matches(msg, p.keys.Enter) {
				if item := p.homeComponent.SelectedItem(); item != nil {
					switch item.Type {
					case "album":
						// Fetch tracks for the album
						if p.libSvc != nil {
							reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
							defer cancel()
							_, _, _ = p.libSvc.FetchTracks(reqCtx, item.Key)
							p.showingTracks = true
						}
					case "station":
						// Play the station with continuous playback support (async)
						return p, p.startStationPlaybackCmd(item.Key)
					}
				}
			}
		}
		return p, cmd

	case app.LibraryTab:
		if !p.IsFocusedQueue() && !p.appCtx.View.ShowQueueModal() {
			p.recentlyAddedComponent.SetFocused(true)
			_, cmd = p.recentlyAddedComponent.Update(msg)
			newIdx := p.recentlyAddedComponent.Index()
			p.appCtx.View.SetSelectedAlbum(newIdx)

			// If selection changed, fetch tracks for the selected album in the background
			if item, ok := p.recentlyAddedComponent.SelectedItem().(util.AlbumItem); ok {
				newIdx := p.recentlyAddedComponent.Index()
				if newIdx != p.lastSelectedAlbumIndex && p.libSvc != nil {
					p.lastSelectedAlbumIndex = newIdx
					cmd = tea.Batch(cmd, p.fetchTracksCmd(item.Album.Key))
					// Note: We don't fetch cover art here as it would interfere with
					// the currently playing track's art. Art is fetched on playback.started.
				}
			}
		}

		if key.Matches(msg, p.keys.Enter) {
			if item, ok := p.recentlyAddedComponent.SelectedItem().(util.AlbumItem); ok {
				if p.libSvc != nil {
					reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
					defer cancel()
					_, _, _ = p.libSvc.FetchTracks(reqCtx, item.Album.Key)
					p.showingTracks = true
				}
			}
		}

	case app.PlaylistsTab:
		if !p.IsFocusedQueue() && !p.appCtx.View.ShowQueueModal() {
			_, cmd = p.playlistComponent.Update(msg)
			p.appCtx.View.SetSelectedPlaylist(p.playlistComponent.Index())

			if item, ok := p.playlistComponent.SelectedItem().(util.PlaylistItem); ok {
				newIdx := p.playlistComponent.Index()
				if newIdx != p.lastSelectedPlaylistIndex && p.libSvc != nil {
					p.lastSelectedPlaylistIndex = newIdx
					cmd = tea.Batch(cmd, p.fetchTracksCmd(item.Playlist.Key))
				}
			}
		}

		if key.Matches(msg, p.keys.Enter) {
			if item, ok := p.playlistComponent.SelectedItem().(util.PlaylistItem); ok {
				if p.libSvc != nil {
					reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
					defer cancel()
					_, _, _ = p.libSvc.FetchTracks(reqCtx, item.Playlist.Key)
					p.showingTracks = true
				}
			}
		}

	case app.QueueTab:
		p.queueComponent.SetFocused(true)
		_, cmd = p.queueComponent.Update(msg)
		return p, cmd
	case app.SettingsTab:
		_, cmd = p.settingsComponent.Update(msg)
	}

	return p, cmd
}

// Helper to detect rune keypresses in a KeyMsg when the bubbles/list key mapping
// doesn't directly express a specific rune. Useful for supporting both "k"/"j"
// navigation as rune fallbacks where clients send runes instead of key events.
func isRuneKey(msg tea.KeyMsg, r rune) bool {
	key := msg.Key()
	if key.Text == "" {
		return false
	}
	// Only accept a single rune to prevent conflicts with multi-rune sequences
	return len(key.Text) == 1 && []rune(key.Text)[0] == r
}

// Helper: play the provided domain.Track (UI & playback service)
func (p *LibraryPage) playTrack(dt *domain.Track) tea.Cmd {
	if dt == nil {
		return nil
	}
	// Convert to app.Track for UI/Orchestrator compatibility
	at := util.DomainTrackToApp(dt)
	if at == nil {
		return nil
	}

	// Update UI coordinator state preemptively for immediate UI feedback — playback.started will reconcile state later
	p.appCtx.Playback.SetCurrentTrack(at)
	p.appCtx.Playback.SetState(app.PlaybackPlaying)

	var cmds []tea.Cmd

	// Fetch cover art if available
	if at.Thumb != "" {
		cmds = append(cmds, p.fetchCoverArtCmd(at.Thumb))
	}

	// Orchestrator is required to perform playback orchestration
	// Run the orchestrator in a background tea.Cmd to avoid blocking the UI.
	var playCmd tea.Cmd
	if p.orchestrator != nil {
		// mark initialization, spinner will show; create cancellable ctx for startup
		p.playbackInitializing = true
		var initCtx context.Context
		initCtx, p.playbackInitCancel = context.WithCancel(p.ctx)
		playCmd = func() tea.Msg {
			err := p.orchestrator.PlayAppTrack(initCtx, at)
			return playResultMsg{Err: err}
		}
	} else {
		// Orchestrator missing: notify user
		p.appCtx.View.SetNotification("Play failed: playback orchestrator unavailable", "error", 10*time.Second)
		return nil
	}
	// Also kick the spinner clock while initializing playback
	return tea.Batch(append(cmds, playCmd, p.spinner.Tick)...)
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
	if p.appCtx == nil {
		return false
	}
	// Queue modal takes precedence
	if p.appCtx.View.ShowQueueModal() {
		return true
	}
	// Queue tab active is treated as visible
	if p.appCtx.View.ActiveTab() == app.QueueTab {
		return true
	}
	// Compute cover art position from config manager and determine whether the
	// queue content is shown as a secondary pane (right or left) depending on the
	// configured layout and whether drawers/tracklist are displayed.
	pos := "left"
	if p.appCtx.Services.ConfigManager() != nil {
		pos = p.appCtx.Services.ConfigManager().GetCoverArtPosition()
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
	p.queueComponent.SetFocused(true)

	// Delegate to queue component - wrap returned cmd so we can track spinner
	// Track selection changes so we can fetch cover art for the selected item
	oldSel := p.queueComponent.Index()
	_, cmd := p.queueComponent.Update(msg)
	newSel := p.queueComponent.Index()

	var cmds []tea.Cmd
	if cmd != nil {
		// If component returned a cmd (likely to start playback), mark initializing
		p.playbackInitializing = true
		cmds = append(cmds, cmd, p.spinner.Tick)
	}

	// If the selection changed, update tracking only; do not update cover art —
	// cover art is updated only when the track is actually played.
	if newSel != oldSel {
		p.lastSelectedQueueIndex = newSel
		// Ensure we return a non-nil command so the caller can execute and
		// advance the spinner or update the UI even when the list update
		// didn't provide a cmd. This matches previous behavior and test
		// expectations.
		if len(cmds) == 0 {
			cmds = append(cmds, p.spinner.Tick)
		}
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

// playNext advances to the next track in the queue or tracklist.
// If in station playback mode, it also triggers a playQueue refresh to get new tracks.
func (p *LibraryPage) playNext() tea.Cmd {
	if p.orchestrator == nil {
		return nil
	}

	// If we're in station playback mode, refresh the playQueue to get new tracks
	// Only refresh when we're approaching the end of the queue (within 5 tracks)
	var refreshCmd tea.Cmd
	isStation := p.appCtx.Content.IsStationPlayback()
	hasLibSvc := p.libSvc != nil

	if isStation && hasLibSvc {
		activeQueue := p.appCtx.Content.ActivePlayQueue()
		if activeQueue != nil && activeQueue.PlayQueueID > 0 {
			queue := p.appCtx.Content.Queue()
			currentIdx := p.appCtx.Content.QueueIndex()
			nextIdx := currentIdx + 1
			tracksRemaining := len(queue) - nextIdx - 1 // tracks after the one we're about to play

			// Only refresh when we're within 5 tracks of the end
			const refreshThreshold = 5
			shouldRefresh := tracksRemaining <= refreshThreshold

			var selectedItemID int
			if nextIdx < len(queue) {
				selectedItemID = queue[nextIdx].PlayQueueItemID
			}

			if shouldRefresh {
				refreshCmd = p.refreshPlayQueueCmd(activeQueue.PlayQueueID, selectedItemID)
			}
		} else {
			// TODO: Add logging - no refresh needed
			_ = activeQueue // satisfy linter
		}
	} else {
		// TODO: Add logging - not in station mode
		_ = isStation // satisfy linter
	}

	// PlaybackController for navigation logic doesn't need the service
	pc := service.NewPlaybackController(nil)
	err := p.orchestrator.PlayNext(
		p.ctx,
		pc,
		p.appCtx.Content.Queue(),
		p.appCtx.Content.QueueIndex(),
		p.appCtx.Content.Tracks(),
		p.appCtx.View.SelectedTrack(),
	)
	if err != nil {
		// TODO: Add logging
		_ = err // satisfy linter
	}

	return refreshCmd
}

// playPrev goes to the previous track.
func (p *LibraryPage) playPrev() tea.Cmd {
	if p.orchestrator == nil {
		return nil
	}
	// PlaybackController for navigation logic doesn't need the service
	pc := service.NewPlaybackController(nil)
	err := p.orchestrator.PlayPrev(
		p.ctx,
		pc,
		p.appCtx.Content.Queue(),
		p.appCtx.Content.QueueIndex(),
		p.appCtx.Content.Tracks(),
		p.appCtx.View.SelectedTrack(),
	)
	if err != nil {
		// TODO: Add logging
		_ = err // satisfy linter
	}
	return nil
}

// adjustVolumeByPercent modifies the volume by the given delta percentage.
func (p *LibraryPage) adjustVolumeByPercent(delta int) {
	if p.orchestrator == nil {
		return
	}
	p.orchestrator.AdjustVolume(float64(delta))
}
