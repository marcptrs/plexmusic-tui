//go:build darwin

package mediacontrol

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"plexmusic-tui/internal/domain"

	log "github.com/charmbracelet/log/v2"
)

const (
	daemonSocketPath = "/tmp/plexmusic-daemon.sock"
	reconnectDelay   = 2 * time.Second
	maxReconnects    = 5
)

// DaemonClient communicates with the plexmusic-daemon over Unix socket
type DaemonClient struct {
	conn         net.Conn
	mu           sync.Mutex
	connected    bool
	reconnecting bool
	ctx          context.Context
	cancel       context.CancelFunc
	commandChan  chan DaemonCommand
	wg           sync.WaitGroup
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
	log.Info("DaemonClient: Starting connection to daemon")

	if err := c.connect(); err != nil {
		log.Warn("DaemonClient: Initial connection failed", "error", err)
		// Continue anyway - we'll reconnect in the background
	}

	// Start message reader goroutine
	c.wg.Add(1)
	go c.messageLoop()

	return nil
}

// Stop disconnects from the daemon
func (c *DaemonClient) Stop() error {
	log.Info("DaemonClient: Stopping")
	c.cancel()

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.connected = false
	}
	c.mu.Unlock()

	c.wg.Wait()
	close(c.commandChan)
	return nil
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
	log.Info("DaemonClient: Connected to daemon")
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
			log.Info("DaemonClient: Reconnect attempt %d/%d", attempt+1, maxReconnects)
			if err := c.connect(); err != nil {
				log.Warn("DaemonClient: Reconnect failed: %v", err)
				continue
			}
			log.Info("DaemonClient: Reconnected successfully")
			return
		}
	}

	log.Error("DaemonClient: Failed to reconnect after %d attempts", maxReconnects)
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

				log.Warn("DaemonClient: Read error: %v", err)
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
	log.Debug("DaemonClient: Received command: %s", msg.Type)

	cmd := DaemonCommand{
		Type: msg.Type,
		Data: msg.Data,
	}

	select {
	case c.commandChan <- cmd:
	case <-c.ctx.Done():
	default:
		log.Warn("DaemonClient: Command channel full, dropping command: %s", msg.Type)
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

// SendPlaybackStarted sends a playback started event
func (c *DaemonClient) SendPlaybackStarted(track *domain.Track) error {
	msg := &daemonMessage{
		Type: "playback.started",
		Data: map[string]interface{}{
			"title":    track.Title,
			"artist":   track.Artist,
			"album":    track.Album,
			"duration": track.Duration,
		},
	}

	if err := c.sendMessage(msg); err != nil {
		log.Debug("DaemonClient: Failed to send playback.started: %v", err)
		return err
	}

	log.Debug("DaemonClient: Sent playback.started for '%s'", track.Title)
	return nil
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
		log.Debug("DaemonClient: Failed to send playback.paused: %v", err)
		return err
	}

	log.Debug("DaemonClient: Sent playback.paused")
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
		log.Debug("DaemonClient: Failed to send playback.resumed: %v", err)
		return err
	}

	log.Debug("DaemonClient: Sent playback.resumed")
	return nil
}

// SendPlaybackStopped sends a playback stopped event
func (c *DaemonClient) SendPlaybackStopped() error {
	msg := &daemonMessage{
		Type: "playback.stopped",
	}

	if err := c.sendMessage(msg); err != nil {
		log.Debug("DaemonClient: Failed to send playback.stopped: %v", err)
		return err
	}

	log.Debug("DaemonClient: Sent playback.stopped")
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
		log.Debug("DaemonClient: Failed to send playback.artwork: %v", err)
		return err
	}

	log.Debug("DaemonClient: Sent playback.artwork (%d bytes)", len(pngData))
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
