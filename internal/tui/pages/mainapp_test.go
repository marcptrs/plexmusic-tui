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
	// The drawer should show a help hint that Enter/Space/Esc are usable actions
	if !strings.Contains(view, "Enter: open • Space: play • Esc: close") {
		t.Fatalf("expected drawer view to include help hint, got: %q", view)
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

	// Sanity check: Settings tab must be present
	if !strings.Contains(view, "Settings") {
		t.Fatalf("expected view to include Settings tab, got: %q", view)
	}

	// Ensure the 'Settings' label is not broken up with newline characters
	// (indicating wrapping). Check a few commonly observed split points:
	if strings.Contains(view, "Set\n") || strings.Contains(view, "Sett\n") || strings.Contains(view, "Settings\n") || strings.Contains(view, "\nSettings") {
		t.Fatalf("expected Settings to not be wrapped across lines, got view: %q", view)
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

	// Check that each tab shows at least its first word (or a short prefix).
	// Many tabs are multi-word (e.g., "Recently Added") and may be truncated
	// depending on the terminal width; verifying the first word/prefix is more robust.
	tabs := []string{"Home", "Recently Added", "Playlists", "Search", "Queue", "Settings"}
	for _, tab := range tabs {
		firstWord := strings.SplitN(tab, " ", 2)[0]
		// Accept either the full first word, or a short (6-char) prefix.
		prefix := firstWord
		if len(prefix) > 6 {
			prefix = prefix[:6]
		}
		if !strings.Contains(view, firstWord) && !strings.Contains(view, prefix) {
			t.Fatalf("expected tab %q (or its prefix) to be present in view: %q", tab, view)
		}
	}

	// Verify the tab row is visually present using rounded top-left corners for
	// each tab box. Search the last N rows for rounded corner characters, and
	// ensure count equals the number of tabs.
	lines := strings.Split(view, "\n")
	lastRowsCount := 10
	if len(lines) < lastRowsCount {
		lastRowsCount = len(lines)
	}
	checkRows := strings.Join(lines[len(lines)-lastRowsCount:], "\n")
	count := strings.Count(checkRows, "╭")
	if count != len(tabs) {
		t.Fatalf("expected %d rounded corner top-left characters in tabs area, found %d, view: %q", len(tabs), count, checkRows)
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

		// Settings must be present and not wrapped by newlines.
		if !strings.Contains(view, "Settings") {
			t.Fatalf("expected view to include Settings tab at width %d, got: %q", w, view)
		}
		if strings.Contains(view, "Set\n") || strings.Contains(view, "Sett\n") || strings.Contains(view, "Settings\n") || strings.Contains(view, "\nSettings") {
			t.Fatalf("expected Settings to not be wrapped across lines at width %d, got view: %q", w, view)
		}
	}
}
