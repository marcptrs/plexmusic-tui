//go:build !darwin

package bootstrap

import (
	"context"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
)

// MediaControlWrapper stub for non-macOS platforms
type MediaControlWrapper struct{}

// Start is a no-op on unsupported platforms
func (w *MediaControlWrapper) Start(ctx context.Context) error {
	return nil
}

// InProcessMediaControl stub for non-macOS platforms
type InProcessMediaControl struct{}

// Start is a no-op on unsupported platforms
func (w *InProcessMediaControl) Start(ctx context.Context) error {
	return nil
}

// NewInProcessMediaControl returns nil on unsupported platforms
func NewInProcessMediaControl(
	playbackService *service.PlaybackService,
	orchestrator *tui.Orchestrator,
	coordinator *app.Coordinator,
) (*InProcessMediaControl, error) {
	return nil, nil
}

// provideMediaControlWrapper returns nil on unsupported platforms
func provideMediaControlWrapper(
	playbackService *service.PlaybackService,
	orchestrator *tui.Orchestrator,
	coordinator *app.Coordinator,
) *MediaControlWrapper {
	return nil
}
