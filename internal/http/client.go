package http

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
)

// Client wraps http.Client with Plex-specific functionality.
// It handles TLS verification skipping for local servers and
// provides convenient methods for Plex API requests.
type Client struct {
	client *http.Client
}

// New creates a standard HTTP client suitable for remote Plex servers.
func New() *Client {
	return &Client{
		client: &http.Client{},
	}
}

// NewWithTLSHandling creates an HTTP client that intelligently handles
// TLS verification based on whether the host is local or remote.
// Local addresses (localhost, private IPs) skip certificate verification.
func NewWithTLSHandling() *Client {
	return &Client{
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: false,
				},
			},
		},
	}
}

// GetForHost returns an appropriate HTTP client for the given host.
// If the host is local/private, TLS verification is disabled.
// Otherwise, standard HTTPS verification is used.
func GetForHost(host string) *Client {
	if isLocalAddress(host) {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		return &Client{
			client: &http.Client{Transport: transport},
		}
	}
	return New()
}

// HTTPClient returns the underlying http.Client for direct use.
func (c *Client) HTTPClient() *http.Client {
	return c.client
}

// Do performs an HTTP request using the wrapped client.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

// Get performs a GET request.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.client.Do(req)
}

// isLocalAddress checks if a hostname/IP is local or private.
// Returns true for:
// - localhost, 127.0.0.1, ::1
// - Private IPv4 ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
// - Private IPv6 ranges (fc00::/7)
// - Link-local addresses (169.254.0.0/16, fe80::/10)
func isLocalAddress(hostPort string) bool {
	// Extract hostname (remove port if present)
	host := hostPort
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		// IPv4 with port
		host, _, _ = strings.Cut(host, ":")
	} else if strings.HasPrefix(host, "[") && strings.Contains(host, "]:") {
		// IPv6 with port
		idx := strings.LastIndex(host, "]:")
		if idx != -1 {
			host = host[1:idx] // Remove [ and ]:port
		}
	}

	// Check for localhost by name
	if host == "localhost" {
		return true
	}

	// Parse as IP
	ip := net.ParseIP(host)
	if ip == nil {
		// Not an IP address, assume it could be local if it's not a domain
		// Default to false for unknown hostnames
		return false
	}

	// Check various local/private ranges
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsUnspecified()
}
