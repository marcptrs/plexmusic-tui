package plex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"plexmusic-tui/internal/domain"
)

const (
	plexTVURL = "https://plex.tv"
)

// APIClient handles all Plex API requests
type APIClient struct {
	httpClient *HTTPClient
	token      string
	serverHost string
	serverPort string
	scheme     string
}

// NewAPIClient creates a new Plex API client
func NewAPIClient(serverHost, serverPort, scheme, token string) *APIClient {
	config := &ClientConfig{
		Token:      token,
		ServerHost: serverHost,
		ServerPort: serverPort,
		Scheme:     scheme,
	}
	httpClient := NewHTTPClient(config)
	return &APIClient{
		httpClient: httpClient,
		token:      token,
		serverHost: serverHost,
		serverPort: serverPort,
		scheme:     scheme,
	}
}

// FetchServers fetches available Plex servers for the authenticated user
func FetchServers(token string) ([]domain.PlexServer, error) {
	url := plexTVURL + "/api/v2/resources?includeHttps=1&includeRelay=0"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", token)
	req.Header.Set("X-Plex-Product", "Plex TUI")
	req.Header.Set("X-Plex-Version", "1.0")
	req.Header.Set("X-Plex-Client-Identifier", "plexmusic-tui-v1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var resources []domain.PlexResourceResponse
	if err := json.NewDecoder(resp.Body).Decode(&resources); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var servers []domain.PlexServer
	for _, resource := range resources {
		// Only include actual Plex Media Servers (not clients or other devices)
		// The "provides" field contains "server" for Plex Media Servers
		if !strings.Contains(resource.Provides, "server") {
			continue
		}

		if len(resource.Connections) > 0 {
			// Prefer local connections
			var bestConn *struct {
				Protocol string `json:"protocol"`
				Address  string `json:"address"`
				Port     int    `json:"port"`
				Local    bool   `json:"local"`
			}

			for i := range resource.Connections {
				conn := &resource.Connections[i]
				if bestConn == nil || (conn.Local && !bestConn.Local) {
					bestConn = conn
				}
			}

			if bestConn != nil {
				servers = append(servers, domain.PlexServer{
					Name:        resource.Name,
					Host:        bestConn.Address,
					Port:        fmt.Sprintf("%d", bestConn.Port),
					AccessToken: resource.AccessToken,
					Scheme:      bestConn.Protocol,
				})
			}
		}
	}

	return servers, nil
}

// FetchLibraries fetches music libraries from the server
func (a *APIClient) FetchLibraries() ([]domain.MusicLibrary, error) {
	url := fmt.Sprintf("%s://%s:%s/library/sections", a.scheme, a.serverHost, a.serverPort)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", a.token)

	client := a.httpClient.GetClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch libraries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("library fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var container struct {
		MediaContainer domain.PlexMediaContainer `json:"MediaContainer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter for music libraries only
	var musicLibs []domain.MusicLibrary
	for _, lib := range container.MediaContainer.Directory {
		if lib.Type == "artist" {
			musicLibs = append(musicLibs, lib)
		}
	}

	return musicLibs, nil
}

// FetchAlbums fetches albums from a library
func (a *APIClient) FetchAlbums(libraryKey string) ([]domain.Album, error) {
	url := fmt.Sprintf("%s://%s:%s/library/sections/%s/albums", a.scheme, a.serverHost, a.serverPort, libraryKey)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", a.token)

	client := a.httpClient.GetClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch albums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("album fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var container struct {
		MediaContainer domain.PlexMediaContainer `json:"MediaContainer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return container.MediaContainer.Metadata, nil
}

// FetchRecentlyAdded fetches recently added albums from the server
func (a *APIClient) FetchRecentlyAdded() ([]domain.Album, error) {
	url := fmt.Sprintf("%s://%s:%s/library/recentlyAdded?type=9", a.scheme, a.serverHost, a.serverPort)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", a.token)

	client := a.httpClient.GetClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recently added: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recently added fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	var container struct {
		MediaContainer domain.PlexMediaContainer `json:"MediaContainer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return container.MediaContainer.Metadata, nil
}

// FetchPlaylists fetches all playlists from the server
func (a *APIClient) FetchPlaylists() ([]domain.Playlist, error) {
	url := fmt.Sprintf("%s://%s:%s/playlists", a.scheme, a.serverHost, a.serverPort)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", a.token)

	client := a.httpClient.GetClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlists: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playlist fetch failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Read and log the response body for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var container struct {
		MediaContainer domain.PlexPlaylistContainer `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (body: %s)", err, string(body))
	}

	return container.MediaContainer.Metadata, nil
}

// FetchTracks fetches tracks from a Plex key (album or playlist)
func (a *APIClient) FetchTracks(key, source string) ([]domain.Track, error) {
	url := fmt.Sprintf("%s://%s:%s%s", a.scheme, a.serverHost, a.serverPort, key)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", a.token)

	client := a.httpClient.GetClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s tracks: %w", source, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s tracks fetch failed (status %d): %s", source, resp.StatusCode, string(body))
	}

	var container struct {
		MediaContainer domain.PlexTrackContainer `json:"MediaContainer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return container.MediaContainer.Metadata, nil
}

// FetchAudioStream fetches audio stream for a track
func (a *APIClient) FetchAudioStream(streamURL string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Plex-Token", a.token)

	client := a.httpClient.GetClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch audio: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("audio fetch failed (status %d)", resp.StatusCode)
	}

	return resp.Body, nil
}

// BuildStreamURL constructs the stream URL for a track
func (a *APIClient) BuildStreamURL(track *domain.Track) (string, error) {
	if len(track.Media) == 0 || len(track.Media[0].Part) == 0 {
		return "", fmt.Errorf("track has no media parts")
	}

	partKey := track.Media[0].Part[0].Key
	return fmt.Sprintf("%s://%s:%s%s", a.scheme, a.serverHost, a.serverPort, partKey), nil
}
