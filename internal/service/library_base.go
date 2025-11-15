package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"plexmusic-tui/internal/domain"
	httpclient "plexmusic-tui/internal/http"
)

// LibraryService provides methods to fetch library data from Plex servers.
type LibraryService struct {
	baseURL    string // e.g., "https://192.168.1.100:32400"
	token      string
	httpClient *httpclient.Client
}

// NewLibraryService creates a new library service for a Plex server.
// baseURL should be the full URL to the Plex server (scheme + host + port)
// e.g., "https://192.168.1.100:32400" or "http://localhost:32400"
func NewLibraryService(baseURL, token string) *LibraryService {
	// Extract host from baseURL for intelligent TLS handling
	u, err := url.Parse(baseURL)
	if err != nil {
		// Fallback to standard client if URL parsing fails
		return &LibraryService{
			baseURL:    baseURL,
			token:      token,
			httpClient: httpclient.New(),
		}
	}

	return &LibraryService{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpclient.GetForHost(u.Host),
	}
}

// FetchLibraries fetches all music libraries from the server.
func (s *LibraryService) FetchLibraries(ctx context.Context) ([]domain.MusicLibrary, error) {
	endpoint := s.baseURL + "/library/sections"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add Plex headers
	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch libraries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var container domain.PlexMediaContainer
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode libraries: %w", err)
	}

	// Filter to music libraries only
	var libraries []domain.MusicLibrary
	for _, dir := range container.Directory {
		if dir.Type == "artist" {
			libraries = append(libraries, dir)
		}
	}

	return libraries, nil
}

// FetchAlbums fetches all albums from a specific library.
func (s *LibraryService) FetchAlbums(ctx context.Context, libraryKey string) ([]domain.Album, error) {
	endpoint := fmt.Sprintf("%s/library/sections/%s/albums", s.baseURL, libraryKey)
	return s.fetchAlbums(ctx, endpoint)
}

// FetchRecentlyAdded fetches recently added albums.
func (s *LibraryService) FetchRecentlyAdded(ctx context.Context) ([]domain.Album, error) {
	endpoint := fmt.Sprintf("%s/library/recentlyAdded?type=9", s.baseURL)
	return s.fetchAlbums(ctx, endpoint)
}

// fetchAlbums is a helper for fetching albums from any endpoint.
func (s *LibraryService) fetchAlbums(ctx context.Context, endpoint string) ([]domain.Album, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch albums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var container domain.PlexMediaContainer
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode albums: %w", err)
	}

	return container.Metadata, nil
}

// FetchPlaylists fetches all playlists from the server.
func (s *LibraryService) FetchPlaylists(ctx context.Context) ([]domain.Playlist, error) {
	endpoint := s.baseURL + "/playlists"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlists: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Playlists response has a different structure with Playlist field
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var rawResponse map[string]interface{}
	if err := json.Unmarshal(body, &rawResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Try to find playlist data in the response
	playlists := []domain.Playlist{}
	if metadata, ok := rawResponse["Metadata"].([]interface{}); ok {
		for _, item := range metadata {
			if playlistMap, ok := item.(map[string]interface{}); ok {
				var p domain.Playlist
				// Manual unmarshaling from the item
				if title, ok := playlistMap["title"].(string); ok {
					p.Title = title
				}
				if key, ok := playlistMap["key"].(string); ok {
					p.Key = key
				}
				if leafCount, ok := playlistMap["leafCount"].(float64); ok {
					p.LeafCount = int(leafCount)
				}
				if duration, ok := playlistMap["duration"].(float64); ok {
					p.Duration = int(duration)
				}
				if playlistType, ok := playlistMap["playlistType"].(string); ok {
					p.PlaylistType = playlistType
				}
				playlists = append(playlists, p)
			}
		}
	}

	return playlists, nil
}

// FetchTracks fetches tracks from a specific album or playlist.
// The key parameter should be the media key (e.g., /library/metadata/{id} or /playlists/{id})
func (s *LibraryService) FetchTracks(ctx context.Context, key string) ([]domain.Track, error) {
	endpoint := s.baseURL + key
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tracks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var container domain.PlexTrackContainer
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode tracks: %w", err)
	}

	return container.Metadata, nil
}

// BuildStreamURL constructs the URL for streaming an audio track.
// It prefers Media[0].Part[0].Key if available, falls back to track.Key.
func (s *LibraryService) BuildStreamURL(track *domain.Track) (string, error) {
	var key string

	// Prefer the full media part key
	if len(track.Media) > 0 && len(track.Media[0].Part) > 0 {
		key = track.Media[0].Part[0].Key
	} else {
		key = track.Key
	}

	if key == "" {
		return "", fmt.Errorf("no playable key found for track")
	}

	// Build full URL
	streamURL := s.baseURL + key + "?X-Plex-Token=" + url.QueryEscape(s.token)
	return streamURL, nil
}

// addPlexHeaders adds standard Plex API headers to a request.
func (s *LibraryService) addPlexHeaders(req *http.Request) {
	req.Header.Set("X-Plex-Token", s.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "plexmusic-tui")
}

// SetBaseURL updates the base URL (useful for switching servers).
func (s *LibraryService) SetBaseURL(baseURL string) {
	s.baseURL = baseURL
	// Update HTTP client for new host
	u, err := url.Parse(baseURL)
	if err == nil {
		s.httpClient = httpclient.GetForHost(u.Host)
	}
}

// SetToken updates the authentication token.
func (s *LibraryService) SetToken(token string) {
	s.token = token
}
