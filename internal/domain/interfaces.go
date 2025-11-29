package domain

import (
	"context"
	"image"
	"net/http"
)

// HTTPClient defines the interface for making HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPClientFactory defines the interface for creating HTTP clients based on host.
type HTTPClientFactory interface {
	GetClient(host string) HTTPClient
}

// AuthGateway defines the interface for Plex authentication operations.
type AuthGateway interface {
	AuthenticateUser(ctx context.Context, username, password string) (string, error)
	FetchServers(ctx context.Context, token string) ([]PlexServer, error)
}

// ImageRenderer defines the interface for rendering images to the terminal.
type ImageRenderer interface {
	Render(img image.Image, width, height int) string
	SetDebug(enabled bool)
	SetProtocol(p Protocol)
	ClearHashCache()
}
