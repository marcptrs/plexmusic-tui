package http

import "plexmusic-tui/internal/domain"

// Factory implements domain.HTTPClientFactory using resty with retry support
type Factory struct{}

// NewFactory creates a new HTTP client factory
func NewFactory() *Factory {
	return &Factory{}
}

// GetClient returns an HTTP client appropriate for the given host.
// Clients are created with resty for built-in retry support and exponential backoff.
// Local/private hosts skip TLS verification.
func (f *Factory) GetClient(host string) domain.HTTPClient {
	if isLocalAddress(host) {
		return NewRestyClientWithTLS(true)
	}
	return NewRestyClient()
}
