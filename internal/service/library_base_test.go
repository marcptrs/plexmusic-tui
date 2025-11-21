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
	body := `{"MediaContainer":{"Metadata":[{"title":"Test Album","parentTitle":"Test Artist","year":2022,"key":"/library/metadata/123","thumb":"/thumb.jpg"}]}}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	albums, err := s.FetchRecentlyAdded(ctx)
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
	body := `{"MediaContainer":{"Metadata":[{"title":"Test Album","parentTitle":"Artist","year":2020,"key":"/library/metadata/99","thumb":"/thumb.jpg"}]}}`
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

	s := NewLibraryService(srv.URL, "server-token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Fetch libraries - endpoint uses addPlexHeaders
	_, err := s.FetchLibraries(ctx)
	if err != nil {
		t.Fatalf("FetchLibraries failed: %v", err)
	}
}

func TestFetchPlaylistsReturnsPlaylists(t *testing.T) {
	body := `{"MediaContainer":{"Metadata":[{"title":"Test Playlist","key":"/playlists/1","leafCount":3,"duration":120000,"playlistType":"audio"}]}}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	playlists, err := s.FetchPlaylists(ctx)
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
	body := `{"MediaContainer":{"Metadata":[{"title":"Album A","parentTitle":"Artist A","year":2021,"key":"/library/metadata/1","thumb":"/thumb1.jpg"}]}}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	albums, err := s.FetchAlbums(ctx, "1")
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
	body := `{"Playlist":[{"title":"Test Playlist","key":"/playlists/1","leafCount":3,"duration":120000,"playlistType":"audio"}]}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call FetchPlaylists using the playlist-specific endpoint
	playlists, err := s.FetchPlaylists(ctx)
	if err != nil {
		t.Fatalf("FetchPlaylists failed: %v", err)
	}
	if len(playlists) != 1 {
		t.Fatalf("expected 1 playlist, got %d", len(playlists))
	}
}

func TestFetchRecentlyAddedInLibraryReturnsAlbums(t *testing.T) {
	body := `{"MediaContainer":{"Metadata":[{"title":"Lib Album","parentTitle":"Lib Artist","year":2023,"key":"/library/metadata/456","thumb":"/thumb2.jpg"}]}}`
	srv := startTestServer(body)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	albums, err := s.FetchRecentlyAddedInLibrary(ctx, "1")
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

	s := NewLibraryService(srv.URL, "server-token")

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
		fmt.Fprintln(w, `{"Metadata":[{"title":"Child Track","grandparentTitle":"Artist","parentTitle":"Album","duration":1000,"index":1,"key":"/library/metadata/1/track/1"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tracks, err := s.FetchTracks(ctx, "/library/metadata/1")
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
		fmt.Fprintln(w, `{"Metadata":[{"title":"Child Track Absolute","grandparentTitle":"Artist","parentTitle":"Album","duration":1000,"index":1,"key":"/library/metadata/1/track/1"}]}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewLibraryService(srv.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call FetchTracks with an absolute URL returned by some Plex servers
	absKey := srv.URL + "/library/metadata/1"
	tracks, err := s.FetchTracks(ctx, absKey)
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
