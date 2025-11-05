package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"plexmusic-tui/internal/domain"
)

const (
	clientIdentifier = "plexmusic-tui-v1"
	plexTVURL        = "https://plex.tv"
)

// Authenticator handles Plex authentication
type Authenticator struct {
	httpClient *http.Client
}

// NewAuthenticator creates a new Authenticator instance
func NewAuthenticator() *Authenticator {
	return &Authenticator{
		httpClient: &http.Client{},
	}
}

// AuthenticateUser authenticates a user with Plex using username and password
// Returns an auth token if successful, or an error
func (a *Authenticator) AuthenticateUser(username, password string) (string, error) {
	// Validate inputs
	if username == "" || password == "" {
		return "", fmt.Errorf("username or password is empty")
	}

	// Use form encoding with user[login] and user[password] format
	formData := url.Values{}
	formData.Set("user[login]", username)
	formData.Set("user[password]", password)

	requestURL := plexTVURL + "/users/sign_in.json"
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", "Plex TUI")
	req.Header.Set("X-Plex-Version", "1.0")
	req.Header.Set("X-Plex-Client-Identifier", clientIdentifier)
	req.Header.Set("X-Plex-Platform", "Linux")
	req.Header.Set("X-Plex-Device", "PC")
	req.Header.Set("X-Plex-Device-Name", "Plex TUI")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("authentication request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("authentication failed (status %d): %s\nUsername length: %d",
			resp.StatusCode, string(body), len(username))
	}

	var authResp domain.PlexAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if authResp.User.AuthToken == "" {
		return "", fmt.Errorf("no auth token received")
	}

	return authResp.User.AuthToken, nil
}

// FetchServers fetches the list of Plex servers available to the user
// Requires a valid authentication token
func (a *Authenticator) FetchServers(token string) ([]domain.PlexServer, error) {
	if token == "" {
		return nil, fmt.Errorf("authentication token is empty")
	}

	url := plexTVURL + "/api/v2/resources?includeHttps=1&includeRelay=0"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", token)
	req.Header.Set("X-Plex-Product", "Plex TUI")
	req.Header.Set("X-Plex-Version", "1.0")
	req.Header.Set("X-Plex-Client-Identifier", clientIdentifier)

	resp, err := a.httpClient.Do(req)
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
