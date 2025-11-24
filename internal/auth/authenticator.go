package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"plexmusic-tui/internal/domain"
)

const (
	clientIdentifier = "plexmusic-tui-v1"
	plexTVURL        = "https://plex.tv"
)

// Authenticator handles Plex authentication
type Authenticator struct {
	httpClient domain.HTTPClient
}

// NewAuthenticator creates a new Authenticator instance
func NewAuthenticator(client domain.HTTPClient) *Authenticator {
	return &Authenticator{
		httpClient: client,
	}
}

// isReachable attempts a quick TCP connection to the provided host:port using a short timeout.
// It returns true if the host is reachable within the timeout.
func isReachable(ctx context.Context, host string, port int, timeout time.Duration) bool {
	if host == "" || port <= 0 {
		return false
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	address := net.JoinHostPort(host, strconv.Itoa(port))
	d := net.Dialer{}
	conn, err := d.DialContext(dialCtx, "tcp", address)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// AuthenticateUser authenticates a user with Plex using username and password
// Returns an auth token if successful, or an error
func (a *Authenticator) AuthenticateUser(
	ctx context.Context,
	username, password string,
) (string, error) {
	// Validate inputs
	if username == "" || password == "" {
		return "", fmt.Errorf("username or password is empty")
	}

	// Use form encoding with user[login] and user[password] format
	formData := url.Values{}
	formData.Set("user[login]", username)
	formData.Set("user[password]", password)

	requestURL := plexTVURL + "/users/sign_in.json"
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		requestURL,
		strings.NewReader(formData.Encode()),
	)
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
func (a *Authenticator) FetchServers(
	ctx context.Context,
	token string,
) ([]domain.PlexServer, error) {
	if token == "" {
		return nil, fmt.Errorf("authentication token is empty")
	}

	// includeRelay can be overridden via environment variable `PLEX_INCLUDE_RELAY`.
	// When set to '1', relays are included. Default behavior is '0'.
	relay := os.Getenv("PLEX_INCLUDE_RELAY")
	if relay != "1" {
		relay = "0"
	}
	url := fmt.Sprintf("%s/api/v2/resources?includeHttps=1&includeRelay=%s", plexTVURL, relay)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
			// Prefer reachable remote connections, then reachable local connections,
			// then fall back to the first remote or local address.
			firstLocalIdx := -1
			firstRemoteIdx := -1
			reachableLocalIdx := -1
			reachableRemoteIdx := -1

			for i := range resource.Connections {
				conn := &resource.Connections[i]
				if conn.Local {
					if firstLocalIdx == -1 {
						firstLocalIdx = i
					}
				} else {
					if firstRemoteIdx == -1 {
						firstRemoteIdx = i
					}
				}

				// Quick reachability test with a short timeout. If the overall ctx is
				// canceled, this will return quickly.
				if isReachable(ctx, conn.Address, conn.Port, 500*time.Millisecond) {
					if conn.Local {
						if reachableLocalIdx == -1 {
							reachableLocalIdx = i
						}
					} else {
						// Prefer a reachable remote address and stop searching further.
						reachableRemoteIdx = i
						break
					}
				}
			}

			chosenIdx := -1
			switch {
			case reachableRemoteIdx != -1:
				chosenIdx = reachableRemoteIdx
			case reachableLocalIdx != -1:
				chosenIdx = reachableLocalIdx
			case firstRemoteIdx != -1:
				chosenIdx = firstRemoteIdx
			default:
				chosenIdx = firstLocalIdx
			}

			if chosenIdx != -1 {
				chosenConn := &resource.Connections[chosenIdx]
				servers = append(servers, domain.PlexServer{
					Name:        resource.Name,
					Host:        chosenConn.Address,
					Port:        fmt.Sprintf("%d", chosenConn.Port),
					AccessToken: resource.AccessToken,
					Scheme:      chosenConn.Protocol,
				})
			}
		}
	}

	return servers, nil
}
