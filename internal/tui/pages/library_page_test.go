package pages

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	components "plexmusic-tui/internal/tui/components"
	util "plexmusic-tui/internal/tui/util"
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

	// Confirm Nothing Playing placeholder is visible by default (no drawer)
	view := page.View()
	if !strings.Contains(view, "Nothing Playing") {
		t.Fatalf("expected Home view to show Now Playing placeholder, got: %q", view)
	}

	// Open the drawer explicitly so the "Recently Added" list is shown over Now Playing
	page.drawerOpen = true
	view = page.View()
	if !strings.Contains(view, "Test Album") {
		t.Fatalf("expected Home view to include recently added album title, got: %q", view)
	}
	// When drawer is open, Now Playing may be hidden; ensure the left pane shows the list
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

	// Open the drawer explicitly so the playlist list is shown over Now Playing
	page.drawerOpen = true

	view := page.View()
	if !strings.Contains(view, "Test Playlist") {
		t.Fatalf("expected Playlists view to include playlist title, got: %q", view)
	}
}

func TestLibraryPage_DefaultLayout_ShowsNowPlayingAndQueue(t *testing.T) {
	coord := app.NewCoordinator()
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.coordinator.SetActiveTab(app.HomeTab)
	page.width = 120
	page.height = 30

	// Populate queue so it shows up (otherwise retro logo is shown)
	coord.SetQueue([]app.Track{{Title: "Test Track"}})

	view := page.View()
	if !strings.Contains(view, "Nothing Playing") {
		t.Fatalf("expected Now Playing placeholder in default view; got: %q", view)
	}
	if !strings.Contains(view, "Queue") {
		t.Fatalf("expected Queue pane in default view; got: %q", view)
	}
}

func TestLibraryPage_DrawerOnRight_KeepsNowPlayingVisible(t *testing.T) {
	coord := app.NewCoordinator()
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	// Ensure we have items to show in drawer
	albums := []app.Album{{Title: "Test Album", Artist: "X", Key: "/library/metadata/1"}}
	coord.SetAlbums(albums)
	coord.SetActiveTab(app.HomeTab)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 30

	// Drawer defaults to closed; open it and verify it appears on the right and
	// that Now Playing (left) remains visible. Populate the left list so the
	// drawer has content to render on the right.
	// Send a library event simulating the server returned recently added albums
	evt := service.LibraryEvent{
		Type:   "recently_added.loaded",
		Albums: []domain.Album{{Title: "Test Album", Artist: "X", Year: 2020, Key: "/library/metadata/1"}},
	}
	page.Update(evt)
	page.drawerOpen = true
	view := page.View()

	if !strings.Contains(view, "Nothing Playing") {
		t.Fatalf("expected Now Playing placeholder in left pane when drawer opened; got: %q", view)
	}
	if !strings.Contains(view, "Test Album") {
		t.Fatalf("expected drawer contents to include Test Album on right; got: %q", view)
	}
}

func TestLibraryPage_Settings_ToggleCoverArtPosition(t *testing.T) {
	// Isolate config path by setting HOME to a temp dir.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgMgr, err := config.NewManager()
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	coord := app.NewCoordinator()
	coord.SetConfigManager(cfgMgr)
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40
	if page.Init() == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	// Wait for settingsComponent to be populated by Init
	start := time.Now()
	for len(page.settingsComponent.Items()) == 0 {
		if time.Since(start) > 2*time.Second {
			t.Fatalf("settingsComponent not initialized in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Ensure settings item exists and is a coverArtPos choice
	idx := -1
	for i, it := range page.settingsComponent.Items() {
		if s, ok := it.(util.SettingsItem); ok {
			if s.Key == "coverArtPos" {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		t.Fatalf("expected coverArtPos setting item")
	}

	// Open settings and select the item
	coord.SetActiveTab(app.SettingsTab)
	page.drawerOpen = true
	page.settingsComponent.Select(idx)

	// The initial setting should be the default (left)
	if cfgMgr.GetCoverArtPosition() != "left" {
		// default may be "left" if not set; ensure test is predictable by forcing default
		cfgMgr.SetCoverArtPosition("left")
	}

	// Press Enter to toggle the setting
	m, _ := page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = m

	// The setting should now be toggled to 'right'
	if cfgMgr.GetCoverArtPosition() != "right" {
		t.Fatalf("expected cover art to be toggled to right; got %s", cfgMgr.GetCoverArtPosition())
	}
}

func TestLibraryPage_SwitchView_PressesOpenDrawer(t *testing.T) {
	coord := app.NewCoordinator()
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Press '1' to select Home and open drawer if closed — numeric-only
	m, _ := page.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	_ = m
	if !page.drawerOpen {
		t.Fatalf("expected drawerOpen true after pressing '1'")
	}
}

func TestLibraryPage_LineCountStableOnSwitchView(t *testing.T) {
	coord := app.NewCoordinator()
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	// prepare some albums so the drawer has content
	albums := []app.Album{{Title: "Test Album", Artist: "X", Key: "/library/metadata/1"}}
	coord.SetAlbums(albums)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Ensure page renders consistently before opening drawer
	before := page.View()
	beforeLines := strings.Count(before, "\n")

	// Press '1' to switch and open drawer
	page.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})

	after := page.View()
	afterLines := strings.Count(after, "\n")

	if beforeLines != afterLines {
		t.Fatalf("expected same number of lines before/after SwitchView; before=%d after=%d\npre:\n%s\npost:\n%s", beforeLines, afterLines, before, after)
	}
}

func TestLibraryPage_SelectTrack_DoesNotDuplicateTrackList_LeftArt(t *testing.T) {
	coord := app.NewCoordinator()
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	albums := []app.Album{{Title: "Test Album", Artist: "X", Key: "/library/metadata/1"}}
	coord.SetAlbums(albums)
	coord.SetSelectedAlbum(0)
	coord.SetActiveTab(app.HomeTab)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Prepare a tracklist for the album and set showingTracks directly
	tracks := []domain.Track{{Title: "T1", Artist: "X", Album: "Test Album"}}
	items := make([]list.Item, len(tracks))
	for i, t := range tracks {
		items[i] = util.TrackItem{Track: t}
	}
	page.trackComponent.SetItems(items)
	page.trackComponent.Select(0)
	page.coordinator.SetTracks([]app.Track{{Title: "T1", Artist: "X", Album: "Test Album"}})
	page.showingTracks = true
	// Also open the drawer to simulate the case that previously caused duplication
	page.drawerOpen = true

	view := page.View()
	// Expect the track title to show once in the view overall and not be duplicated
	if strings.Count(view, "T1") != 1 {
		t.Fatalf("expected track title once in view, got: %d occurrences\nview:\n%s", strings.Count(view, "T1"), view)
	}
}

func TestLibraryPage_SelectTrack_DoesNotDuplicateTrackList_RightArt(t *testing.T) {
	// Set cover art position to right so content remains left; verify duplication does not occur
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgMgr, _ := config.NewManager()
	cfgMgr.SetCoverArtPosition("right")

	coord := app.NewCoordinator()
	coord.SetConfigManager(cfgMgr)
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	albums := []app.Album{{Title: "Test Album", Artist: "X", Key: "/library/metadata/1"}}
	coord.SetAlbums(albums)
	coord.SetSelectedAlbum(0)
	coord.SetActiveTab(app.HomeTab)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Prepare a tracklist and set showingTracks directly
	tracks := []domain.Track{{Title: "T1", Artist: "X", Album: "Test Album"}}
	items := make([]list.Item, len(tracks))
	for i, t := range tracks {
		items[i] = util.TrackItem{Track: t}
	}
	page.trackComponent.SetItems(items)
	page.trackComponent.Select(0)
	page.coordinator.SetTracks([]app.Track{{Title: "T1", Artist: "X", Album: "Test Album"}})
	page.showingTracks = true
	// Also open the drawer to simulate the case where the drawer conflicts
	page.drawerOpen = true

	view := page.View()
	if strings.Count(view, "T1") != 1 {
		t.Fatalf("expected track title once in view, got: %d occurrences\nview:\n%s", strings.Count(view, "T1"), view)
	}
}

func TestLibraryPage_ArtLoad_NoExtraLine(t *testing.T) {
	coord := app.NewCoordinator()
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// initial render with no art
	before := page.View()
	beforeLines := strings.Count(before, "\n")

	// Simulate loading of a tiny image
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	page.coordinator.SetPlaybackAlbumArt(img, "/thumb.png")

	// Now render again — no extra blank lines should appear due to trailing newline
	after := page.View()
	afterLines := strings.Count(after, "\n")

	if beforeLines != afterLines {
		t.Fatalf("expected same number of lines before and after art load; before=%d after=%d\nviewBefore:\n%s\nviewAfter:\n%s", beforeLines, afterLines, before, after)
	}
}

// Debug test: assert the view lines are consistent and print the first differing line
func TestLibraryPage_ArtLoad_LineDiff(t *testing.T) {
	coord := app.NewCoordinator()
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	before := page.View()
	beforeLines := strings.Split(before, "\n")

	// Simulate loading art
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	page.coordinator.SetPlaybackAlbumArt(img, "/thumb.png")
	after := page.View()
	afterLines := strings.Split(after, "\n")

	// Count blank/empty lines (after trimming whitespace) in both views
	beforeEmpty := 0
	for _, l := range beforeLines {
		if strings.TrimSpace(l) == "" {
			beforeEmpty++
		}
	}
	afterEmpty := 0
	for _, l := range afterLines {
		if strings.TrimSpace(l) == "" {
			afterEmpty++
		}
	}
	if afterEmpty > beforeEmpty {
		t.Fatalf("expected no additional blank lines after art load; before empty=%d after empty=%d\nviewBefore:\n%s\nviewAfter:\n%s", beforeEmpty, afterEmpty, before, after)
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
	_, _, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	_, _, err = page.libSvc.FetchPlaylists(ctx)
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
	_, _, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	// Read and apply the event(s)
	ev := <-evCh
	page.Update(ev.Payload)

	// Ensure the list contains multiple albums and the selection starts at 0
	if len(page.recentlyAddedComponent.Items()) < 2 {
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
	// Open the drawer on the right so track list is visible
	page.drawerOpen = true
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
	_, _, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	// Read and apply the event(s)
	ev := <-evCh
	page.Update(ev.Payload)

	// Ensure the list contains multiple albums and the selection starts at 0
	if len(page.recentlyAddedComponent.Items()) < 2 {
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
	// Open the drawer on the right so track list is visible
	page.drawerOpen = true
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
	_, _, err := page.libSvc.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	if len(page.playlistComponent.Items()) == 0 {
		t.Fatalf("expected playlists after fetch")
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
	}
	if !seenTrack {
		t.Fatalf("expected tracks.loaded event for selected playlist 2")
	}
	// After the prefetch, the page should NOT automatically show tracks until Enter is pressed
	if page.showingTracks {
		t.Fatalf("did not expect page to show tracks on selection change")
	}
	// Open the drawer on the right so track list is visible
	page.drawerOpen = true
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
	_, _, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	// Ensure album is present
	if len(page.recentlyAddedComponent.Items()) == 0 {
		t.Fatalf("expected recently added albums after fetch")
	}
	// Make sure tab is active and selection set
	page.coordinator.SetActiveTab(app.HomeTab)
	page.coordinator.SetSelectedAlbum(0)

	// Open the drawer on the right so the track list shows
	page.drawerOpen = true
	// Press Enter and expect the track list to open (no immediate playback)
	// ensure the drawer is open so the playlist contents appear
	page.drawerOpen = true
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
	_, _, err := page.libSvc.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	if len(page.playlistComponent.Items()) == 0 {
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
	_, _, err := page.libSvc.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	if len(page.recentlyAddedComponent.Items()) == 0 {
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
	if page.showingTracks {
		t.Fatalf("expected page.showingTracks to be false after pressing p on album (should switch to queue)")
	}
	if coord.ActiveTab() != app.QueueTab {
		t.Fatalf("expected active tab to be QueueTab after pressing p on album, got %v", coord.ActiveTab())
	}
	view := page.View()
	if !strings.Contains(view, "T1") {
		t.Fatalf("expected view to contain currently playing track T1, got: %s", view)
	}
}

// mockPbSvcOK is a minimal playback service used for UI tests that
// need to exercise orchestrator playback orchestration without actually
// initializing audio. It satisfies service.PlaybackServicer.
type mockPbSvcOK struct {
	ch chan pubsub.Event[service.PlaybackEvent]
}

func (m *mockPbSvcOK) Play(track *domain.Track) error { return nil }
func (m *mockPbSvcOK) Pause() error                   { return nil }
func (m *mockPbSvcOK) Resume() error                  { return nil }
func (m *mockPbSvcOK) Stop() error                    { return nil }
func (m *mockPbSvcOK) Seek(position int) error        { return nil }
func (m *mockPbSvcOK) SetVolume(v float64)            {}
func (m *mockPbSvcOK) GetVolume() float64             { return 0 }
func (m *mockPbSvcOK) GetPosition() int               { return 0 }
func (m *mockPbSvcOK) GetDuration() int               { return 0 }
func (m *mockPbSvcOK) GetState() domain.PlaybackState { return domain.PlaybackPlaying }
func (m *mockPbSvcOK) PlayDomainTrack(ctx context.Context, lib interface {
	FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error)
}, track *domain.Track,
) error {
	return nil
}

func (m *mockPbSvcOK) Subscribe(ctx context.Context) <-chan pubsub.Event[service.PlaybackEvent] {
	if m.ch == nil {
		ch := make(chan pubsub.Event[service.PlaybackEvent], 4)
		close(ch)
		return ch
	}
	return m.ch
}

func TestLibraryPage_PlaySelected_QueuesTracksFromSelection(t *testing.T) {
	coord := app.NewCoordinator()
	// Simulate authenticated server so page can build layout; orchestrator will be swapped with a mock below.
	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}})
	coord.SetSelectedServer(0)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Build a simple in-memory tracklist on the page and select the second track.
	appTracks := []app.Track{
		{Title: "T1", Artist: "Artist", Album: "Album", Key: "/t1"},
		{Title: "T2", Artist: "Artist", Album: "Album", Key: "/t2"},
		{Title: "T3", Artist: "Artist", Album: "Album", Key: "/t3"},
	}
	coord.SetTracks(appTracks)

	items := make([]list.Item, len(appTracks))
	for i, t := range appTracks {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			items[i] = util.TrackItem{Track: *dt}
		}
	}
	page.trackComponent.SetItems(items)
	page.trackComponent.Select(1)
	coord.SetSelectedTrack(1)
	page.showingTracks = true

	// Use a minimal mock playback service so we don't initialize audio.
	pb := &mockPbSvcOK{ch: make(chan pubsub.Event[service.PlaybackEvent], 4)}
	page.orchestrator = tui.NewOrchestrator(coord, nil, pb)
	page.pbEvtCh = page.orchestrator.Subscribe(page.ctx)
	page.nowPlaying = components.NewNowPlayingComponent(coord, page.orchestrator)

	// Press space (PlaySelected) to queue from selected track and start playback.
	_, cmd := page.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if cmd != nil {
		_ = cmd()
	}

	q := coord.Queue()
	if len(q) != 2 {
		t.Fatalf("expected queue to contain 2 items (selected + remaining), got %d", len(q))
	}
	if coord.QueueIndex() != 0 {
		t.Fatalf("expected queue index to be 0, got %d", coord.QueueIndex())
	}
	if !coord.HasCurrentTrack() {
		t.Fatalf("expected coordinator to have a current track set")
	}
	if coord.CurrentTrack().Title != "T2" {
		t.Fatalf("expected currently playing track to be T2, got %s", coord.CurrentTrack().Title)
	}
}

func TestLibraryPage_AutoAdvance_QueuePlaysNext(t *testing.T) {
	coord := app.NewCoordinator()

	coord.SetToken("test-token")
	coord.SetServers([]app.PlexServer{{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}})
	coord.SetSelectedServer(0)

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40

	// Prepare queue with two tracks and mark playing on the first.
	qTracks := []app.Track{
		{Title: "Q1", Artist: "Artist", Album: "Album", Key: "/q1"},
		{Title: "Q2", Artist: "Artist", Album: "Album", Key: "/q2"},
	}
	coord.SetQueue(qTracks)
	coord.SetQueueIndex(0)
	coord.SetCurrentTrack(&qTracks[0])
	coord.SetPlaybackState(app.PlaybackPlaying)

	// Use a minimal mock playback service and orchestrator.
	pb := &mockPbSvcOK{ch: make(chan pubsub.Event[service.PlaybackEvent], 4)}
	page.orchestrator = tui.NewOrchestrator(coord, nil, pb)
	page.pbEvtCh = page.orchestrator.Subscribe(page.ctx)
	page.nowPlaying = components.NewNowPlayingComponent(coord, page.orchestrator)

	// Simulate end-of-track via a playback.position event; position >= length triggers auto-advance
	ev := service.PlaybackEvent{
		Type:     "playback.position",
		Position: 100,
		Length:   100,
	}
	_, cmd := page.Update(ev)
	if cmd != nil {
		_ = cmd()
	}

	// Expect the queue to have advanced to the second track
	if coord.QueueIndex() != 1 {
		t.Fatalf("expected queue index 1 after auto-advance, got %d", coord.QueueIndex())
	}
	if !coord.HasCurrentTrack() {
		t.Fatalf("expected coordinator to have a current track set after auto-advance")
	}
	if coord.CurrentTrack().Title != "Q2" {
		t.Fatalf("expected current track to be Q2 after auto-advance, got %s", coord.CurrentTrack().Title)
	}
}

// Test that when the Queue modal is open, Up/Down keys scroll the queue instead
// of any other focused list (like Recently Added).
func TestLibraryPage_QueueModalInterceptsUpDown(t *testing.T) {
	coord := app.NewCoordinator()

	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40
	if page.Init() == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	// Provide a recently-added album so a different list is visible
	items := []list.Item{
		util.AlbumItem{Album: domain.Album{Title: "Album A", Artist: "Artist", Year: 2020, Key: "/library/metadata/1"}},
	}
	page.recentlyAddedComponent.SetItems(items)
	page.recentlyAddedComponent.Select(0)

	// Populate queue with 3 items and verify queue list sync
	qTracks := []app.Track{
		{Title: "Q1", Artist: "Artist", Album: "Album", Key: "/q1"},
		{Title: "Q2", Artist: "Artist", Album: "Album", Key: "/q2"},
		{Title: "Q3", Artist: "Artist", Album: "Album", Key: "/q3"},
	}
	coord.SetQueue(qTracks)
	coord.SetQueueIndex(0)
	page.queueComponent.UpdateListFromCoordinator()
	page.queueComponent.Select(0)

	// Open the queue modal
	page.coordinator.SetShowQueueModal(true)

	oldQIdx := page.queueComponent.Index()
	oldRaIdx := page.recentlyAddedComponent.Index()

	m, cmd := page.Update(tea.KeyMsg{Type: tea.KeyDown})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}

	// Queue selection should change, recently added should remain unchanged
	if page.queueComponent.Index() != oldQIdx+1 {
		t.Fatalf("expected queue index to increment, got %d", page.queueComponent.Index())
	}
	if page.recentlyAddedComponent.Index() != oldRaIdx {
		t.Fatalf("expected recently added unchanged, got %d", page.recentlyAddedComponent.Index())
	}
}

// Test that toggling queue focus while on the Queue tab routes Up/Down to queue
// (and that toggling off restores the previous routing).
func TestLibraryPage_QueueFocusTogglesWithKeyOnQueueTab(t *testing.T) {
	coord := app.NewCoordinator()

	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

	page := NewLibraryPageWithAuth(coord, nil)
	page.width = 120
	page.height = 40
	if page.Init() == nil {
		t.Fatalf("expected Init to return a cmd")
	}

	// Populate a queue with multiple tracks
	qTracks := []app.Track{
		{Title: "Q1", Artist: "Artist", Album: "Album", Key: "/q1"},
		{Title: "Q2", Artist: "Artist", Album: "Album", Key: "/q2"},
		{Title: "Q3", Artist: "Artist", Album: "Album", Key: "/q3"},
	}
	coord.SetQueue(qTracks)
	coord.SetQueueIndex(0)
	page.queueComponent.UpdateListFromCoordinator()
	page.queueComponent.Select(0)

	// Put the page on the Queue tab (the tab is visible — focus should be off initially)
	coord.SetActiveTab(app.QueueTab)
	if page.IsFocusedQueue() {
		t.Fatalf("expected queue not to have focus initially")
	}

	// Press 'o' to toggle queue focus
	m, cmd := page.Update(tea.KeyMsg{Runes: []rune{'o'}})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}
	if !page.IsFocusedQueue() {
		t.Fatalf("expected queue to be focused after toggling focus via Queue key")
	}

	oldIdx := page.queueComponent.Index()
	// Press Down and expect queue index to advance
	m2, cmd2 := page.Update(tea.KeyMsg{Type: tea.KeyDown})
	page = m2.(*LibraryPage)
	if cmd2 != nil {
		_ = cmd2()
	}
	if page.queueComponent.Index() != oldIdx+1 {
		t.Fatalf("expected queue index to advance when queue is focused, got %d", page.queueComponent.Index())
	}

	// Toggle focus off again
	m3, cmd3 := page.Update(tea.KeyMsg{Runes: []rune{'o'}})
	page = m3.(*LibraryPage)
	if cmd3 != nil {
		_ = cmd3()
	}
	if page.IsFocusedQueue() {
		t.Fatalf("expected queue focus to be cleared after toggling again")
	}
}

func TestLibraryPage_QueueVisibleInterceptsUpDown(t *testing.T) {
	coord := app.NewCoordinator()
	server := app.PlexServer{Name: "Local Server", Host: "127.0.0.1", Port: "32400", AccessToken: "token", Scheme: "http"}
	coord.SetServers([]app.PlexServer{server})
	coord.SetSelectedServer(0)
	coord.SetToken("test-token")

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

	// Provide a recently-added album so a different list is visible
	items := []list.Item{
		util.AlbumItem{Album: domain.Album{Title: "Album A", Artist: "Artist", Year: 2020, Key: "/library/metadata/1"}},
	}
	page.recentlyAddedComponent.SetItems(items)
	page.recentlyAddedComponent.Select(0)

	// Populate queue with 3 items and verify queue list sync
	qTracks := []app.Track{
		{Title: "Q1", Artist: "Artist", Album: "Album", Key: "/q1"},
		{Title: "Q2", Artist: "Artist", Album: "Album", Key: "/q2"},
		{Title: "Q3", Artist: "Artist", Album: "Album", Key: "/q3"},
	}
	coord.SetQueue(qTracks)
	coord.SetQueueIndex(0)
	page.queueComponent.UpdateListFromCoordinator()
	page.queueComponent.Select(0)

	// Ensure default Home tab and that queue is visible as the right pane (no drawer or tracklist)
	coord.SetActiveTab(app.HomeTab)
	page.drawerOpen = false
	page.showingTracks = false

	oldQIdx := page.queueComponent.Index()
	oldRaIdx := page.recentlyAddedComponent.Index()

	// Press Down; expect queue index to change, recently added remains unchanged
	m, cmd := page.Update(tea.KeyMsg{Type: tea.KeyDown})
	page = m.(*LibraryPage)
	if cmd != nil {
		_ = cmd()
	}

	if page.queueComponent.Index() != oldQIdx+1 {
		t.Fatalf("expected queue index to increment, got %d", page.queueComponent.Index())
	}
	if page.recentlyAddedComponent.Index() != oldRaIdx {
		t.Fatalf("expected recently added unchanged, got %d", page.recentlyAddedComponent.Index())
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
	page.searchComponent.SetValue("super")
	// Open search drawer so results are visible
	page.drawerOpen = true
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
	_, _, err := page.libSvc.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists error: %v", err)
	}
	ev := <-evCh
	page.Update(ev.Payload)

	if len(page.playlistComponent.Items()) == 0 {
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
	if page.showingTracks {
		t.Fatalf("expected page.showingTracks to be false after pressing p on playlist (should switch to queue)")
	}
	if coord.ActiveTab() != app.QueueTab {
		t.Fatalf("expected active tab to be QueueTab after pressing p on playlist, got %v", coord.ActiveTab())
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
