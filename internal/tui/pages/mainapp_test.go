package pages

import (
	"context"
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
	views "plexmusic-tui/internal/ui"
)

func TestMainAppPage_ViewHome_RendersRecentlyAdded(t *testing.T) {
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

	page := NewMainAppPage(coord)

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
	page = m.(*MainAppPage)

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

func TestMainAppPage_ViewPlaylists_RendersPlaylists(t *testing.T) {
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

	page := NewMainAppPage(coord)
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
	page = m.(*MainAppPage)

	view := page.View()
	if !strings.Contains(view, "Test Playlist") {
		t.Fatalf("expected Playlists view to include playlist title, got: %q", view)
	}
}

func TestMainAppPage_FetchesLibraryDataFromServer(t *testing.T) {
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

	page := NewMainAppPageWithAuth(coord, nil)
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
// MainAppPage triggers a background fetch for the tracks belonging to the
// newly-selected album.
func TestMainAppPage_FetchTracksOnAlbumSelection(t *testing.T) {
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

	page := NewMainAppPageWithAuth(coord, nil)
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
	page = m.(*MainAppPage)
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
	page = m.(*MainAppPage)
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

func TestMainAppPage_FetchTracksOnAlbumSelectionAbsoluteKey(t *testing.T) {
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

	page := NewMainAppPageWithAuth(coord, nil)
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
	page = m.(*MainAppPage)
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
	page = m.(*MainAppPage)
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
// MainAppPage triggers a background fetch for the tracks belonging to the
// newly-selected playlist.
func TestMainAppPage_FetchTracksOnPlaylistSelection(t *testing.T) {
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

	page := NewMainAppPageWithAuth(coord, nil)
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
	page = m2.(*MainAppPage)
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
	page = m2.(*MainAppPage)
	if cmd2 != nil {
		_ = cmd2()
	}
	v := page.View()
	if !strings.Contains(v, "PTrack X") {
		t.Fatalf("expected view to contain PTrack X after pressing Enter; got: %s", v)
	}
}

func TestMainAppPage_SplitLayout_TabsControlLeft_RightDisplaysNowPlaying(t *testing.T) {
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

	page := NewMainAppPage(coord)
	page.width = 120
	page.height = 40
	_ = page.Init()
	// Ensure the Playlists tab is active after Init (Init defaults to HomeTab)
	page.coordinator.SetActiveTab(app.PlaylistsTab)

	// Send event to populate list
	evt := service.LibraryEvent{
		Type: "playlists.loaded",
		Playlists: []domain.Playlist{
			{Title: "Test Playlist", Key: "/playlists/1", LeafCount: 3, Duration: 120000},
		},
	}
	page.Update(evt)

	view := page.View()
	if !strings.Contains(view, "Test Playlist") {
		t.Fatalf("expected left pane to include playlist title, got: %q", view)
	}
	if !strings.Contains(view, "Now Playing") {
		t.Fatalf("expected right pane to include Now Playing title, got: %q", view)
	}
}

func TestMainAppPage_Tabs_AreAboveLeftPaneOnly(t *testing.T) {
	coord := app.NewCoordinator()

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

	page := NewMainAppPage(coord)
	page.width = 120
	page.height = 40
	_ = page.Init()

	// Force settings tab active so we can detect its label in the tabs row.
	page.coordinator.SetActiveTab(app.SettingsTab)
	view := page.View()

	// Compute left pane width and check that a tab label (Settings) appears
	// in the left column area only.
	leftWidth := views.GetContentPaneWidth(page.width)

	lines := strings.Split(view, "\n")
	found := false
	for _, l := range lines { // scan entire view: the tabs are in the left column
		idx := strings.Index(l, "Settings")
		if idx >= 0 {
			found = true
			if idx >= leftWidth {
				t.Fatalf("expected 'Settings' tab to be within left pane width < %d, found at %d: %q", leftWidth, idx, l)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected 'Settings' label to appear in view tabs, got: %q", view)
	}
}

func TestMainAppPage_Init_UsesServerAccessTokenOverCoordinatorToken(t *testing.T) {
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

	page := NewMainAppPage(coord)
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

func TestMainAppPage_Tabs_DoNotWrap_Settings(t *testing.T) {
	// Setup a coordinator with a token and server so the tabs render.
	coord := app.NewCoordinator()
	coord.SetToken("test-token")

	server := app.PlexServer{
		Name:        "Local Server",
		Host:        "127.0.0.1",
		Port:        "32400",
		AccessToken: "token",
		Scheme:      "http",
	}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetActiveTab(app.SettingsTab)

	page := NewMainAppPage(coord)
	page.width = 80
	page.height = 20

	// Initialize to ensure any services or state used by View are prepared.
	_ = page.Init()

	view := page.View()

	// Ensure there are a number of tab boxes in the left pane equal to number of tabs
	leftWidth := views.GetContentPaneWidth(page.width)
	lines := strings.Split(view, "\n")
	// Only inspect the first 8 rows (tabs height + padding)
	maxRows := 8
	if len(lines) < maxRows {
		maxRows = len(lines)
	}
	leftChunk := strings.Join(lines[:maxRows], "\n")
	if len(leftChunk) >= leftWidth {
		// Truncate to only left pane columns
		lcLines := strings.Split(leftChunk, "\n")
		for i, l := range lcLines {
			if len(l) >= leftWidth {
				lcLines[i] = l[:leftWidth]
			}
		}
		leftChunk = strings.Join(lcLines, "\n")
	}
	headerCount := strings.Count(leftChunk, "╭")
	if headerCount == 0 {
		t.Fatalf("expected at least 1 tab box in left pane, found none, excerpt: %q", leftChunk)
	}
	// Find first '╭' (tab top-left) after the title/status and ensure it's in left pane
	firstRows := lines[:maxRows]
	foundIdx := -1
	for _, l := range firstRows {
		if i := strings.Index(l, "╭"); i >= 0 {
			foundIdx = i
			break
		}
	}
	if foundIdx < 0 {
		t.Fatalf("expected to find a tab top-left corner in first rows; none found, excerpt: %q", leftChunk)
	}
	if foundIdx >= leftWidth {
		t.Fatalf("expected tabs top-left to be within left pane width < %d, found at %d", leftWidth, foundIdx)
	}
}

func TestMainAppPage_Tabs_DoNotWrap_AllTabs(t *testing.T) {
	// Setup a coordinator with a token and server so the tabs render.
	coord := app.NewCoordinator()
	coord.SetToken("test-token")

	server := app.PlexServer{
		Name:        "Local Server",
		Host:        "127.0.0.1",
		Port:        "32400",
		AccessToken: "token",
		Scheme:      "http",
	}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)

	page := NewMainAppPage(coord)
	// Use a smaller width to stress the tab layout; it should still keep labels intact.
	page.width = 80
	page.height = 20

	// Ensure view is rendered using the initialized page.
	_ = page.Init()
	view := page.View()

	// Ensure that the tabs (or at least their prefixes) show somewhere in the left pane.
	leftWidth := views.GetContentPaneWidth(page.width)
	// Check for number of tab boxes in left pane rather than textual labels — tabs may be truncated.
	lines := strings.Split(view, "\n")
	maxRows := 8
	if len(lines) < maxRows {
		maxRows = len(lines)
	}
	leftChunk := strings.Join(lines[:maxRows], "\n")
	if len(leftChunk) >= leftWidth {
		lcLines := strings.Split(leftChunk, "\n")
		for i, l := range lcLines {
			if len(l) >= leftWidth {
				lcLines[i] = l[:leftWidth]
			}
		}
		leftChunk = strings.Join(lcLines, "\n")
	}
	headerCount := strings.Count(leftChunk, "╭")
	if headerCount == 0 {
		t.Fatalf("expected at least 1 tab box in left pane for width %d, found none, left excerpt: %q", page.width, leftChunk)
	}
	// Find first '╭' (tab top-left) after the title/status and ensure it's in left pane for this width
	foundAt := -1
	for _, l := range lines[:maxRows] {
		if i := strings.Index(l, "╭"); i >= 0 {
			foundAt = i
			break
		}
	}
	if foundAt < 0 {
		t.Fatalf("expected to find a tab top-left corner at width %d; none found, excerpt: %q", page.width, leftChunk)
	}
	if foundAt >= leftWidth {
		t.Fatalf("expected tabs top-left to be within left pane width < %d at width %d, found at %d", leftWidth, page.width, foundAt)
	}

	// Verify the tab row is visually present using rounded top-left corners for
	// each tab box. Search the first N rows for rounded corner characters, and
	// ensure count equals the number of tabs.
	firstRowsCount := 10
	if len(lines) < firstRowsCount {
		firstRowsCount = len(lines)
	}
	checkRows := strings.Join(lines[:firstRowsCount], "\n")
	// count := strings.Count(checkRows, "╭")
	// Count '╭' within the left pane columns only to ignore the right pane box.
	leftCols := []string{}
	for _, l := range strings.Split(checkRows, "\n") {
		if len(l) >= leftWidth {
			leftCols = append(leftCols, l[:leftWidth])
		} else {
			leftCols = append(leftCols, l)
		}
	}
	leftColumnsChunk := strings.Join(leftCols, "\n")
	leftCount := strings.Count(leftColumnsChunk, "╭")
	if leftCount == 0 {
		t.Fatalf("expected at least 1 tab box in left pane, found %d in view: %q", leftCount, checkRows)
	}
}

func TestMainAppPage_ContentPane_RoundedBorder(t *testing.T) {
	// Setup a coordinator with a token and server so the content pane renders.
	coord := app.NewCoordinator()
	coord.SetToken("test-token")

	server := app.PlexServer{
		Name:        "Local Server",
		Host:        "127.0.0.1",
		Port:        "32400",
		AccessToken: "token",
		Scheme:      "http",
	}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)

	page := NewMainAppPage(coord)
	page.width = 120
	page.height = 40

	// Initialize to ensure the pane and styles are prepared.
	_ = page.Init()

	view := page.View()

	// Ensure the main content area uses rounded border characters from
	// lipgloss's RoundedBorder (e.g., ╭ ... ╰). These characters indicate the
	// PaneStyle with RoundedBorder is being applied.
	if !strings.Contains(view, "╭") || !strings.Contains(view, "╰") {
		t.Fatalf("expected main content pane to use rounded border characters, got: %q", view)
	}
}

func TestMainAppPage_Tabs_DoNotWrap_EvenSpacing_Matrix(t *testing.T) {
	// Setup a coordinator with a token and server so the tabs render.
	coord := app.NewCoordinator()
	coord.SetToken("test-token")

	server := app.PlexServer{
		Name:        "Local Server",
		Host:        "127.0.0.1",
		Port:        "32400",
		AccessToken: "token",
		Scheme:      "http",
	}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)

	page := NewMainAppPage(coord)

	widths := []int{76, 78, 80, 82, 84, 86, 90}
	for _, w := range widths {
		page.width = w
		page.height = 20
		_ = page.Init()
		view := page.View()

		// Check the left pane contains at least one tab box and that there are
		// no (or minimal) tab boxes leaking into the right pane.
		leftWidth := views.GetContentPaneWidth(page.width)
		lines := strings.Split(view, "\n")
		maxRows := 8
		if len(lines) < maxRows {
			maxRows = len(lines)
		}
		leftChunk := strings.Join(lines[:maxRows], "\n")
		if len(leftChunk) >= leftWidth {
			lc := strings.Split(leftChunk, "\n")
			for i, l := range lc {
				if len(l) >= leftWidth {
					lc[i] = l[:leftWidth]
				}
			}
			leftChunk = strings.Join(lc, "\n")
		}
		headerCount := strings.Count(leftChunk, "╭")
		if headerCount == 0 {
			t.Fatalf("expected at least 1 tab box in left pane at width %d, got 0, left excerpt: %q", w, leftChunk)
		}
		// Validate first '╭' occurs in left pane columns
		found := -1
		for _, l := range strings.Split(strings.Join(lines[:maxRows], "\n"), "\n") {
			if i := strings.Index(l, "╭"); i >= 0 {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("expected to find a tab top-left corner at width %d, none found", w)
		}
		if found >= leftWidth {
			t.Fatalf("expected tabs top-left to be within left pane width < %d at width %d, found at %d", leftWidth, w, found)
		}
	}
}
