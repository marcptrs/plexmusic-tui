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

// provideMediaControlWrapper returns nil on unsupported platforms
func provideMediaControlWrapper(
	playbackService *service.PlaybackService,
	orchestrator *tui.Orchestrator,
	coordinator *app.Coordinator,
) *MediaControlWrapper {
	return nil
}
