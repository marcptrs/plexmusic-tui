package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"plexmusic-tui/internal/domain"
	httpclient "plexmusic-tui/internal/http"
)

// Client handles Plex API communication
// Deprecated: Use service.LibraryService instead for new code.
// This client is maintained for backward compatibility only.
type Client struct {
	scheme      string
	host        string
	port        string
	accessToken string
	httpClient  *httpclient.Client
}

// NewClient creates a new Plex API client
// Deprecated: Use service.NewLibraryService instead for new code.
func NewClient(scheme, host, port, accessToken string, httpClient *http.Client) *Client {
	// Extract host for intelligent HTTP client selection
	hostPort := fmt.Sprintf("%s:%s", host, port)

	client := httpclient.GetForHost(hostPort)
	if httpClient != nil {
		// If custom HTTP client provided, wrap it
		client = &httpclient.Client{}
	}

	return &Client{
		scheme:      scheme,
		host:        host,
		port:        port,
		accessToken: accessToken,
		httpClient:  client,
	}
}

// FetchLibraries fetches all music libraries from the Plex server
func (c *Client) FetchLibraries() ([]domain.MusicLibrary, error) {
	url := fmt.Sprintf("%s://%s:%s/library/sections", c.scheme, c.host, c.port)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch libraries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("library fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var container domain.PlexMediaContainer
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter for music libraries only. Use a whitelist of known music types
	// to avoid accidentally returning TV/movie/photo sections.
	allowed := map[string]bool{"artist": true, "music": true, "album": true, "collection": true, "music_video": true}
	var musicLibs []domain.MusicLibrary
	for _, lib := range container.Directory {
		if allowed[strings.ToLower(lib.Type)] {
			musicLibs = append(musicLibs, lib)
		}
	}

	return musicLibs, nil
}

// FetchAlbums fetches albums from a specific library
func (c *Client) FetchAlbums(libraryKey string) ([]domain.Album, error) {
	url := fmt.Sprintf("%s://%s:%s/library/sections/%s/albums", c.scheme, c.host, c.port, libraryKey)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch albums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("album fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var container domain.PlexMediaContainer
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return container.Metadata, nil
}

// FetchRecentlyAdded fetches recently added albums
func (c *Client) FetchRecentlyAdded() ([]domain.Album, error) {
	url := fmt.Sprintf("%s://%s:%s/library/recentlyAdded?type=9", c.scheme, c.host, c.port)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recently added: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recently added fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var container domain.PlexMediaContainer
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return container.Metadata, nil
}

// FetchPlaylists fetches all playlists from the server
func (c *Client) FetchPlaylists() ([]domain.Playlist, error) {
	url := fmt.Sprintf("%s://%s:%s/playlists", c.scheme, c.host, c.port)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlists: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playlist fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	// First read body to bytes
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Then unmarshal
	var container struct {
		Metadata []domain.Playlist `json:"Playlist"`
	}
	if err := json.Unmarshal(bodyBytes, &container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return container.Metadata, nil
}

// FetchTracks fetches tracks from an album or playlist
func (c *Client) FetchTracks(key string) ([]domain.Track, error) {
	url := fmt.Sprintf("%s://%s:%s%s", c.scheme, c.host, c.port, key)

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tracks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("track fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var container domain.PlexTrackContainer
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return container.Metadata, nil
}

// FetchAlbumArt fetches the album art image for an album
func (c *Client) FetchAlbumArt(thumbPath string) ([]byte, error) {
	url := fmt.Sprintf("%s://%s:%s%s?X-Plex-Token=%s", c.scheme, c.host, c.port, thumbPath, c.accessToken)

	resp, err := c.httpClient.Get(context.Background(), url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch album art: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("album art fetch failed (status %d)", resp.StatusCode)
	}

	imageBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	return imageBytes, nil
}

// BuildStreamURL constructs the URL for streaming a track
func (c *Client) BuildStreamURL(track domain.Track) string {
	if len(track.Media) > 0 && len(track.Media[0].Part) > 0 {
		return fmt.Sprintf("%s://%s:%s%s?X-Plex-Token=%s", c.scheme, c.host, c.port, track.Media[0].Part[0].Key, c.accessToken)
	}
	return fmt.Sprintf("%s://%s:%s%s?X-Plex-Token=%s", c.scheme, c.host, c.port, track.Key, c.accessToken)
}
