//go:build windows

package mediacontrol

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"sync"
	"time"
	"unsafe"

	log "github.com/charmbracelet/log/v2"
	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/media"
	"github.com/saltosystems/winrt-go/windows/media/playback"
	"github.com/saltosystems/winrt-go/windows/storage/streams"
)

// windowsController implements MediaController for Windows using SMTC
type windowsController struct {
	handler CommandHandler
	mu      sync.RWMutex
	started bool

	// WinRT objects
	player *playback.MediaPlayer
	smtc   *media.SystemMediaTransportControls

	// Event registration token for cleanup
	buttonPressedToken foundation.EventRegistrationToken

	// Current state tracking
	currentMetadata Metadata
	currentState    PlaybackState
	currentPosition time.Duration
	currentDuration time.Duration
}

// newPlatformController creates a new Windows media controller
func newPlatformController() (MediaController, error) {
	log.Info("Creating Windows media controller (SMTC)")

	// Initialize Windows Runtime
	if err := ole.RoInitialize(1); err != nil {
		// RoInitialize may return S_FALSE (1) if already initialized, which is fine
		if err.(*ole.OleError).Code() != 1 {
			log.Warn("Windows Runtime initialization warning", "error", err)
		}
	}

	// Create MediaPlayer instance
	player, err := playback.NewMediaPlayer()
	if err != nil {
		return nil, err
	}

	// Get SystemMediaTransportControls from the player
	smtc, err := player.GetSystemMediaTransportControls()
	if err != nil {
		player.Release()
		return nil, err
	}

	return &windowsController{
		player:       player,
		smtc:         smtc,
		currentState: StateStopped,
	}, nil
}

// Start initializes and starts the media controller
func (c *windowsController) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	log.Info("Starting Windows SMTC controller")

	// Enable SMTC
	if err := c.smtc.SetIsEnabled(true); err != nil {
		log.Warn("Failed to enable SMTC", "error", err)
	}

	// Enable control buttons
	if err := c.smtc.SetIsPlayEnabled(true); err != nil {
		log.Warn("Failed to enable play button", "error", err)
	}
	if err := c.smtc.SetIsPauseEnabled(true); err != nil {
		log.Warn("Failed to enable pause button", "error", err)
	}
	if err := c.smtc.SetIsNextEnabled(true); err != nil {
		log.Warn("Failed to enable next button", "error", err)
	}
	if err := c.smtc.SetIsPreviousEnabled(true); err != nil {
		log.Warn("Failed to enable previous button", "error", err)
	}
	if err := c.smtc.SetIsStopEnabled(true); err != nil {
		log.Warn("Failed to enable stop button", "error", err)
	}

	// Register button press handler
	handler := foundation.NewTypedEventHandler(
		ole.NewGUID(media.GUIDiSystemMediaTransportControls),
		c.handleButtonPressed,
	)

	token, err := c.smtc.AddButtonPressed(handler)
	if err != nil {
		log.Warn("Failed to register button handler", "error", err)
	} else {
		c.buttonPressedToken = token
	}

	c.started = true

	// Monitor context for shutdown
	go func() {
		<-ctx.Done()
		c.Stop()
	}()

	return nil
}

// handleButtonPressed handles SMTC button press events
func (c *windowsController) handleButtonPressed(
	instance *foundation.TypedEventHandler,
	sender unsafe.Pointer,
	args unsafe.Pointer,
) {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler == nil {
		return
	}

	// Cast args to SystemMediaTransportControlsButtonPressedEventArgs
	eventArgs := (*media.SystemMediaTransportControlsButtonPressedEventArgs)(args)
	button, err := eventArgs.GetButton()
	if err != nil {
		log.Warn("Failed to get pressed button", "error", err)
		return
	}

	log.Debug("SMTC button pressed", "button", button)

	switch button {
	case media.SystemMediaTransportControlsButtonPlay:
		handler.HandlePlay()
	case media.SystemMediaTransportControlsButtonPause:
		handler.HandlePause()
	case media.SystemMediaTransportControlsButtonStop:
		handler.HandleStop()
	case media.SystemMediaTransportControlsButtonNext:
		handler.HandleNext()
	case media.SystemMediaTransportControlsButtonPrevious:
		handler.HandlePrevious()
	}
}

// Stop shuts down the media controller
func (c *windowsController) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	log.Info("Stopping Windows SMTC controller")

	// Remove button handler
	if c.buttonPressedToken.Value != 0 {
		if err := c.smtc.RemoveButtonPressed(c.buttonPressedToken); err != nil {
			log.Warn("Failed to remove button handler", "error", err)
		}
	}

	// Disable SMTC
	if c.smtc != nil {
		c.smtc.SetIsEnabled(false)
		c.smtc.Release()
		c.smtc = nil
	}

	// Release player
	if c.player != nil {
		c.player.Release()
		c.player = nil
	}

	c.started = false
	return nil
}

// UpdateMetadata updates the currently playing track metadata
func (c *windowsController) UpdateMetadata(metadata Metadata) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentMetadata = metadata

	if c.smtc == nil {
		return nil
	}

	log.Debug("Windows SMTC: Updating metadata",
		"title", metadata.Title,
		"artist", metadata.Artist,
		"album", metadata.Album)

	// Get display updater
	updater, err := c.smtc.GetDisplayUpdater()
	if err != nil {
		return err
	}
	defer updater.Release()

	// Set type to music
	if err := updater.SetType(media.MediaPlaybackTypeMusic); err != nil {
		log.Warn("Failed to set media type", "error", err)
	}

	// Get music properties
	musicProps, err := updater.GetMusicProperties()
	if err != nil {
		return err
	}
	defer musicProps.Release()

	// Set metadata
	if err := musicProps.SetTitle(metadata.Title); err != nil {
		log.Warn("Failed to set title", "error", err)
	}
	if err := musicProps.SetArtist(metadata.Artist); err != nil {
		log.Warn("Failed to set artist", "error", err)
	}
	if err := musicProps.SetAlbumTitle(metadata.Album); err != nil {
		log.Warn("Failed to set album", "error", err)
	}
	if metadata.AlbumArtist != "" {
		if err := musicProps.SetAlbumArtist(metadata.AlbumArtist); err != nil {
			log.Warn("Failed to set album artist", "error", err)
		}
	}

	// Update the display
	if err := updater.Update(); err != nil {
		return err
	}

	return nil
}

// UpdatePlaybackState updates the playback state
func (c *windowsController) UpdatePlaybackState(state PlaybackState) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentState = state

	if c.smtc == nil {
		return nil
	}

	log.Debug("Windows SMTC: Updating playback state", "state", state)

	var smtcState media.MediaPlaybackStatus
	switch state {
	case StateStopped:
		smtcState = media.MediaPlaybackStatusStopped
	case StatePlaying:
		smtcState = media.MediaPlaybackStatusPlaying
	case StatePaused:
		smtcState = media.MediaPlaybackStatusPaused
	default:
		smtcState = media.MediaPlaybackStatusClosed
	}

	return c.smtc.SetPlaybackStatus(smtcState)
}

// UpdatePosition updates the playback position and duration
func (c *windowsController) UpdatePosition(position, duration time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentPosition = position
	c.currentDuration = duration

	if c.smtc == nil {
		return nil
	}

	// Create timeline properties
	timelineProps, err := media.NewSystemMediaTransportControlsTimelineProperties()
	if err != nil {
		return err
	}
	defer timelineProps.Release()

	// Windows uses 100-nanosecond intervals (ticks)
	// 1 millisecond = 10,000 ticks
	const ticksPerMs = 10000

	positionTicks := position.Milliseconds() * ticksPerMs
	durationTicks := duration.Milliseconds() * ticksPerMs

	// Set timeline properties
	if err := timelineProps.SetStartTime(foundation.TimeSpan{Duration: 0}); err != nil {
		log.Warn("Failed to set start time", "error", err)
	}
	if err := timelineProps.SetEndTime(foundation.TimeSpan{Duration: durationTicks}); err != nil {
		log.Warn("Failed to set end time", "error", err)
	}
	if err := timelineProps.SetMinSeekTime(foundation.TimeSpan{Duration: 0}); err != nil {
		log.Warn("Failed to set min seek time", "error", err)
	}
	if err := timelineProps.SetMaxSeekTime(foundation.TimeSpan{Duration: durationTicks}); err != nil {
		log.Warn("Failed to set max seek time", "error", err)
	}
	if err := timelineProps.SetPosition(foundation.TimeSpan{Duration: positionTicks}); err != nil {
		log.Warn("Failed to set position", "error", err)
	}

	// Update timeline
	return c.smtc.UpdateTimelineProperties(timelineProps)
}

// SetArtwork sets the album artwork for the currently playing track
func (c *windowsController) SetArtwork(img image.Image) error {
	c.mu.RLock()
	smtc := c.smtc
	c.mu.RUnlock()

	if img == nil || smtc == nil {
		return nil
	}

	log.Debug("Windows SMTC: Setting artwork")

	// For now, we'll skip artwork implementation as it requires
	// creating an InMemoryRandomAccessStream which is more complex.
	// The basic SMTC functionality will work without artwork.
	//
	// TODO: Implement artwork support using:
	// 1. Encode image to PNG bytes
	// 2. Create InMemoryRandomAccessStream
	// 3. Write bytes to stream
	// 4. Create RandomAccessStreamReference from stream
	// 5. Set thumbnail on display updater

	return nil
}

// SetArtworkFromURL sets artwork from a URL
func (c *windowsController) SetArtworkFromURL(url string) error {
	c.mu.RLock()
	smtc := c.smtc
	c.mu.RUnlock()

	if url == "" || smtc == nil {
		return nil
	}

	log.Debug("Windows SMTC: Setting artwork from URL", "url", url)

	// Create URI from URL string
	uri, err := foundation.UriCreateUri(url)
	if err != nil {
		return err
	}
	defer uri.Release()

	// Create stream reference from URI
	streamRef, err := streams.RandomAccessStreamReferenceCreateFromUri(uri)
	if err != nil {
		return err
	}
	defer streamRef.Release()

	// Get display updater
	updater, err := smtc.GetDisplayUpdater()
	if err != nil {
		return err
	}
	defer updater.Release()

	// Set thumbnail
	if err := updater.SetThumbnail(streamRef); err != nil {
		return err
	}

	// Update display
	return updater.Update()
}

// SetCommandHandler sets the handler for OS media commands
func (c *windowsController) SetCommandHandler(handler CommandHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Info("Windows SMTC: Setting command handler")
	c.handler = handler

	return nil
}

// SupportsFeature returns true if the platform supports the given feature
func (c *windowsController) SupportsFeature(feature Feature) bool {
	switch feature {
	case FeatureMediaKeys,
		FeatureNowPlaying,
		FeaturePositionSync,
		FeatureLockScreen:
		return true
	case FeatureArtwork:
		// Artwork from URL is supported, in-memory artwork not yet implemented
		return true
	default:
		return false
	}
}

// Compile-time check that we implement the interface
var _ MediaController = (*windowsController)(nil)

// imageToBytes converts an image to PNG bytes (for future artwork support)
func imageToBytes(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
