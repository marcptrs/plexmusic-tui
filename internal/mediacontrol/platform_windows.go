//go:build windows

package mediacontrol

import (
	"context"
	"image"
	"sync"
	"time"

	log "github.com/charmbracelet/log/v2"
)

// TODO: Add Windows dependencies when testing on Windows:
// - github.com/go-ole/go-ole
// - github.com/saltosystems/winrt-go (or github.com/go-musicfox/winrt-go fork)
//
// Example imports for full implementation:
// "github.com/go-ole/go-ole"
// "github.com/saltosystems/winrt-go"
// "github.com/saltosystems/winrt-go/windows/foundation"
// "github.com/saltosystems/winrt-go/windows/media"
// "github.com/saltosystems/winrt-go/windows/media/playback"
// "github.com/saltosystems/winrt-go/windows/storage/streams"

// windowsController implements MediaController for Windows using SMTC
type windowsController struct {
	handler CommandHandler
	mu      sync.RWMutex
	started bool

	// TODO: Add these fields when winrt-go is available:
	// smtc   *media.SystemMediaTransportControls
	// player *playback.MediaPlayer

	// Current state tracking
	currentMetadata Metadata
	currentState    PlaybackState
	currentPosition time.Duration
}

// newPlatformController creates a new Windows media controller
func newPlatformController() (MediaController, error) {
	log.Info("Creating Windows media controller (SMTC)")

	// TODO: Initialize Windows Runtime and SMTC when dependencies are added:
	//
	// err := ole.RoInitialize(1)
	// if err != nil {
	//     return nil, fmt.Errorf("failed to initialize Windows Runtime: %w", err)
	// }
	//
	// player, err := playback.NewMediaPlayer()
	// if err != nil {
	//     return nil, fmt.Errorf("failed to create MediaPlayer: %w", err)
	// }
	//
	// smtc, err := player.GetSystemMediaTransportControls()
	// if err != nil {
	//     return nil, fmt.Errorf("failed to get SMTC: %w", err)
	// }

	return &windowsController{
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

	// TODO: Enable SMTC buttons when dependencies are added:
	//
	// c.smtc.SetIsEnabled(true)
	// c.smtc.SetIsPlayEnabled(true)
	// c.smtc.SetIsPauseEnabled(true)
	// c.smtc.SetIsNextEnabled(true)
	// c.smtc.SetIsPreviousEnabled(true)
	//
	// Register button press handler:
	// buttonPressedHandler := foundation.NewTypedEventHandler(...)
	// c.smtc.AddButtonPressed(buttonPressedHandler)

	c.started = true

	// Monitor context for shutdown
	go func() {
		<-ctx.Done()
		c.Stop()
	}()

	return nil
}

// Stop shuts down the media controller
func (c *windowsController) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	log.Info("Stopping Windows SMTC controller")

	// TODO: Release SMTC resources when dependencies are added:
	//
	// if c.smtc != nil {
	//     c.smtc.Release()
	// }
	// if c.player != nil {
	//     c.player.Release()
	// }

	c.started = false
	return nil
}

// UpdateMetadata updates the currently playing track metadata
func (c *windowsController) UpdateMetadata(metadata Metadata) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentMetadata = metadata

	log.Debug("Windows SMTC: Updating metadata",
		"title", metadata.Title,
		"artist", metadata.Artist,
		"album", metadata.Album)

	// TODO: Update SMTC display when dependencies are added:
	//
	// updater, err := c.smtc.GetDisplayUpdater()
	// if err != nil {
	//     return err
	// }
	// defer updater.Release()
	//
	// updater.SetType(media.MediaPlaybackTypeMusic)
	//
	// musicProps, err := updater.GetMusicProperties()
	// if err != nil {
	//     return err
	// }
	// defer musicProps.Release()
	//
	// musicProps.SetTitle(metadata.Title)
	// musicProps.SetArtist(metadata.Artist)
	// musicProps.SetAlbumTitle(metadata.Album)
	// if metadata.AlbumArtist != "" {
	//     musicProps.SetAlbumArtist(metadata.AlbumArtist)
	// }
	//
	// updater.Update()

	return nil
}

// UpdatePlaybackState updates the playback state
func (c *windowsController) UpdatePlaybackState(state PlaybackState) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentState = state

	log.Debug("Windows SMTC: Updating playback state", "state", state)

	// TODO: Update SMTC playback status when dependencies are added:
	//
	// var smtcState media.MediaPlaybackStatus
	// switch state {
	// case StateStopped:
	//     smtcState = media.MediaPlaybackStatusStopped
	// case StatePlaying:
	//     smtcState = media.MediaPlaybackStatusPlaying
	// case StatePaused:
	//     smtcState = media.MediaPlaybackStatusPaused
	// }
	// c.smtc.SetPlaybackStatus(smtcState)

	return nil
}

// UpdatePosition updates the playback position and duration
func (c *windowsController) UpdatePosition(position, duration time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentPosition = position

	// TODO: Update SMTC timeline properties when dependencies are added:
	//
	// const ticksPerMillisecond = 10000
	//
	// timelineProps, err := media.NewSystemMediaTransportControlsTimelineProperties()
	// if err != nil {
	//     return err
	// }
	// defer timelineProps.Release()
	//
	// timelineProps.SetStartTime(foundation.TimeSpan{Duration: 0})
	// timelineProps.SetEndTime(foundation.TimeSpan{Duration: duration.Milliseconds() * ticksPerMillisecond})
	// timelineProps.SetMinSeekTime(foundation.TimeSpan{Duration: 0})
	// timelineProps.SetMaxSeekTime(foundation.TimeSpan{Duration: duration.Milliseconds() * ticksPerMillisecond})
	// timelineProps.SetPosition(foundation.TimeSpan{Duration: position.Milliseconds() * ticksPerMillisecond})
	//
	// c.smtc.UpdateTimelineProperties(timelineProps)

	return nil
}

// SetArtwork sets the album artwork for the currently playing track
func (c *windowsController) SetArtwork(img image.Image) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if img == nil {
		return nil
	}

	log.Debug("Windows SMTC: Setting artwork")

	// TODO: Set artwork when dependencies are added:
	//
	// For URL-based artwork (simpler):
	// imgUri, err := foundation.UriCreateUri(artworkURL)
	// if err != nil {
	//     return err
	// }
	// defer imgUri.Release()
	//
	// stream, err := streams.RandomAccessStreamReferenceCreateFromUri(imgUri)
	// if err != nil {
	//     return err
	// }
	// defer stream.Release()
	//
	// updater, err := c.smtc.GetDisplayUpdater()
	// if err != nil {
	//     return err
	// }
	// defer updater.Release()
	//
	// updater.SetThumbnail(stream)
	// updater.Update()

	return nil
}

// SetCommandHandler sets the handler for OS media commands
func (c *windowsController) SetCommandHandler(handler CommandHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Info("Windows SMTC: Setting command handler")
	c.handler = handler

	// TODO: The button press handler registered in Start() will call
	// the appropriate handler methods when buttons are pressed:
	//
	// In the button pressed callback:
	// switch button {
	// case media.SystemMediaTransportControlsButtonPlay:
	//     c.handler.HandlePlay()
	// case media.SystemMediaTransportControlsButtonPause:
	//     c.handler.HandlePause()
	// case media.SystemMediaTransportControlsButtonNext:
	//     c.handler.HandleNext()
	// case media.SystemMediaTransportControlsButtonPrevious:
	//     c.handler.HandlePrevious()
	// }

	return nil
}

// SupportsFeature returns true if the platform supports the given feature
func (c *windowsController) SupportsFeature(feature Feature) bool {
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
