//go:build !darwin && !windows

package mediacontrol

import (
	"context"
	"image"
	"time"
)

// stubController is a no-op implementation for unsupported platforms
type stubController struct{}

// newPlatformController returns an error on unsupported platforms
func newPlatformController() (MediaController, error) {
	return nil, ErrUnsupportedPlatform
}

func (c *stubController) Start(ctx context.Context) error {
	return ErrUnsupportedPlatform
}

func (c *stubController) Stop() error {
	return ErrUnsupportedPlatform
}

func (c *stubController) UpdateMetadata(metadata Metadata) error {
	return ErrUnsupportedPlatform
}

func (c *stubController) UpdatePlaybackState(state PlaybackState) error {
	return ErrUnsupportedPlatform
}

func (c *stubController) UpdatePosition(position, duration time.Duration) error {
	return ErrUnsupportedPlatform
}

func (c *stubController) SetArtwork(img image.Image) error {
	return ErrUnsupportedPlatform
}

func (c *stubController) SetCommandHandler(handler CommandHandler) error {
	return ErrUnsupportedPlatform
}

func (c *stubController) SupportsFeature(feature Feature) bool {
	return false
}
