package http

import "plexmusic-tui/internal/domain"

// Factory implements domain.HTTPClientFactory
type Factory struct{}

// NewFactory creates a new HTTP client factory
func NewFactory() *Factory {
	return &Factory{}
}

// GetClient returns an HTTP client appropriate for the given host
func (f *Factory) GetClient(host string) domain.HTTPClient {
	return GetForHost(host)
}
