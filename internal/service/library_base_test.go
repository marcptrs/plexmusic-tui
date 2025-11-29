package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"plexmusic-tui/internal/domain"
	plexhttp "plexmusic-tui/internal/http"
)

func startTestServer(respBody string) *httptest.Server {
	h := http.NewServeMux()
	h.HandleFunc("/library/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, respBody)
	})
	h.HandleFunc("/playlists", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, respBody)
	})
	h.HandleFunc("/library/sections/1/albums", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, respBody)
	})
	// Also handle library-scoped recently added
	h.HandleFunc("/library/sections/1/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, respBody)
	})
	return httptest.NewServer(h)
}

func TestFetchRecentlyAddedReturnsAlbums(t *testing.T) {
	body := `{"MediaContainer":{"Metadata":[{"title":"Test Album","parentTitle":"Test Artist",` +
		`"year":2022,"key":"/library/metadata/123","thumb":"/thumb.jpg"}]}}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	albums, _, err := s.FetchRecentlyAdded(ctx)
	if err != nil {
		t.Fatalf("FetchRecentlyAdded failed: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("expected 1 album, got %d", len(albums))
	}
	if strings.TrimSpace(albums[0].Title) != "Test Album" {
		t.Fatalf("unexpected album title: %s", albums[0].Title)
	}
}

func TestAddPlexHeadersSetsHeaderAndQuery(t *testing.T) {
	body := `{"MediaContainer":{"Metadata":[{"title":"Test Album","parentTitle":"Artist",` +
		`"year":2020,"key":"/library/metadata/99","thumb":"/thumb.jpg"}]}}`
	mux := http.NewServeMux()
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, r *http.Request) {
		// Verify header and query param
		tokenHeader := r.Header.Get("X-Plex-Token")
		tokenQuery := r.URL.Query().Get("X-Plex-Token")
		if tokenHeader != "server-token" || tokenQuery != "server-token" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "missing token: header=%s query=%s", tokenHeader, tokenQuery)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, body)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "server-token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Fetch libraries - endpoint uses addPlexHeaders
	_, _, err := s.FetchLibraries(ctx)
	if err != nil {
		t.Fatalf("FetchLibraries failed: %v", err)
	}
}

func TestFetchPlaylistsReturnsPlaylists(t *testing.T) {
	body := `{"MediaContainer":{"Metadata":[{"title":"Test Playlist","key":"/playlists/1",` +
		`"leafCount":3,"duration":120000,"playlistType":"audio"}]}}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	playlists, _, err := s.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists failed: %v", err)
	}
	if len(playlists) != 1 {
		t.Fatalf("expected 1 playlist, got %d", len(playlists))
	}
	if strings.TrimSpace(playlists[0].Title) != "Test Playlist" {
		t.Fatalf("unexpected playlist title: %s", playlists[0].Title)
	}
}

func TestFetchAlbumsReturnsAlbums(t *testing.T) {
	body := `{"MediaContainer":{"Metadata":[{"title":"Album A","parentTitle":"Artist A",` +
		`"year":2021,"key":"/library/metadata/1","thumb":"/thumb1.jpg"}]}}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	albums, _, err := s.FetchAlbums(ctx, "1")
	if err != nil {
		t.Fatalf("FetchAlbums failed: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("expected 1 album, got %d", len(albums))
	}
	if strings.TrimSpace(albums[0].Title) != "Album A" {
		t.Fatalf("unexpected album title: %s", albums[0].Title)
	}
}

func TestFetchRecentlyAddedHandlesPlaylistFormat(t *testing.T) {
	// Some plex servers return playlists as top-level Playlist array in MediaContainer
	body := `{"Playlist":[{"title":"Test Playlist","key":"/playlists/1",` +
		`"leafCount":3,"duration":120000,"playlistType":"audio"}]}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call FetchPlaylists using the playlist-specific endpoint
	playlists, _, err := s.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists failed: %v", err)
	}
	if len(playlists) != 1 {
		t.Fatalf("expected 1 playlist, got %d", len(playlists))
	}
}

func TestFetchRecentlyAddedInLibraryReturnsAlbums(t *testing.T) {
	body := `{"MediaContainer":{"Metadata":[{"title":"Lib Album","parentTitle":"Lib Artist",` +
		`"year":2023,"key":"/library/metadata/456","thumb":"/thumb2.jpg"}]}}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	albums, _, err := s.FetchRecentlyAddedInLibrary(ctx, "1")
	if err != nil {
		t.Fatalf("FetchRecentlyAddedInLibrary failed: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("expected 1 album, got %d", len(albums))
	}
	if strings.TrimSpace(albums[0].Title) != "Lib Album" {
		t.Fatalf("unexpected album title: %s", albums[0].Title)
	}
}

func TestBuildStreamURLUsesMediaPartKeyOrKeyAndDoesNotDuplicateToken(t *testing.T) {
	srv := startTestServer(`{"MediaContainer":{"Metadata":[]}}`)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "server-token", plexhttp.NewFactory())

	// Case 1: Media part key present
	track1 := &domain.Track{
		Media: []struct {
			Part []struct {
				Key string `json:"key"`
			} `json:"Part"`
		}{
			{
				Part: []struct {
					Key string `json:"key"`
				}{
					{Key: "/library/parts/1"},
				},
			},
		},
	}

	url1, err := s.BuildStreamURL(track1)
	if err != nil {
		t.Fatalf("BuildStreamURL failed: %v", err)
	}
	if !strings.Contains(url1, "X-Plex-Token=server-token") {
		t.Fatalf("expected URL to include server token; got %s", url1)
	}

	// Case 2: Track key present and already has token param - should not duplicate
	track2 := &domain.Track{Key: "/library/metadata/2?X-Plex-Token=existing"}
	url2, err := s.BuildStreamURL(track2)
	if err != nil {
		t.Fatalf("BuildStreamURL failed: %v", err)
	}
	q := strings.Count(url2, "X-Plex-Token=")
	if q != 1 {
		t.Fatalf("expected token param to appear once, but got %d occurrences in %s", q, url2)
	}
}

func TestFetchTracksChildrenFallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/library/metadata/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"Metadata":[]}`)
	})
	mux.HandleFunc("/library/metadata/1/children", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(
			w,
			`{"Metadata":[{"title":"Child Track","grandparentTitle":"Artist","parentTitle":"Album",`+
				`"duration":1000,"index":1,"key":"/library/metadata/1/track/1"}]}`,
		)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tracks, _, err := s.FetchTracks(ctx, "/library/metadata/1")
	if err != nil {
		t.Fatalf("FetchTracks returned error: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track from children fallback, got %d", len(tracks))
	}
	if tracks[0].Title != "Child Track" {
		t.Fatalf("unexpected track title: %s", tracks[0].Title)
	}
}

func TestFetchTracksChildrenFallbackAbsoluteKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/library/metadata/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"Metadata":[]}`)
	})
	mux.HandleFunc("/library/metadata/1/children", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(
			w,
			`{"Metadata":[{"title":"Child Track Absolute","grandparentTitle":"Artist","parentTitle":"Album",`+
				`"duration":1000,"index":1,"key":"/library/metadata/1/track/1"}]}`,
		)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call FetchTracks with an absolute URL returned by some Plex servers
	absKey := srv.URL + "/library/metadata/1"
	tracks, _, err := s.FetchTracks(ctx, absKey)
	if err != nil {
		t.Fatalf("FetchTracks returned error for absolute key: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track from children fallback with absolute key, got %d", len(tracks))
	}
	if tracks[0].Title != "Child Track Absolute" {
		t.Fatalf("unexpected track title: %s", tracks[0].Title)
	}
}

// When recentlyAdded returns album entries with children, HasSonicAnalysis should
// fetch the children and detect sonic analysis fields on the tracks.
func TestHasSonicAnalysisWithAlbumChildren(t *testing.T) {
	childCalled := false
	// Mock server: /library/recentlyAdded returns album entries with key=/library/metadata/84564/children
	// and /library/metadata/84564/children returns a track with hasSonicAnalysis true.
	mux := http.NewServeMux()
	mux.HandleFunc("/library/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(
			w,
			`{"MediaContainer":{"Metadata":[{"title":"Album A",`+
				`"key":"/library/metadata/84564/children","parentTitle":"Artist A"}]}}`,
		)
	})
	mux.HandleFunc("/library/metadata/84564/children", func(w http.ResponseWriter, r *http.Request) {
		childCalled = true
		fmt.Fprintln(
			w,
			`{"Metadata":[{"title":"Track X","grandparentTitle":"Artist A",`+
				`"parentTitle":"Album A","hasSonicAnalysis":true,"musicAnalysisVersion":1}]}`,
		)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ok, err := s.HasSonicAnalysis(ctx)
	if err != nil {
		t.Fatalf("HasSonicAnalysis returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected HasSonicAnalysis to detect sonic analysis via album children, got false")
	}
	if !childCalled {
		t.Fatalf("expected child endpoint to be called, but it wasn't")
	}
}

func TestDecodePlexTrackContainerHubItems(t *testing.T) {
	// Some plex hubs return tracks nested under 'Hub' -> [{'Metadata': []}]
	body := `{"Hub":[{"Metadata":[{"title":"Hub Track","grandparentTitle":"Artist",` +
		`"parentTitle":"Album","duration":1000,"index":1,"key":"/library/metadata/1/track/1"}]}]}`
	_ = NewLibraryService("http://example.com", "token", plexhttp.NewFactory())
	var container domain.PlexTrackContainer
	if err := decodePlexTrackContainer([]byte(body), &container); err != nil {
		t.Fatalf("decodePlexTrackContainer failed to decode Hub payload: %v", err)
	}
	if len(container.Metadata) != 1 || container.Metadata[0].Title != "Hub Track" {
		t.Fatalf("unexpected decoded metadata: %+v", container.Metadata)
	}
}

func TestDecodePlexTrackContainerItemsArray(t *testing.T) {
	// Some responses return a top-level 'items' array of tracks
	body := `{"items":[{"title":"Item Track","grandparentTitle":"Artist",` +
		`"parentTitle":"Album","duration":1000,"index":1,"key":"/library/metadata/1/track/1"}]}`
	var container domain.PlexTrackContainer
	if err := decodePlexTrackContainer([]byte(body), &container); err != nil {
		t.Fatalf("decodePlexTrackContainer failed to decode items payload: %v", err)
	}
	if len(container.Metadata) != 1 || container.Metadata[0].Title != "Item Track" {
		t.Fatalf("unexpected decoded metadata: %+v", container.Metadata)
	}
}

func TestDecodePlexTrackContainerPlayQueue(t *testing.T) {
	// PlayQueue responses contain playQueueID, playQueueVersion at MediaContainer level
	body := `{"MediaContainer":{"size":3,"playQueueID":12345,"playQueueSelectedItemID":99,` +
		`"playQueueVersion":2,"Metadata":[` +
		`{"title":"Track One","grandparentTitle":"Artist","parentTitle":"Album",` +
		`"duration":1000,"index":1,"key":"/library/metadata/1"},` +
		`{"title":"Track Two","grandparentTitle":"Artist","parentTitle":"Album",` +
		`"duration":2000,"index":2,"key":"/library/metadata/2"}` +
		`]}}`
	var container domain.PlexTrackContainer
	if err := decodePlexTrackContainer([]byte(body), &container); err != nil {
		t.Fatalf("decodePlexTrackContainer failed to decode playQueue payload: %v", err)
	}
	if len(container.Metadata) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(container.Metadata))
	}
	if container.PlayQueueID != 12345 {
		t.Errorf("expected PlayQueueID=12345, got %d", container.PlayQueueID)
	}
	if container.PlayQueueSelectedItemID != 99 {
		t.Errorf("expected PlayQueueSelectedItemID=99, got %d", container.PlayQueueSelectedItemID)
	}
	if container.PlayQueueVersion != 2 {
		t.Errorf("expected PlayQueueVersion=2, got %d", container.PlayQueueVersion)
	}
}

// Per-library detection: HasSonicAnalysis should return true when a library's
// recentlyAdded contains a track with hasSonicAnalysis set to true.
func TestHasSonicAnalysisPerLibrary(t *testing.T) {
	mux := http.NewServeMux()
	// Global recentlyAdded returns empty or album entries without sonic fields
	mux.HandleFunc("/library/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"MediaContainer":{"Metadata":[]}}`)
	})
	// Return one library in sections
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"MediaContainer":{"Directory":[{"key":"1","type":"artist","title":"MusicLib"}]}}`)
	})
	// Per-library recentlyAdded returns a track with hasSonicAnalysis true
	mux.HandleFunc("/library/sections/1/recentlyAdded", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Type=10 style track items
		fmt.Fprintln(
			w,
			`{"Metadata":[{"title":"Library Track","grandparentTitle":"Artist",`+
				`"parentTitle":"Album","hasSonicAnalysis":true,"musicAnalysisVersion":1}]}`,
		)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token", plexhttp.NewFactory())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ok, err := s.HasSonicAnalysis(ctx)
	if err != nil {
		t.Fatalf("HasSonicAnalysis returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected HasSonicAnalysis to detect sonic analysis via per-library recentlyAdded, got false")
	}
}
