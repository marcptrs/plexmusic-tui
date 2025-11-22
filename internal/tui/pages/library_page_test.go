package pages

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/service"
)

func TestLibraryPage_ViewHome_RendersRecentlyAdded(t *testing.T) {
	coord := app.NewCoordinator()

	albums := []app.Album{
		{
			Title:  "Test Album",
			Artist: "Test Artist",
			Year:   2022,
			Key:    "/library/metadata/123",
			Thumb:  "/thumb.jpg",
		},
	}

	coord.SetAlbums(albums)
	coord.SetSelectedAlbum(0)
	coord.SetActiveTab(app.HomeTab)

	// Simulate an authenticated session and a selected server so the page renders content.
	server := app.PlexServer{
		Name:        "Local Server",
		Host:        "127.0.0.1",
		Port:        "32400",
		AccessToken: "token",
		Scheme:      "http",
	}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPage(coord)

	// Ensure the page has a size to render its layout
	page.width = 120
	page.height = 40

	// Send event to populate list
	evt := service.LibraryEvent{
		Type: "recently_added.loaded",
		Albums: []domain.Album{
			{Title: "Test Album", Artist: "Test Artist", Year: 2023},
		},
	}
	page.Update(evt)

	// Open drawer for the selected tab (Enter) so the "Recently Added" list is shown over Now Playing
	m, _ := page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	page = m.(*LibraryPage)

	view := page.View()
	if !strings.Contains(view, "Test Album") {
		t.Fatalf("expected Home view to include recently added album title, got: %q", view)
	}
	// Home should also show now playing placeholder (Nothing Playing) when no track is selected
	if !strings.Contains(view, "Nothing Playing") {
		t.Fatalf("expected Home view to show Now Playing placeholder, got: %q", view)
	}
	// Left pane should show navigation help for the list
	if !strings.Contains(view, "↑/k") {
		t.Fatalf("expected left pane view to include list navigation help, got: %q", view)
	}
}

func TestLibraryPage_ViewPlaylists_RendersPlaylists(t *testing.T) {
	coord := app.NewCoordinator()

	playlists := []app.Playlist{
		{
			Title:     "Test Playlist",
			Key:       "/playlists/1",
			LeafCount: 3,
			Duration:  120000,
		},
	}

	coord.SetPlaylists(playlists)
	coord.SetSelectedPlaylist(0)
	coord.SetActiveTab(app.PlaylistsTab)

	// Simulate an authenticated session and a selected server so the page renders content.
	server := app.PlexServer{
		Name:        "Local Server",
		Host:        "127.0.0.1",
		Port:        "32400",
		AccessToken: "token",
		Scheme:      "http",
	}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPage(coord)
	page.width = 120
	page.height = 40

	// Send event to populate list
	evt := service.LibraryEvent{
		Type: "playlists.loaded",
		Playlists: []domain.Playlist{
			{Title: "Test Playlist", Key: "/playlists/1", LeafCount: 3, Duration: 120000},
		},
	}
	page.Update(evt)

	// Open drawer for the selected tab (Enter) so the playlist list is shown over Now Playing
	m, _ := page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	page = m.(*LibraryPage)

	view := page.View()
	if !strings.Contains(view, "Test Playlist") {
		t.Fatalf("expected Playlists view to include playlist title, got: %q", view)
	}
}

func TestLibraryPage_FetchesLibraryDataFromServer(t *testing.T) {
	// Start a test HTTP server to simulate Plex responses.
	mux := http.NewServeMux()
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Directory":[{"key":"1","title":"Music","type":"artist"}]}}`)
	})
	mux.HandleFunc("/library/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[{"title":"Test Album","parentTitle":"Test Artist","year":2022,"key":"/library/metadata/123","thumb":"/thumb.jpg"}]}}`)
	})
	mux.HandleFunc("/playlists", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[{"title":"Test Playlist","key":"/playlists/1","leafCount":3,"duration":120000,"playlistType":"audio"}]}}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	coord := app.NewCoordinator()
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Test Server", Host: host, Port: port, Scheme: u.Scheme, AccessToken: "test-token"}})
	coord.SetSelectedServer(0)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Ensure the page will create a library service on Init
	cmd := page.Init()
	if cmd == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	// Wait briefly for the lib service to be created
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Wait until libSvc is initialized
	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Subscribe directly to the library service events
	evCh := page.libSvc.Subscribe(context.Background())

	// Trigger fetches
	_, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	_, err = page.libSvc.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists error: %v", err)
	}

	// Read and apply events to the page update
	// We expect at least two events: recently_added.loaded and playlists.loaded
	for i := 0; i < 2; i++ {
		ev := <-evCh
		// Convert to page-level update message
		page.Update(ev.Payload)
	}

	// Now coordinator should have both albums and playlists populated
	if len(coord.Albums()) == 0 {
		t.Fatalf("expected coordinator to have albums after fetch")
	}
	if len(coord.Playlists()) == 0 {
		t.Fatalf("expected coordinator to have playlists after fetch")
	}
}

// Test that when a user changes selection in the Recently Added list, the
// LibraryPage triggers a background fetch for the tracks belonging to the
// newly-selected album.
func TestLibraryPage_FetchTracksOnAlbumSelection(t *testing.T) {
	// Start a test HTTP server to simulate Plex responses.
	mux := http.NewServeMux()
	mux.HandleFunc("/library/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[{"title":"Album 1","parentTitle":"Artist 1","year":2022,"key":"/library/metadata/1","thumb":"/thumb1.jpg"},{"title":"Album 2","parentTitle":"Artist 2","year":2021,"key":"/library/metadata/2","thumb":"/thumb2.jpg"}]}}`)
	})
	// Simulate a server that only returns track data under /children for album 2
	mux.HandleFunc("/library/metadata/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"Track 1","grandparentTitle":"Artist 1","parentTitle":"Album 1","duration":60000,"index":1,"key":"/library/metadata/1/track/1","thumb":"/thumb1.jpg"}]}`)
	})
	mux.HandleFunc("/library/metadata/2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[]}`)
	})
	mux.HandleFunc("/library/metadata/2/children", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"Track X","grandparentTitle":"Artist 2","parentTitle":"Album 2","duration":50000,"index":1,"key":"/library/metadata/2/track/1","thumb":"/thumb2.jpg"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	coord := app.NewCoordinator()
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Test Server", Host: host, Port: port, Scheme: u.Scheme, AccessToken: "test-token"}})
	coord.SetSelectedServer(0)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Ensure the page will create a library service on Init
	cmd := page.Init()
	if cmd == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	// Wait until libSvc is initialized
	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Subscribe to library events so we can assert we eventually receive tracks.loaded
	evCh := page.libSvc.Subscribe(context.Background())

	// Trigger recently added fetch and apply events to the page
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	// Read and apply the event(s)
	ev := <-evCh
	page.Update(ev.Payload)

	// Ensure the list contains multiple albums and the selection starts at 0
	if len(page.recentlyAddedList.Items()) < 2 {
		t.Fatalf("expected at least 2 albums in list")
	}
	// Switch active tab to Home to ensure key navigation updates the recentlyAdded list
	page.coordinator.SetActiveTab(app.HomeTab)
	// Move selection to index 1 (Down) which should trigger a track fetch for album 2
	m, cmd := page.Update(tea.KeyMsg{Type: tea.KeyDown})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}

	// Wait for a tracks.loaded event for the album
	start = time.Now()
	seenTrack := false
	for time.Since(start) < 2*time.Second {
		select {
		case ev := <-evCh:
			if ev.Payload.Type == "tracks.loaded" {
				// Simulate the page receiving the library event via its subscription
				page.Update(ev.Payload)
				if len(ev.Payload.Tracks) > 0 && ev.Payload.Tracks[0].Title == "Track X" {
					seenTrack = true
					break
				}
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
		if seenTrack {
			break
		}
	}
	if !seenTrack {
		t.Fatalf("expected tracks.loaded event for selected album 2")
	}
	// After the prefetch, the page should NOT automatically show tracks until Enter is pressed
	if page.showingTracks {
		t.Fatalf("did not expect page to show tracks on selection change")
	}
	// Press Enter to open the track list and ensure it now displays Track X
	m, cmd = page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}
	// Simulate any library events that reach the page
	// (evCh events were already applied in-loop), now view should include Track X
	view := page.View()
	if !strings.Contains(view, "Track X") {
		t.Fatalf("expected view to contain Track X after pressing Enter; got: %s", view)
	}
}

func TestLibraryPage_PlaybackPositionEventUpdatesCoordinator(t *testing.T) {
	coord := app.NewCoordinator()

	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)

	// Simulate a playback position event coming from the playback service.
	ev := service.PlaybackEvent{Type: "playback.position", Position: 1234, Length: 5678}
	page.Update(ev)

	if coord.StreamPosition() != 1234 {
		t.Fatalf("expected coordinator stream position to be 1234, got %d", coord.StreamPosition())
	}
	if coord.StreamLength() != 5678 {
		t.Fatalf("expected coordinator stream length to be 5678, got %d", coord.StreamLength())
	}
}

func TestLibraryPage_PlaybackLoadFailure_ShowsNotification(t *testing.T) {
	coord := app.NewCoordinator()

	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Ensure notification not active initially
	if coord.NotificationActive() {
		t.Fatalf("expected no active notification initially")
	}

	// Simulate load failed event and apply to the page
	ev := service.PlaybackEvent{Type: "playback.load_failed", Error: fmt.Errorf("boom")}
	page.Update(ev)

	// Coordinator notification should be active after the event
	if !coord.NotificationActive() {
		t.Fatalf("expected notification to be active after playback.load_failed event")
	}

	// Ensure the rendered view contains the error message string for visual confirmation
	view := page.View()
	if !strings.Contains(view, "Load failed") {
		t.Fatalf("expected view to include notification 'Load failed', got: %q", view)
	}
}

func TestLibraryPage_FetchTracksOnAlbumSelectionAbsoluteKey(t *testing.T) {
	// Start a test HTTP server to simulate Plex responses.
	mux := http.NewServeMux()
	mux.HandleFunc("/library/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		// Use absolute URL for album keys (some Plex servers return absolute urls)
		base := fmt.Sprintf("http://%s", r.Host)
		fmt.Fprintf(w, `{"MediaContainer":{"Metadata":[{"title":"Album 1","parentTitle":"Artist 1","year":2022,"key":"%s/library/metadata/1","thumb":"/thumb1.jpg"},{"title":"Album 2","parentTitle":"Artist 2","year":2021,"key":"%s/library/metadata/2","thumb":"/thumb2.jpg"}]}}`+"\n", base, base)
	})
	// Simulate a server that only returns track data under /children for album 2
	mux.HandleFunc("/library/metadata/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"Track 1","grandparentTitle":"Artist 1","parentTitle":"Album 1","duration":60000,"index":1,"key":"/library/metadata/1/track/1","thumb":"/thumb1.jpg"}]}`)
	})
	mux.HandleFunc("/library/metadata/2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[]}`)
	})
	mux.HandleFunc("/library/metadata/2/children", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"Track X","grandparentTitle":"Artist 2","parentTitle":"Album 2","duration":50000,"index":1,"key":"/library/metadata/2/track/1","thumb":"/thumb2.jpg"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	coord := app.NewCoordinator()
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Test Server", Host: host, Port: port, Scheme: u.Scheme, AccessToken: "test-token"}})
	coord.SetSelectedServer(0)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Ensure the page will create a library service on Init
	cmd := page.Init()
	if cmd == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	// Wait until libSvc is initialized
	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Subscribe to library events so we can assert we eventually receive tracks.loaded
	evCh := page.libSvc.Subscribe(context.Background())

	// Trigger recently added fetch and apply events to the page
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	// Read and apply the event(s)
	ev := <-evCh
	page.Update(ev.Payload)

	// Ensure the list contains multiple albums and the selection starts at 0
	if len(page.recentlyAddedList.Items()) < 2 {
		t.Fatalf("expected at least 2 albums in list")
	}
	// Switch active tab to Home to ensure key navigation updates the recentlyAdded list
	page.coordinator.SetActiveTab(app.HomeTab)
	// Move selection to index 1 (Down) which should trigger a track fetch for album 2
	m, cmd := page.Update(tea.KeyMsg{Type: tea.KeyDown})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}

	// Wait for a tracks.loaded event for the album
	start = time.Now()
	seenTrack := false
	for time.Since(start) < 2*time.Second {
		select {
		case ev := <-evCh:
			if ev.Payload.Type == "tracks.loaded" {
				// Simulate the page receiving the library event via its subscription
				page.Update(ev.Payload)
				if len(ev.Payload.Tracks) > 0 && ev.Payload.Tracks[0].Title == "Track X" {
					seenTrack = true
					break
				}
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
		if seenTrack {
			break
		}
	}
	if !seenTrack {
		t.Fatalf("expected tracks.loaded event for selected album 2 using absolute keys")
	}
	// After the prefetch, the page should NOT automatically show tracks until Enter is pressed
	if page.showingTracks {
		t.Fatalf("did not expect page to show tracks on selection change")
	}
	// Press Enter to open the track list and ensure it now displays Track X
	m, cmd = page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}
	// Simulate any library events that reach the page
	// (evCh events were already applied in-loop), now view should include Track X
	view := page.View()
	if !strings.Contains(view, "Track X") {
		t.Fatalf("expected view to contain Track X after pressing Enter; got: %s", view)
	}
}

// Test that when a user changes selection in the Playlists list, the
// LibraryPage triggers a background fetch for the tracks belonging to the
// newly-selected playlist.
func TestLibraryPage_FetchTracksOnPlaylistSelection(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/playlists", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[{"title":"Pl 1","key":"/playlists/1"},{"title":"Pl 2","key":"/playlists/2"}]}}`)
	})
	mux.HandleFunc("/playlists/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"PTrack 1","grandparentTitle":"Artist P","parentTitle":"Pl 1","duration":40000,"index":1,"key":"/playlists/1/track/1"}]}`)
	})
	mux.HandleFunc("/playlists/2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"PTrack X","grandparentTitle":"Artist P2","parentTitle":"Pl 2","duration":30000,"index":1,"key":"/playlists/2/track/1"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	coord := app.NewCoordinator()
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Test Server", Host: host, Port: port, Scheme: u.Scheme, AccessToken: "test-token"}})
	coord.SetSelectedServer(0)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40
	if page.Init() == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	// Wait until libSvc is initialized
	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	evCh := page.libSvc.Subscribe(context.Background())

	// Trigger playlists fetch and apply events to the page
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := page.libSvc.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	if len(page.playlistList.Items()) < 2 {
		t.Fatalf("expected at least 2 playlists in list")
	}
	// Switch active tab to Playlists to ensure key navigation updates the playlist list
	page.coordinator.SetActiveTab(app.PlaylistsTab)

	// Move selection down which should fetch tracks for playlist 2
	m2, cmd2 := page.Update(tea.KeyMsg{Type: tea.KeyDown})
	page = m2.(*LibraryPage)
	if cmd2 != nil {
		_ = cmd2()
	}

	// Wait for tracks.loaded for playlist 2
	start = time.Now()
	seenTrack := false
	for time.Since(start) < 2*time.Second {
		select {
		case ev := <-evCh:
			if ev.Payload.Type == "tracks.loaded" {
				// Simulate the page receiving the library event via its subscription
				page.Update(ev.Payload)
				if len(ev.Payload.Tracks) > 0 && ev.Payload.Tracks[0].Title == "PTrack X" {
					seenTrack = true
					break
				}
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
		if seenTrack {
			break
		}
	}
	if !seenTrack {
		t.Fatalf("expected tracks.loaded event for selected playlist 2")
	}
	// After the prefetch, the page should NOT automatically show tracks until Enter is pressed
	if page.showingTracks {
		t.Fatalf("did not expect page to show tracks on selection change")
	}
	// Press Enter to open the track list and ensure it now displays PTrack X
	m2, cmd2 = page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	page = m2.(*LibraryPage)
	if cmd2 != nil {
		_ = cmd2()
	}
	v := page.View()
	if !strings.Contains(v, "PTrack X") {
		t.Fatalf("expected view to contain PTrack X after pressing Enter; got: %s", v)
	}
}

func TestLibraryPage_EnterOpensTrackList_RecentlyAdded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Directory":[{"key":"1","title":"Music","type":"artist"}]}}`)
	})
	mux.HandleFunc("/library/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[{"title":"Album 1","parentTitle":"Artist 1","year":2022,"key":"/library/metadata/1","thumb":"/thumb1.jpg"}]}}`)
	})
	mux.HandleFunc("/library/metadata/1/children", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"T1","grandparentTitle":"Artist 1","parentTitle":"Album 1","duration":60000,"index":1,"key":"/library/metadata/1/track/1","thumb":"/thumb1.jpg"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	coord := app.NewCoordinator()
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Test Server", Host: host, Port: port, Scheme: u.Scheme, AccessToken: "test-token"}})
	coord.SetSelectedServer(0)
	coord.SetActiveTab(app.HomeTab)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40
	if page.Init() == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	// Wait until libSvc is initialized
	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	evCh := page.libSvc.Subscribe(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	// Ensure album is present
	if len(page.recentlyAddedList.Items()) == 0 {
		t.Fatalf("expected recently added albums after fetch")
	}
	// Make sure tab is active and selection set
	page.coordinator.SetActiveTab(app.HomeTab)
	page.coordinator.SetSelectedAlbum(0)

	// Press Enter and expect the track list to open (no immediate playback)
	m, cmd := page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}
	if !page.showingTracks {
		t.Fatalf("expected page.showingTracks after pressing Enter")
	}
	if coord.HasCurrentTrack() {
		t.Fatalf("did not expect coordinator to have current track after pressing Enter")
	}
	if coord.IsPlaying() {
		t.Fatalf("did not expect coordinator playback state to be Playing after pressing Enter")
	}
}

func TestLibraryPage_EnterOpensTrackList_Playlist(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/playlists", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[{"title":"Pl 1","key":"/playlists/1"}]}}`)
	})
	mux.HandleFunc("/playlists/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"P1","grandparentTitle":"Artist P","parentTitle":"Pl 1","duration":40000,"index":1,"key":"/playlists/1/track/1","thumb":"/thumbp.jpg"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	coord := app.NewCoordinator()
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Test Server", Host: host, Port: port, Scheme: u.Scheme, AccessToken: "test-token"}})
	coord.SetSelectedServer(0)
	coord.SetActiveTab(app.PlaylistsTab)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40
	if page.Init() == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	evCh := page.libSvc.Subscribe(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := page.libSvc.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	if len(page.playlistList.Items()) == 0 {
		t.Fatalf("expected playlists after fetch")
	}
	page.coordinator.SetActiveTab(app.PlaylistsTab)
	page.coordinator.SetSelectedPlaylist(0)

	m, cmd := page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}
	if !page.showingTracks {
		t.Fatalf("expected page.showingTracks after pressing Enter on playlist")
	}
	if coord.HasCurrentTrack() {
		t.Fatalf("did not expect coordinator to have current track after pressing Enter on playlist")
	}
	if coord.IsPlaying() {
		t.Fatalf("did not expect coordinator playback state to be Playing after pressing Enter on playlist")
	}
}

func TestLibraryPage_PPlaysAlbumAndQueuesTracks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Directory":[{"key":"1","title":"Music","type":"artist"}]}}`)
	})
	mux.HandleFunc("/library/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[{"title":"Album 1","parentTitle":"Artist 1","year":2022,"key":"/library/metadata/1","thumb":"/thumb1.jpg"}]}}`)
	})
	mux.HandleFunc("/library/metadata/1/children", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"T1","grandparentTitle":"Artist 1","parentTitle":"Album 1","duration":60000,"index":1,"key":"/library/metadata/1/track/1","thumb":"/thumb1.jpg"},{"title":"T2","grandparentTitle":"Artist 1","parentTitle":"Album 1","duration":50000,"index":2,"key":"/library/metadata/1/track/2","thumb":"/thumb2.jpg"}]}`)
	})
	// Return a minimal WAV for the track stream endpoints so playback succeeds
	mux.HandleFunc("/library/metadata/1/track/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(createSilenceWav(1))
	})
	mux.HandleFunc("/library/metadata/1/track/2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(createSilenceWav(1))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	coord := app.NewCoordinator()
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Test Server", Host: host, Port: port, Scheme: u.Scheme, AccessToken: "test-token"}})
	coord.SetSelectedServer(0)
	coord.SetActiveTab(app.HomeTab)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40
	if page.Init() == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	evCh := page.libSvc.Subscribe(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	if len(page.recentlyAddedList.Items()) == 0 {
		t.Fatalf("expected recently added albums after fetch")
	}
	page.coordinator.SetActiveTab(app.HomeTab)
	page.coordinator.SetSelectedAlbum(0)

	// Press 'space' to play album and expect queue to be set and playback to begin
	m, cmd := page.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}

	if !coord.HasCurrentTrack() {
		t.Fatalf("expected coordinator to have current track after pressing space on album")
	}
	if !coord.IsPlaying() {
		t.Fatalf("expected coordinator playback state to be Playing after pressing space on album")
	}
	queue := coord.Queue()
	if len(queue) != 2 {
		t.Fatalf("expected queue to contain all album tracks, got %d", len(queue))
	}
	if coord.QueueIndex() != 0 {
		t.Fatalf("expected queue index to be 0, got %d", coord.QueueIndex())
	}
	if !page.showingTracks {
		t.Fatalf("expected page.showingTracks after pressing p on album")
	}
	view := page.View()
	if !strings.Contains(view, "T1") {
		t.Fatalf("expected view to contain currently playing track T1, got: %s", view)
	}
}

func TestLibraryPage_RenderSearch_IncludesTracks(t *testing.T) {
	coord := app.NewCoordinator()

	// Add a sample track that should be matched by search
	tracks := []app.Track{
		{
			Title:  "Super Track",
			Artist: "Search Artist",
			Album:  "Some Album",
			Key:    "/library/metadata/1/track/1",
		},
	}
	coord.SetTracks(tracks)
	// Activate Search tab
	coord.SetActiveTab(app.SearchTab)

	// Simulate authenticated server so the page renders
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 100
	page.height = 30

	// Put the search query into the input and ensure it's visible
	page.searchInput.SetValue("super")
	view := page.View()

	if !strings.Contains(view, "Super Track") {
		t.Fatalf("expected Search view to include matching track title, got: %q", view)
	}
}

// createSilenceWav returns a WAV byte slice with `seconds` seconds of silence
func createSilenceWav(seconds int) []byte {
	sampleRate := 44100
	bitsPerSample := 16
	numChannels := 1
	numSamples := sampleRate * seconds
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := numSamples * blockAlign

	// RIFF header
	buff := &bytes.Buffer{}
	buff.WriteString("RIFF")
	// ChunkSize = 36 + Subchunk2Size
	chunkSize := uint32(36 + dataSize)
	_ = binary.Write(buff, binary.LittleEndian, chunkSize)
	buff.WriteString("WAVE")

	// fmt subchunk
	buff.WriteString("fmt ")
	_ = binary.Write(buff, binary.LittleEndian, uint32(16))            // Subchunk1Size
	_ = binary.Write(buff, binary.LittleEndian, uint16(1))             // AudioFormat (1 = PCM)
	_ = binary.Write(buff, binary.LittleEndian, uint16(numChannels))   // NumChannels
	_ = binary.Write(buff, binary.LittleEndian, uint32(sampleRate))    // SampleRate
	_ = binary.Write(buff, binary.LittleEndian, uint32(byteRate))      // ByteRate
	_ = binary.Write(buff, binary.LittleEndian, uint16(blockAlign))    // BlockAlign
	_ = binary.Write(buff, binary.LittleEndian, uint16(bitsPerSample)) // BitsPerSample

	// data subchunk
	buff.WriteString("data")
	_ = binary.Write(buff, binary.LittleEndian, uint32(dataSize))
	// Write zeroed sample data
	zero := make([]byte, dataSize)
	buff.Write(zero)

	return buff.Bytes()
}

func TestLibraryPage_PPlaysPlaylistAndQueuesTracks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/playlists", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[{"title":"Pl 1","key":"/playlists/1"}]}}`)
	})
	mux.HandleFunc("/playlists/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"Metadata":[{"title":"P1","grandparentTitle":"Artist P","parentTitle":"Pl 1","duration":40000,"index":1,"key":"/playlists/1/track/1","thumb":"/thumbp.jpg"},{"title":"P2","grandparentTitle":"Artist P","parentTitle":"Pl 1","duration":30000,"index":2,"key":"/playlists/1/track/2","thumb":"/thumbp2.jpg"}]}`)
	})
	// Add stream endpoints for playlist tracks
	mux.HandleFunc("/playlists/1/track/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(createSilenceWav(1))
	})
	mux.HandleFunc("/playlists/1/track/2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(createSilenceWav(1))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host := u.Hostname()
	port := u.Port()

	coord := app.NewCoordinator()
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Test Server", Host: host, Port: port, Scheme: u.Scheme, AccessToken: "test-token"}})
	coord.SetSelectedServer(0)
	coord.SetActiveTab(app.PlaylistsTab)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40
	if page.Init() == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	evCh := page.libSvc.Subscribe(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := page.libSvc.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	if len(page.playlistList.Items()) == 0 {
		t.Fatalf("expected playlists after fetch")
	}
	page.coordinator.SetActiveTab(app.PlaylistsTab)
	page.coordinator.SetSelectedPlaylist(0)

	m, cmd := page.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}

	if !coord.HasCurrentTrack() {
		t.Fatalf("expected coordinator to have current track after pressing space on playlist")
	}
	if !coord.IsPlaying() {
		t.Fatalf("expected coordinator playback state to be Playing after pressing space on playlist")
	}
	queue := coord.Queue()
	if len(queue) != 2 {
		t.Fatalf("expected queue to contain all playlist tracks, got %d", len(queue))
	}
	if coord.QueueIndex() != 0 {
		t.Fatalf("expected queue index to be 0, got %d", coord.QueueIndex())
	}
	if !page.showingTracks {
		t.Fatalf("expected page.showingTracks after pressing p on playlist")
	}
	view := page.View()
	if !strings.Contains(view, "P1") {
		t.Fatalf("expected view to contain currently playing track P1, got: %s", view)
	}
}

func TestLibraryPage_Init_UsesServerAccessTokenOverCoordinatorToken(t *testing.T) {
	coord := app.NewCoordinator()
	coord.SetToken("coord-token")

	server := app.PlexServer{
		Name:        "Local Server",
		Host:        "127.0.0.1",
		Port:        "32400",
		AccessToken: "server-token",
		Scheme:      "http",
	}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)

	page := NewLibraryPage(coord)
	page.width = 120
	page.height = 40

	_ = page.Init()

	// Wait for libSvc to be created
	start := time.Now()
	for page.libSvc == nil {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("libSvc not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Use BuildStreamURL to verify the token used in libSvc is the server's token
	url, err := page.libSvc.BuildStreamURL(&domain.Track{Key: "/library/metadata/1"})
	if err != nil {
		t.Fatalf("BuildStreamURL returned error: %v", err)
	}
	if !strings.Contains(url, "X-Plex-Token=server-token") {
		t.Fatalf("expected BuildStreamURL to include server AccessToken; got: %s", url)
	}
}
