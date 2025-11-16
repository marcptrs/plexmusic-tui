package pages

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
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
	// Check help hint is shown in playlists drawer too
	if !strings.Contains(view, "Enter: open • Space: play • Esc: close") {
		t.Fatalf("expected playlists drawer view to include help hint, got: %q", view)
	}
	// Now Playing should not be duplicated; ensure only one Now Playing title exists
	if strings.Count(view, "Now Playing") > 1 {
		t.Fatalf("expected single Now Playing header, found multiple: %q", view)
	}
}
