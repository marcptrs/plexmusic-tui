package mediacontrol

import (
	"context"
	"errors"
	"image"
	"time"
)

// ErrUnsupportedPlatform is returned when media control is not supported on the current platform
var ErrUnsupportedPlatform = errors.New("media control not supported on this platform")

// MediaController provides OS-level media control integration
type MediaController interface {
	// Start initializes and starts the media controller
	Start(ctx context.Context) error

	// Stop shuts down the media controller
	Stop() error

	// UpdateMetadata updates the currently playing track metadata
	UpdateMetadata(metadata Metadata) error

	// UpdatePlaybackState updates the playback state (playing, paused, stopped)
	UpdatePlaybackState(state PlaybackState) error

	// UpdatePosition updates the playback position and duration
	UpdatePosition(position, duration time.Duration) error

	// SetArtwork sets the album artwork for the currently playing track
	SetArtwork(img image.Image) error

	// SetArtworkFromURL sets the album artwork from a URL
	// This is more efficient on some platforms (e.g., Windows) that can load
	// images directly from URLs without requiring the caller to download first
	SetArtworkFromURL(url string) error

	// SetCommandHandler sets the handler for OS media commands
	SetCommandHandler(handler CommandHandler) error

	// SupportsFeature returns true if the platform supports the given feature
	SupportsFeature(feature Feature) bool
}

// CommandHandler handles media control commands from the OS
type CommandHandler interface {
	// HandlePlay is called when the OS requests playback to start
	HandlePlay()

	// HandlePause is called when the OS requests playback to pause
	HandlePause()

	// HandleTogglePlayPause is called when the OS requests play/pause toggle
	HandleTogglePlayPause()

	// HandleStop is called when the OS requests playback to stop
	HandleStop()

	// HandleNext is called when the OS requests next track
	HandleNext()

	// HandlePrevious is called when the OS requests previous track
	HandlePrevious()

	// HandleSeek is called when the OS requests seeking to a position
	HandleSeek(position time.Duration)
}

// Feature represents a platform capability
type Feature int

const (
	// FeatureMediaKeys indicates support for global media key events
	FeatureMediaKeys Feature = iota

	// FeatureNowPlaying indicates support for Now Playing UI
	FeatureNowPlaying

	// FeatureArtwork indicates support for album artwork display
	FeatureArtwork

	// FeaturePositionSync indicates support for position updates and scrubbing
	FeaturePositionSync

	// FeatureLockScreen indicates support for lock screen controls
	FeatureLockScreen
)

// New creates a new MediaController for the current platform
// Returns ErrUnsupportedPlatform if media control is not available
func New() (MediaController, error) {
	return newPlatformController()
}
