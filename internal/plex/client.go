package plex

import (
	"crypto/tls"
	"net"
	"net/http"
)

// ClientConfig holds configuration for Plex API client
type ClientConfig struct {
	Token      string
	ServerHost string
	ServerPort string
	Scheme     string // "http" or "https"
}

// HTTPClient wraps HTTP client creation with TLS handling
type HTTPClient struct {
	config *ClientConfig
}

// NewHTTPClient creates a new Plex HTTP client
func NewHTTPClient(config *ClientConfig) *HTTPClient {
	return &HTTPClient{config: config}
}

// GetClient returns an HTTP client with appropriate TLS settings
// For local/private IPs, it skips TLS verification (self-signed certs)
// For remote/public IPs, it uses proper TLS verification
func (c *HTTPClient) GetClient() *http.Client {
	if isLocalAddress(c.config.ServerHost) {
		// Local server - skip TLS verification for self-signed certificates
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	// Remote server - use proper TLS verification
	return &http.Client{}
}

// isLocalAddress checks if a host is a local/private address
func isLocalAddress(host string) bool {
	// Check for localhost
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Parse the IP address
	ip := net.ParseIP(host)
	if ip == nil {
		// If not an IP, it's a hostname - assume remote (use proper TLS)
		return false
	}

	// Check for private IP ranges
	// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, and link-local addresses
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
