//go:build darwin

package mediacontrol

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"plexmusic-tui/internal/domain"
)

const (
	daemonSocketPath = "/tmp/plexmusic-daemon.sock"
	daemonPidFile    = "/tmp/plexmusic-daemon.pid"
	reconnectDelay   = 2 * time.Second
	maxReconnects    = 5
	daemonStartWait  = 2 * time.Second
)

// DaemonClient communicates with the plexmusic-daemon over Unix socket
type DaemonClient struct {
	conn          net.Conn
	mu            sync.Mutex
	connected     bool
	reconnecting  bool
	ctx           context.Context
	cancel        context.CancelFunc
	commandChan   chan DaemonCommand
	wg            sync.WaitGroup
	daemonStarted bool // true if we started the daemon ourselves
}

// DaemonCommand represents a command from the daemon
type DaemonCommand struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data,omitempty"`
}

// daemonMessage represents the JSON message structure
type daemonMessage struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data,omitempty"`
}

// NewDaemonClient creates a new daemon client
func NewDaemonClient() *DaemonClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &DaemonClient{
		ctx:         ctx,
		cancel:      cancel,
		commandChan: make(chan DaemonCommand, 10),
	}
}

// Start begins the daemon client connection and message loop
func (c *DaemonClient) Start(ctx context.Context) error {
	// Clean up any orphaned daemon from a previous crash
	c.cleanupOrphanedDaemon()

	// Try to connect first - daemon may already be running
	if err := c.connect(); err != nil {
		if launchErr := c.launchDaemon(); launchErr != nil {
			// TODO: Add logging
		} else {
			// Wait for daemon to start and try connecting again
			time.Sleep(daemonStartWait)
			_ = c.connect()
		}
	}

	// Start message reader goroutine
	c.wg.Add(1)
	go c.messageLoop()

	return nil
}

// Stop disconnects from the daemon and optionally stops it
func (c *DaemonClient) Stop() error {
	// TODO: Add logging
	c.cancel()

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.connected = false
	}
	daemonStarted := c.daemonStarted
	c.mu.Unlock()

	c.wg.Wait()
	close(c.commandChan)

	// If we started the daemon, stop it
	if daemonStarted {
		c.stopDaemon()
	}

	return nil
}

// getDaemonPath returns the path to the daemon app bundle
func getDaemonPath() string {
	// Check standard install location
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, "Library", "Application Support", "PlexMusic", "PlexMusicDaemon.app")
}

// cleanupOrphanedDaemon checks if a previous TUI instance crashed and left the daemon running
func (c *DaemonClient) cleanupOrphanedDaemon() {
	// Check if PID file exists
	data, err := os.ReadFile(daemonPidFile)
	if err != nil {
		return // No PID file, nothing to clean up
	}

	// PID file exists - a previous instance started the daemon

	// Stop the daemon and remove the PID file
	c.stopDaemon()
	os.Remove(daemonPidFile)

	// Give it a moment to fully stop
	time.Sleep(500 * time.Millisecond)

	_ = data // silence unused warning
}

// writePidFile writes a marker file indicating we started the daemon
func writePidFile() error {
	return os.WriteFile(daemonPidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
}

// removePidFile removes the marker file
func removePidFile() {
	os.Remove(daemonPidFile)
}

// launchDaemon starts the daemon process
func (c *DaemonClient) launchDaemon() error {
	daemonPath := getDaemonPath()
	if daemonPath == "" {
		return fmt.Errorf("cannot determine daemon path")
	}

	// Check if daemon exists
	if _, err := os.Stat(daemonPath); os.IsNotExist(err) {
		return fmt.Errorf("daemon not installed at %s", daemonPath)
	}

	// Use 'open' command to launch the app bundle
	cmd := exec.CommandContext(context.Background(), "open", "-a", daemonPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Write PID file so we can detect crashes
	_ = writePidFile()

	c.mu.Lock()
	c.daemonStarted = true
	c.mu.Unlock()

	return nil
}

// stopDaemon terminates the daemon process
func (c *DaemonClient) stopDaemon() {
	// Remove PID file first
	removePidFile()

	// Use osascript to quit the app gracefully
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "osascript", "-e", `tell application "PlexMusicDaemon" to quit`)
	if err := cmd.Run(); err != nil {
		// If graceful quit fails, try pkill as fallback (but only our daemon)
		exec.CommandContext(ctx, "pkill", "-f", "PlexMusicDaemon.app/Contents/MacOS/PlexMusicDaemon").Run()
	}
}

// Commands returns a channel that receives commands from the daemon
func (c *DaemonClient) Commands() <-chan DaemonCommand {
	return c.commandChan
}

// connect establishes connection to the daemon
func (c *DaemonClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(context.Background(), "unix", daemonSocketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon socket: %w", err)
	}

	c.conn = conn
	c.connected = true
	return nil
}

// reconnect attempts to reconnect to the daemon
func (c *DaemonClient) reconnect() {
	c.mu.Lock()
	if c.reconnecting {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.reconnecting = false
		c.mu.Unlock()
	}()

	for attempt := 0; attempt < maxReconnects; attempt++ {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(reconnectDelay):
			if err := c.connect(); err != nil {
				// TODO: Add logging
				continue
			}
			return
		}
	}
}

// messageLoop reads messages from the daemon
func (c *DaemonClient) messageLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.mu.Lock()
			conn := c.conn
			connected := c.connected
			c.mu.Unlock()

			if !connected || conn == nil {
				time.Sleep(reconnectDelay)
				c.reconnect()
				continue
			}

			// Set read deadline
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))

			// Read message
			msg, err := c.readMessage(conn)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}

				c.mu.Lock()
				c.connected = false
				if c.conn != nil {
					c.conn.Close()
					c.conn = nil
				}
				c.mu.Unlock()

				go c.reconnect()
				continue
			}

			// Handle message
			c.handleMessage(msg)
		}
	}
}

// readMessage reads a length-prefixed JSON message
func (c *DaemonClient) readMessage(conn net.Conn) (*daemonMessage, error) {
	// Read length prefix (4 bytes, big-endian)
	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	// Read JSON payload - use io.ReadFull to ensure we read all bytes
	buf := make([]byte, length)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}

	var msg daemonMessage
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// handleMessage processes a message from the daemon
func (c *DaemonClient) handleMessage(msg *daemonMessage) {
	cmd := DaemonCommand{
		Type: msg.Type,
		Data: msg.Data,
	}

	select {
	case c.commandChan <- cmd:
	case <-c.ctx.Done():
	default:
		// TODO: Add logging
	}
}

// sendMessage sends a length-prefixed JSON message to the daemon
func (c *DaemonClient) sendMessage(msg *daemonMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected to daemon")
	}

	// Marshal to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write length prefix
	length := uint32(len(data))
	if err := binary.Write(c.conn, binary.BigEndian, length); err != nil {
		c.connected = false
		return fmt.Errorf("failed to write length: %w", err)
	}

	// Write JSON payload
	if _, err := c.conn.Write(data); err != nil {
		c.connected = false
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// SendPlaybackStarted sends a playback started event with optional artwork
func (c *DaemonClient) SendPlaybackStarted(track *domain.Track, artwork image.Image) error {
	data := map[string]interface{}{
		"title":    track.Title,
		"artist":   track.Artist,
		"album":    track.Album,
		"duration": track.Duration,
	}

	if artwork != nil {
		artworkData, err := encodeImageToJPEG(artwork)
		if err != nil {
			// TODO: Add logging
		} else {
			data["artwork"] = base64.StdEncoding.EncodeToString(artworkData)
		}
	}

	msg := &daemonMessage{
		Type: "playback.started",
		Data: data,
	}

	if err := c.sendMessage(msg); err != nil {
		// TODO: Add logging
		return err
	}

	return nil
}

func encodeImageToJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SendPlaybackPaused sends a playback paused event
func (c *DaemonClient) SendPlaybackPaused(position, sampleRate int) error {
	msg := &daemonMessage{
		Type: "playback.paused",
		Data: map[string]interface{}{
			"position":   position,
			"sampleRate": sampleRate,
		},
	}

	if err := c.sendMessage(msg); err != nil {
		// TODO: Add logging
		return err
	}

	return nil
}

// SendPlaybackResumed sends a playback resumed event
func (c *DaemonClient) SendPlaybackResumed(position, sampleRate int) error {
	msg := &daemonMessage{
		Type: "playback.resumed",
		Data: map[string]interface{}{
			"position":   position,
			"sampleRate": sampleRate,
		},
	}

	if err := c.sendMessage(msg); err != nil {
		// TODO: Add logging
		return err
	}

	return nil
}

// SendPlaybackStopped sends a playback stopped event
func (c *DaemonClient) SendPlaybackStopped() error {
	msg := &daemonMessage{
		Type: "playback.stopped",
	}

	if err := c.sendMessage(msg); err != nil {
		// TODO: Add logging
		return err
	}

	return nil
}

// SendArtwork sends artwork data (PNG) to the daemon
func (c *DaemonClient) SendArtwork(pngData []byte) error {
	if len(pngData) == 0 {
		return nil
	}

	encoded := base64.StdEncoding.EncodeToString(pngData)
	msg := &daemonMessage{
		Type: "playback.artwork",
		Data: map[string]interface{}{
			"png_base64": encoded,
		},
	}

	if err := c.sendMessage(msg); err != nil {
		// TODO: Add logging
		return err
	}

	return nil
}

// SendArtworkImage sends album artwork as an image to the daemon
func (c *DaemonClient) SendArtworkImage(img image.Image) error {
	if img == nil {
		return nil
	}

	artworkData, err := encodeImageToJPEG(img)
	if err != nil {
		// TODO: Add logging
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(artworkData)
	msg := &daemonMessage{
		Type: "playback.artwork",
		Data: map[string]interface{}{
			"png_base64": encoded, // Field name kept for backward compatibility, but contains JPEG
		},
	}

	if err := c.sendMessage(msg); err != nil {
		// TODO: Add logging
		return err
	}

	return nil
}

// SendPosition sends playback position update to the daemon
func (c *DaemonClient) SendPosition(position, duration int, sampleRate int) error {
	msg := &daemonMessage{
		Type: "playback.position",
		Data: map[string]interface{}{
			"position":   position,
			"duration":   duration,
			"sampleRate": sampleRate,
		},
	}

	if err := c.sendMessage(msg); err != nil {
		// Don't log position errors - they happen frequently
		return err
	}

	return nil
}
