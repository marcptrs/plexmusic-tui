//go:build linux

package mediacontrol

import (
	"context"
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const (
	mprisPath       = "/org/mpris/MediaPlayer2"
	mprisBusPrefix  = "org.mpris.MediaPlayer2"
	mprisInterface  = "org.mpris.MediaPlayer2"
	playerInterface = "org.mpris.MediaPlayer2.Player"
)

// linuxController implements MediaController for Linux using MPRIS
type linuxController struct {
	conn    *dbus.Conn
	handler CommandHandler
	mu      sync.RWMutex
	started bool

	// MPRIS property management
	props *prop.Properties

	// Current state for MPRIS properties
	metadata   map[string]dbus.Variant
	artworkURL string
	state      PlaybackState
	position   time.Duration
	duration   time.Duration
	canGoNext  bool
	canGoPrev  bool
}

// newPlatformController creates a new Linux MPRIS media controller
func newPlatformController() (MediaController, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	return &linuxController{
		conn:     conn,
		metadata: make(map[string]dbus.Variant),
		state:    StateStopped,
	}, nil
}

// Start initializes and starts the media controller
func (c *linuxController) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	// Request bus name
	busName := fmt.Sprintf("%s.plexmusic", mprisBusPrefix)
	reply, err := c.conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("failed to request bus name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name already taken")
	}

	// Export MPRIS interfaces
	if err := c.conn.Export(c, dbus.ObjectPath(mprisPath), mprisInterface); err != nil {
		return fmt.Errorf("failed to export root interface: %w", err)
	}
	if err := c.conn.Export(c, dbus.ObjectPath(mprisPath), playerInterface); err != nil {
		return fmt.Errorf("failed to export player interface: %w", err)
	}

	// Set up properties
	propsSpec := map[string]map[string]*prop.Prop{
		mprisInterface: {
			"Identity":            {Value: "Plex Music TUI", Writable: false, Emit: prop.EmitFalse},
			"CanQuit":             {Value: false, Writable: false, Emit: prop.EmitFalse},
			"CanRaise":            {Value: false, Writable: false, Emit: prop.EmitFalse},
			"HasTrackList":        {Value: false, Writable: false, Emit: prop.EmitFalse},
			"SupportedUriSchemes": {Value: []string{}, Writable: false, Emit: prop.EmitFalse},
			"SupportedMimeTypes":  {Value: []string{}, Writable: false, Emit: prop.EmitFalse},
		},
		playerInterface: {
			"PlaybackStatus": {Value: "Stopped", Writable: false, Emit: prop.EmitTrue},
			"Rate":           {Value: 1.0, Writable: false, Emit: prop.EmitFalse},
			"Metadata":       {Value: c.metadata, Writable: false, Emit: prop.EmitTrue},
			"Volume":         {Value: 1.0, Writable: false, Emit: prop.EmitFalse},
			"Position":       {Value: int64(0), Writable: false, Emit: prop.EmitInvalidates},
			"MinimumRate":    {Value: 1.0, Writable: false, Emit: prop.EmitFalse},
			"MaximumRate":    {Value: 1.0, Writable: false, Emit: prop.EmitFalse},
			"CanGoNext":      {Value: false, Writable: false, Emit: prop.EmitTrue},
			"CanGoPrevious":  {Value: false, Writable: false, Emit: prop.EmitTrue},
			"CanPlay":        {Value: true, Writable: false, Emit: prop.EmitFalse},
			"CanPause":       {Value: true, Writable: false, Emit: prop.EmitFalse},
			"CanSeek":        {Value: true, Writable: false, Emit: prop.EmitFalse},
			"CanControl":     {Value: true, Writable: false, Emit: prop.EmitFalse},
		},
	}

	props, err := prop.Export(c.conn, dbus.ObjectPath(mprisPath), propsSpec)
	if err != nil {
		return fmt.Errorf("failed to export properties: %w", err)
	}
	c.props = props

	c.started = true

	// Monitor context for shutdown
	go func() {
		<-ctx.Done()
		c.Stop()
	}()

	return nil
}

// Stop shuts down the media controller
func (c *linuxController) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	// Release bus name
	busName := fmt.Sprintf("%s.plexmusic", mprisBusPrefix)
	if _, err := c.conn.ReleaseName(busName); err != nil {
		// TODO: Add logging
	}

	if err := c.conn.Close(); err != nil {
		// TODO: Add logging
	}

	c.started = false
	return nil
}

// UpdateMetadata updates the currently playing track metadata
func (c *linuxController) UpdateMetadata(metadata Metadata) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	// Build MPRIS metadata
	mprisMetadata := make(map[string]dbus.Variant)

	// Track ID (required)
	mprisMetadata["mpris:trackid"] = dbus.MakeVariant(dbus.ObjectPath(fmt.Sprintf("%s/Track/1", mprisPath)))

	// Duration in microseconds
	if metadata.Duration > 0 {
		mprisMetadata["mpris:length"] = dbus.MakeVariant(metadata.Duration.Microseconds())
	}

	// Artwork URL
	if c.artworkURL != "" {
		mprisMetadata["mpris:artUrl"] = dbus.MakeVariant(c.artworkURL)
	}

	// Track metadata
	if metadata.Title != "" {
		mprisMetadata["xesam:title"] = dbus.MakeVariant(metadata.Title)
	}
	if metadata.Artist != "" {
		mprisMetadata["xesam:artist"] = dbus.MakeVariant([]string{metadata.Artist})
	}
	if metadata.Album != "" {
		mprisMetadata["xesam:album"] = dbus.MakeVariant(metadata.Album)
	}
	if metadata.AlbumArtist != "" {
		mprisMetadata["xesam:albumArtist"] = dbus.MakeVariant([]string{metadata.AlbumArtist})
	}
	if metadata.TrackNumber > 0 {
		mprisMetadata["xesam:trackNumber"] = dbus.MakeVariant(int32(metadata.TrackNumber))
	}

	c.metadata = mprisMetadata
	c.duration = metadata.Duration

	// Update property
	if c.props != nil {
		c.props.SetMust(playerInterface, "Metadata", mprisMetadata)
	}

	return nil
}

// UpdatePlaybackState updates the playback state
func (c *linuxController) UpdatePlaybackState(state PlaybackState) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	c.state = state

	var mprisStatus string
	switch state {
	case StatePlaying:
		mprisStatus = "Playing"
	case StatePaused:
		mprisStatus = "Paused"
	case StateStopped:
		mprisStatus = "Stopped"
	default:
		mprisStatus = "Stopped"
	}

	if c.props != nil {
		c.props.SetMust(playerInterface, "PlaybackStatus", mprisStatus)
	}

	return nil
}

// UpdatePosition updates the playback position and duration
func (c *linuxController) UpdatePosition(position, duration time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	c.position = position
	c.duration = duration

	// Update Position property (in microseconds)
	if c.props != nil {
		c.props.SetMust(playerInterface, "Position", position.Microseconds())
	}

	return nil
}

// SetArtwork sets the album artwork for the currently playing track
func (c *linuxController) SetArtwork(img image.Image) error {
	// MPRIS on Linux uses URLs for artwork, not direct images
	// The caller should use SetArtworkFromURL instead
	return nil
}

// SetArtworkFromURL sets the album artwork from a URL
func (c *linuxController) SetArtworkFromURL(url string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	c.artworkURL = url

	// Update metadata with artwork URL
	if len(c.metadata) > 0 {
		c.metadata["mpris:artUrl"] = dbus.MakeVariant(url)
		if c.props != nil {
			c.props.SetMust(playerInterface, "Metadata", c.metadata)
		}
	}

	return nil
}

// SetCommandHandler sets the handler for OS media commands
func (c *linuxController) SetCommandHandler(handler CommandHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.handler = handler

	return nil
}

// SetCanGoNext updates whether next track is available
func (c *linuxController) SetCanGoNext(can bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.canGoNext = can
	if c.props != nil {
		c.props.SetMust(playerInterface, "CanGoNext", can)
	}
}

// SetCanGoPrevious updates whether previous track is available
func (c *linuxController) SetCanGoPrevious(can bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.canGoPrev = can
	if c.props != nil {
		c.props.SetMust(playerInterface, "CanGoPrevious", can)
	}
}

// SupportsFeature returns true if the platform supports the given feature
func (c *linuxController) SupportsFeature(feature Feature) bool {
	switch feature {
	case FeatureMediaKeys,
		FeatureNowPlaying,
		FeatureArtwork,
		FeaturePositionSync:
		return true
	case FeatureLockScreen:
		return false // Not applicable on Linux
	default:
		return false
	}
}

// MPRIS org.mpris.MediaPlayer2 methods

func (c *linuxController) Raise() *dbus.Error {
	// Not supported - TUI application
	return nil
}

func (c *linuxController) Quit() *dbus.Error {
	// Not supported
	return nil
}

// MPRIS org.mpris.MediaPlayer2.Player methods

func (c *linuxController) Play() *dbus.Error {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		handler.HandlePlay()
	}
	return nil
}

func (c *linuxController) Pause() *dbus.Error {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		handler.HandlePause()
	}
	return nil
}

func (c *linuxController) PlayPause() *dbus.Error {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		handler.HandleTogglePlayPause()
	}
	return nil
}

func (c *linuxController) Next() *dbus.Error {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		handler.HandleNext()
	}
	return nil
}

func (c *linuxController) Previous() *dbus.Error {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		handler.HandlePrevious()
	}
	return nil
}

func (c *linuxController) Seek(offset int64) *dbus.Error {
	c.mu.RLock()
	handler := c.handler
	currentPos := c.position
	c.mu.RUnlock()

	if handler != nil {
		// Offset is in microseconds, convert to duration
		newPos := currentPos + time.Duration(offset)*time.Microsecond
		if newPos < 0 {
			newPos = 0
		}
		handler.HandleSeek(newPos)
	}
	return nil
}

func (c *linuxController) SetPosition(trackID dbus.ObjectPath, position int64) *dbus.Error {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()

	if handler != nil {
		// Position is in microseconds, convert to duration
		pos := time.Duration(position) * time.Microsecond
		handler.HandleSeek(pos)
	}
	return nil
}

func (c *linuxController) OpenUri(uri string) *dbus.Error {
	// Not supported
	return dbus.NewError("org.mpris.MediaPlayer2.NotSupported", nil)
}

// Compile-time check that we implement the interface
var _ MediaController = (*linuxController)(nil)
