package pages

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/charmbracelet/bubbles/key"

	// "github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	log "github.com/charmbracelet/log/v2"

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
		isRuneKey(msg, 'j')) && p.coordinator != nil {
		if p.coordinator.ShowQueueModal() || p.IsFocusedQueue() || p.isQueueVisible() {
			if len(p.coordinator.Queue()) > 0 {
				// If we are now routing Up/Down to the queue, prefer to mark the queue
				// as focused so subsequent logic skips updating the left-side lists.
				p.SetFocusedQueue(true)
				return p, p.handleQueueKeyMsg(msg)
			}
		}
	}

	// Let the search component handle keys while on Search tab.
	if p.coordinator.ActiveTab() == app.SearchTab {
		switch msg.String() {
		case "1", "2", "3", "4", "5", "6":
			// Allow these to fall through into the main key handling below
		default:
			var cmd tea.Cmd
			_, cmd = p.searchComponent.Update(msg)

			// If user hits Esc, abort and return to Home
			if msg.String() == "esc" {
				p.searchComponent.Blur()
				p.coordinator.SetActiveTab(app.HomeTab)
			}
			return p, cmd
		}
	}

	// Page-level key handling.
	var cmd tea.Cmd

	// Check global keys first
	switch {
	case p.coordinator != nil &&
		(p.coordinator.ShowQueueModal() || p.IsFocusedQueue() || p.isQueueVisible()) &&
		len(p.coordinator.Queue()) > 0 &&
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
	case p.coordinator != nil && p.coordinator.ShowQueueModal() && key.Matches(msg, p.keys.Back):
		// Close queue modal on 'Back' key when visible
		p.coordinator.SetShowQueueModal(false)
		p.SetFocusedQueue(false)
		// Continue to fallback: no selection handled by libSvc branch.
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

		// If playback is initializing, cancel it instead of exiting to previous page.
		if p.playbackInitializing {
			if p.playbackInitCancel != nil {
				p.playbackInitCancel()
			}
			p.playbackInitializing = false
			p.coordinator.SetNotification("Playback initialization canceled", "info", 3*time.Second)
			return p, nil
		}
		return p, func() tea.Msg {
			return tui.PageChangeMsg{ID: tui.ServerSelectionPageID}
		}

	case key.Matches(msg, p.keys.Play):
		log.Info("Play key matched", "msg", msg, "activeTab", p.coordinator.ActiveTab())
		// Debounce Play key events to avoid auto-repeat toggles (e.g., if key is held).
		if !p.lastPlayKey.IsZero() && time.Since(p.lastPlayKey) < 250*time.Millisecond {
			log.Debug("Play key: ignored due to debounce", "elapsed_ms", time.Since(p.lastPlayKey).Milliseconds())
			return p, nil
		}
		p.lastPlayKey = time.Now()

		if p.orchestrator == nil {
			p.coordinator.SetNotification(
				"Playback unavailable: orchestrator is not initialized",
				"error",
				5*time.Second,
			)
			return p, nil
		}

		log.Debug("Play key: current state",
			"isPlaying", p.coordinator.IsPlaying(),
			"isPaused", p.coordinator.IsPaused(),
			"orchestrator_set", p.orchestrator != nil) // Toggle playback if already active.
		if p.coordinator.IsPlaying() {
			if p.orchestrator != nil {
				err := p.orchestrator.Pause()
				log.Debug("Play key: pause called", "err", err)
				if err != nil {
					p.coordinator.SetNotification(fmt.Sprintf("Pause failed: %v", err), "error", 5*time.Second)
				}
				// Regardless of error state, update coordinator playback state so UI reflects intended pause
				p.coordinator.SetPlaybackState(app.PlaybackPaused)
			} else {
				p.coordinator.SetNotification("Pause failed: playback orchestrator unavailable", "error", 5*time.Second)
			}
			return p, nil
		}
		if p.coordinator.IsPaused() {
			if p.orchestrator != nil {
				err := p.orchestrator.Resume()
				log.Debug("Play key: resume called", "err", err)
				if err != nil {
					p.coordinator.SetNotification(fmt.Sprintf("Resume failed: %v", err), "error", 5*time.Second)
				} else {
					// If resume didn't lead to a playing state, try to restart current track
					if p.orchestrator.GetState() != domain.PlaybackPlaying {
						if tr := p.coordinator.CurrentTrack(); tr != nil {
							at := tr
							return p, func() tea.Msg {
								err := p.orchestrator.PlayAppTrack(context.Background(), at)
								return playResultMsg{Err: err}
							}
						}
					}
					// Ensure UI shows playing state after resume attempt
					p.coordinator.SetPlaybackState(app.PlaybackPlaying)
				}
			} else {
				p.coordinator.SetNotification("Resume failed: playback orchestrator unavailable", "error", 5*time.Second)
			}
			return p, nil
		}
		// Stopped: try to start playback. First prefer current track, else try to play
		// based on the currently selected item (mirroring Space behavior)
		if !p.coordinator.IsPlaying() && !p.coordinator.IsPaused() {
			log.Info(
				"Play key: stopped state, starting new playback",
				"hasCurrentTrack",
				p.coordinator.HasCurrentTrack(),
			)
			// Try to restart current track if present
			if p.coordinator.HasCurrentTrack() {
				log.Info("Play key: playing current track")
				if tr := p.coordinator.CurrentTrack(); tr != nil {
					return p, p.playAppTrack(tr)
				}
			}
			active := p.coordinator.ActiveTab()
			// If Showing Tracks, play selected track
			if p.showingTracks {
				if item, ok := p.trackComponent.SelectedItem().(util.TrackItem); ok {
					at := util.DomainTrackToApp(&item.Track)
					tracks := p.coordinator.Tracks()
					selIdx := p.trackComponent.Index()
					if len(tracks) == 0 {
						return p, nil
					}
					if selIdx < 0 || selIdx >= len(tracks) {
						selIdx = 0
					}
					newQueue := make([]app.Track, len(tracks)-selIdx)
					copy(newQueue, tracks[selIdx:])
					if at != nil && len(newQueue) > 0 {
						newQueue[0] = *at
					}
					p.coordinator.SetQueue(newQueue)
					p.coordinator.SetQueueIndex(0)
					p.queueComponent.UpdateListFromCoordinator()
					p.coordinator.SetSelectedTrack(selIdx)
					p.showingTracks = false
					p.coordinator.SetActiveTab(app.QueueTab)
					if len(newQueue) > 0 {
						return p, p.playAppTrack(&newQueue[0])
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
					} else if p.coordinator != nil {
						// Fallback to the coordinator's selected playlist.
						sel := p.coordinator.SelectedPlaylist()
						if sel >= 0 && sel < len(p.coordinator.Playlists()) {
							key := p.coordinator.Playlists()[sel].Key
							p.playbackInitializing = true
							p.autoPlayOnTracksLoaded = true
							return p, tea.Batch(p.fetchTracksCmd(key), p.spinner.Tick)
						}
					}
				case app.HomeTab:
					if item := p.homeComponent.SelectedItem(); item != nil {
						log.Info("HomeTab: Play pressed for new playback", "itemType", item.Type, "itemKey", item.Key)
						// Check if this is a station - use StartStationPlayback for continuous playback
						if item.Type == "station" {
							log.Info("HomeTab: detected station, using startStationPlaybackCmd")
							return p, p.startStationPlaybackCmd(item.Key)
						}
						// For albums and other items, use regular fetch
						log.Info("HomeTab: using fetchTracksCmd for non-station")
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
		log.Info("PlaySelected key matched", "msg", msg, "activeTab", p.coordinator.ActiveTab())

		// If the Queue tab is active and the user presses space, treat it as
		// a "play selected" from the queue: remove previous entries up to the
		// selected item and begin playing that track.
		active := p.coordinator.ActiveTab()
		if active == app.QueueTab {
			sel := p.queueComponent.Index()
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
				p.queueComponent.UpdateListFromCoordinator()
				p.queueComponent.Select(0)
				return p, p.playAppTrack(&newQueue[0])
			}
			// Selected is the first item — just play it
			if item, ok := p.queueComponent.SelectedItem().(util.QueueItem); ok {
				at := util.DomainTrackToApp(&item.Track)
				return p, p.playAppTrack(at)
			}
			return p, nil
		}

		// Try to play the selected/first track from album, playlist or queue.
		if p.showingTracks {
			if item, ok := p.trackComponent.SelectedItem().(util.TrackItem); ok {
				// Convert domain.Track -> app.Track defensively using the selected item
				at := util.DomainTrackToApp(&item.Track)

				// If the user chooses to play a track while viewing an album/playlist,
				// build a queue from the selected track to the end of the list so the
				// subsequent tracks will be played automatically.
				tracks := p.coordinator.Tracks()
				selIdx := p.trackComponent.Index()
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
				p.queueComponent.UpdateListFromCoordinator()

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
					log.Info(
						"Home tab PlaySelected",
						"item.Type",
						item.Type,
						"item.Key",
						item.Key,
						"item.Title",
						item.Title,
					)
					// Check if this is a station - use StartStationPlayback for continuous playback
					if item.Type == "station" {
						log.Info("HomeTab PlaySelected: detected station, using startStationPlaybackCmd")
						return p, p.startStationPlaybackCmd(item.Key)
					}
					// Start async fetch and auto-play when tracks are loaded
					log.Info("HomeTab PlaySelected: using fetchTracksCmd for non-station")
					p.playbackInitializing = true
					p.autoPlayOnTracksLoaded = true
					return p, tea.Batch(p.fetchTracksCmd(item.Key), p.spinner.Tick)
				}
				// Fallback to coordinator selection if homeComponent doesn't have an item
				if p.coordinator != nil {
					sel := p.coordinator.SelectedAlbum()
					if sel >= 0 && sel < len(p.coordinator.Albums()) {
						key := p.coordinator.Albums()[sel].Key
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
	case key.Matches(msg, p.keys.SeekForward):
		if p.coordinator.PlaybackService() != nil && p.orchestrator != nil {
			svc := p.coordinator.PlaybackService()
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
						log.Error("Panic in SeekForward", "error", r)
					}
				}()
				if err := p.orchestrator.Seek(newPos); err != nil {
					log.Error("Orchestrator seek failed", "error", err)
				}
				return nil
			}
		}
		return p, nil
	case key.Matches(msg, p.keys.SeekBackward):
		if p.coordinator.PlaybackService() != nil && p.orchestrator != nil {
			svc := p.coordinator.PlaybackService()
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
						log.Error("Panic in SeekBackward", "error", r)
					}
				}()
				if err := p.orchestrator.Seek(newPos); err != nil {
					log.Error("Orchestrator seek failed", "error", err)
				}
				return nil
			}
		}
		return p, nil
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
			if p.coordinator != nil {
				p.coordinator.SetSonicAvailable(ok)
			}
			return sonicDetectResultMsg{ok: ok}
		}

	case key.Matches(msg, p.keys.Search):
		// Toggle Search tab and focus/blur the search input accordingly
		if p.coordinator.ActiveTab() == app.SearchTab {
			p.searchComponent.Blur()
			p.coordinator.SetActiveTab(app.HomeTab)
		} else {
			p.coordinator.SetActiveTab(app.SearchTab)
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
			oldSel := p.trackComponent.Index()
			_, cmd = p.trackComponent.Update(msg)
			newSel := p.trackComponent.Index()
			p.coordinator.SetSelectedTrack(newSel)

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
		if !p.IsFocusedQueue() && !p.coordinator.ShowQueueModal() {
			p.homeComponent.SetFocused(true)
			_, cmd = p.homeComponent.Update(msg)

			// Handle Enter key for home items
			if key.Matches(msg, p.keys.Enter) {
				log.Info("HomeTab Enter key pressed")
				if item := p.homeComponent.SelectedItem(); item != nil {
					log.Info("HomeTab Enter: selected item", "type", item.Type, "key", item.Key, "title", item.Title)
					switch item.Type {
					case "album":
						// Fetch tracks for the album
						log.Info("HomeTab Enter: fetching tracks for album")
						if p.libSvc != nil {
							reqCtx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
							defer cancel()
							_, _, _ = p.libSvc.FetchTracks(reqCtx, item.Key)
							p.showingTracks = true
						}
					case "station":
						// Play the station with continuous playback support (async)
						log.Info("HomeTab Enter: using startStationPlaybackCmd for station")
						return p, p.startStationPlaybackCmd(item.Key)
					}
				}
			}
		}
		return p, cmd

	case app.LibraryTab:
		if !p.IsFocusedQueue() && !p.coordinator.ShowQueueModal() {
			p.recentlyAddedComponent.SetFocused(true)
			_, cmd = p.recentlyAddedComponent.Update(msg)
			newIdx := p.recentlyAddedComponent.Index()
			p.coordinator.SetSelectedAlbum(newIdx)

			// If selection changed, fetch tracks for the selected album in the background
			if item, ok := p.recentlyAddedComponent.SelectedItem().(util.AlbumItem); ok {
				newIdx := p.recentlyAddedComponent.Index()
				if newIdx != p.lastSelectedAlbumIndex && p.libSvc != nil {
					p.lastSelectedAlbumIndex = newIdx
					log.Info("LibraryTab: fetching tracks for album", "key", item.Album.Key)
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
		if !p.IsFocusedQueue() && !p.coordinator.ShowQueueModal() {
			_, cmd = p.playlistComponent.Update(msg)
			p.coordinator.SetSelectedPlaylist(p.playlistComponent.Index())

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
	if len(msg.Runes) == 0 {
		return false
	}
	// Only accept a single rune to prevent conflicts with multi-rune sequences
	return len(msg.Runes) == 1 && msg.Runes[0] == r
}

// Helper: play the provided app.Track (UI & playback service)
func (p *LibraryPage) playAppTrack(at *app.Track) tea.Cmd {
	if at == nil {
		return nil
	}
	log.Info("playAppTrack.called", "title", at.Title, "playQueueItemID", at.PlayQueueItemID)
	// Update UI coordinator state preemptively for immediate UI feedback — playback.started will reconcile state later
	p.coordinator.SetCurrentTrack(at)
	p.coordinator.SetPlaybackState(app.PlaybackPlaying)

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
		log.Debug("playAppTrack: starting playback initialization", "title", at.Title)
		playCmd = func() tea.Msg {
			err := p.orchestrator.PlayAppTrack(initCtx, at)
			return playResultMsg{Err: err}
		}
	} else {
		// Orchestrator missing: notify user
		p.coordinator.SetNotification("Play failed: playback orchestrator unavailable", "error", 10*time.Second)
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
	log.Info("playNext: called", "orchestrator", p.orchestrator != nil)
	if p.orchestrator == nil {
		return nil
	}

	// If we're in station playback mode, refresh the playQueue to get new tracks
	// Only refresh when we're approaching the end of the queue (within 5 tracks)
	var refreshCmd tea.Cmd
	isStation := p.coordinator.IsStationPlayback()
	hasLibSvc := p.libSvc != nil
	log.Info("playNext: checking station mode", "isStationPlayback", isStation, "hasLibSvc", hasLibSvc)

	if isStation && hasLibSvc {
		activeQueue := p.coordinator.ActivePlayQueue()
		if activeQueue != nil && activeQueue.PlayQueueID > 0 {
			queue := p.coordinator.Queue()
			currentIdx := p.coordinator.QueueIndex()
			nextIdx := currentIdx + 1
			tracksRemaining := len(queue) - nextIdx - 1 // tracks after the one we're about to play

			// Only refresh when we're within 5 tracks of the end
			const refreshThreshold = 5
			shouldRefresh := tracksRemaining <= refreshThreshold

			var selectedItemID int
			if nextIdx < len(queue) {
				selectedItemID = queue[nextIdx].PlayQueueItemID
			}

			log.Info("playNext: station mode status",
				"playQueueID", activeQueue.PlayQueueID,
				"stationKey", activeQueue.StationKey,
				"nextIdx", nextIdx,
				"queueLen", len(queue),
				"tracksRemaining", tracksRemaining,
				"shouldRefresh", shouldRefresh,
				"selectedItemID", selectedItemID)

			if shouldRefresh {
				log.Info("playNext: triggering playQueue refresh (approaching end of queue)")
				refreshCmd = p.refreshPlayQueueCmd(activeQueue.PlayQueueID, selectedItemID)
			}
		} else {
			log.Warn("playNext: station mode but activeQueue invalid",
				"activeQueue", activeQueue,
				"playQueueID", func() int {
					if activeQueue != nil {
						return activeQueue.PlayQueueID
					}
					return 0
				}())
		}
	} else {
		log.Debug("playNext: not in station mode or no libSvc",
			"isStationPlayback", isStation,
			"hasLibSvc", hasLibSvc)
	}

	// PlaybackController for navigation logic doesn't need the service
	pc := service.NewPlaybackController(nil)
	err := p.orchestrator.PlayNext(
		p.ctx,
		pc,
		p.coordinator.Queue(),
		p.coordinator.QueueIndex(),
		p.coordinator.Tracks(),
		p.coordinator.SelectedTrack(),
	)
	if err != nil {
		log.Error("PlayNext failed", "err", err)
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
		p.coordinator.Queue(),
		p.coordinator.QueueIndex(),
		p.coordinator.Tracks(),
		p.coordinator.SelectedTrack(),
	)
	if err != nil {
		log.Error("PlayPrev failed", "err", err)
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
