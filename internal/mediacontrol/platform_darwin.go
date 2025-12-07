//go:build darwin

package mediacontrol

import (
	"context"
	"image"
	"sync"
	"time"

	"plexmusic-tui/internal/mediacontrol/internal/mpnow"

	log "github.com/charmbracelet/log/v2"
)

// darwinController implements MediaController for macOS
type darwinController struct {
	bridge  *mpnow.Bridge
	handler CommandHandler
	mu      sync.RWMutex
	started bool
}

// newPlatformController creates a new macOS media controller
func newPlatformController() (MediaController, error) {
	log.Info("Creating macOS media controller")
	bridge := mpnow.NewBridge()
	if bridge == nil {
		log.Error("Failed to create macOS media control bridge")
		return nil, ErrUnsupportedPlatform
	}

	log.Info("macOS media controller created successfully")
	return &darwinController{
		bridge: bridge,
	}, nil
}

// Start initializes and starts the media controller
func (c *darwinController) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	c.started = true

	// The bridge is already initialized and command handlers are set up
	// We just need to monitor the context for shutdown
	go func() {
		<-ctx.Done()
		c.Stop()
	}()

	return nil
}

// Stop shuts down the media controller
func (c *darwinController) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	c.started = false

	if c.bridge != nil {
		c.bridge.Destroy()
		c.bridge = nil
	}

	return nil
}

// UpdateMetadata updates the currently playing track metadata
func (c *darwinController) UpdateMetadata(metadata Metadata) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.bridge == nil {
		log.Error("Bridge is nil in UpdateMetadata!")
		return ErrUnsupportedPlatform
	}

	log.Debug("Updating media control metadata",
		"title", metadata.Title,
		"artist", metadata.Artist,
		"album", metadata.Album)

	log.Info("Calling bridge.UpdateMetadata")
	c.bridge.UpdateMetadata(
		metadata.Title,
		metadata.Artist,
		metadata.Album,
		metadata.Duration,
	)
	log.Info("bridge.UpdateMetadata returned")

	return nil
}

// UpdatePlaybackState updates the playback state
func (c *darwinController) UpdatePlaybackState(state PlaybackState) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.bridge == nil {
		return ErrUnsupportedPlatform
	}

	// Convert PlaybackState to bridge state
	var bridgeState int
	switch state {
	case StateStopped:
		bridgeState = 0
	case StatePlaying:
		bridgeState = 1
	case StatePaused:
		bridgeState = 2
	default:
		bridgeState = 0
	}

	c.bridge.UpdatePlaybackState(bridgeState)
	return nil
}

// UpdatePosition updates the playback position and duration
func (c *darwinController) UpdatePosition(position, duration time.Duration) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.bridge == nil {
		return ErrUnsupportedPlatform
	}

	c.bridge.UpdatePosition(position, duration)
	return nil
}

// SetArtwork sets the album artwork for the currently playing track
func (c *darwinController) SetArtwork(img image.Image) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.bridge == nil {
		return ErrUnsupportedPlatform
	}

	return c.bridge.SetArtwork(img)
}

// SetArtworkFromURL sets the album artwork from a URL
// On macOS, this downloads the image and uses SetArtwork
func (c *darwinController) SetArtworkFromURL(url string) error {
	// macOS doesn't have native URL-based artwork loading like Windows SMTC
	// The caller should download the image and use SetArtwork instead
	// Return nil to indicate "not an error, just not supported"
	return nil
}

// SetCommandHandler sets the handler for OS media commands
func (c *darwinController) SetCommandHandler(handler CommandHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Info("darwinController.SetCommandHandler called")
	c.handler = handler

	if c.bridge == nil {
		log.Error("Bridge is nil in SetCommandHandler!")
		return ErrUnsupportedPlatform
	}

	log.Info("Calling bridge.SetCommandHandler")
	c.bridge.SetCommandHandler(handler)
	log.Info("bridge.SetCommandHandler returned")

	return nil
}

// SupportsFeature returns true if the platform supports the given feature
func (c *darwinController) SupportsFeature(feature Feature) bool {
	switch feature {
	case FeatureMediaKeys,
		FeatureNowPlaying,
		FeatureArtwork,
		FeaturePositionSync,
		FeatureLockScreen:
		return true
	default:
		return false
	}
}
