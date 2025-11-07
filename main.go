package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/auth"
	termimg "plexmusic-tui/internal/image"
	"plexmusic-tui/internal/plex"
	"plexmusic-tui/internal/ui"
)

type sessionState int

const (
	loginView sessionState = iota
	authenticatingView
	successView
	errorView
	serverSelectionView
	mainAppView // Unified view with tab-based navigation
)

type contentViewType int

const (
	recentlyAddedContent contentViewType = iota
	albumTracksContent
	playlistsContent
	playlistTracksContent
	searchContent
	settingsContent
	queueContent // New: queue view
)

// New tab-based navigation
type tabType int

const (
	homeTab tabType = iota
	libraryTab
	playlistsTab
	searchTab
	queueTab
	settingsTab
)

type playbackState int

const (
	playbackStopped playbackState = iota
	playbackPlaying
	playbackPaused
)

type model struct {
	state            sessionState
	usernameInput    textinput.Model
	passwordInput    textinput.Model
	focusIndex       int
	token            string
	err              error
	authenticator    *auth.Authenticator // Auth handler
	plexClient       *plex.Client        // Plex API client
	servers          []plexServer
	selectedServer   int
	selectedHome     int // For home menu selection
	libraries        []musicLibrary
	selectedLibrary  int
	albums           []album
	selectedAlbum    int
	playlists        []playlist
	selectedPlaylist int
	tracks           []track
	selectedTrack    int

	// Content view tracking
	currentContent contentViewType

	// New tab-based UI state
	activeTab      tabType
	showQueueModal bool
	queue          []track
	queueIndex     int
	contentScroll  int // Scroll position within current tab content

	// Terminal dimensions
	width  int
	height int

	// Playback state
	playbackState  playbackState
	currentTrack   *track // Currently playing track
	streamer       beep.StreamSeekCloser
	ctrl           *beep.Ctrl
	volume         *effects.Volume
	speakerInit    bool            // Whether speaker has been initialized
	sampleRate     beep.SampleRate // Sample rate for position calculation
	streamPosition int             // Current position in samples
	streamLength   int             // Total length in samples

	// Album art cache
	currentAlbumArt       image.Image // Cached album art for current album/track view
	currentAlbumArtThumb  string      // Thumb URL of cached art (to avoid re-fetching)
	playbackAlbumArt      image.Image // Cached album art for playback control
	playbackAlbumArtThumb string      // Thumb URL of cached playback art

	// Image renderers
	imgRenderer         *termimg.Renderer // Renderer for general views (auto-detect protocol)
	playbackImgRenderer *termimg.Renderer // Renderer for playback pane (Unicode blocks)

	// Coordinator for centralized state management (Phase 3b integration)
	coordinator *app.Coordinator
}

type authResult struct {
	token string
	err   error
}

type config struct {
	AuthToken          string `json:"authToken"`
	LastSelectedServer string `json:"lastSelectedServer,omitempty"` // Server name
}

type plexServer struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         string `json:"port"`
	AccessToken  string `json:"accessToken"`
	LocalAddress string `json:"localAddresses"`
	Scheme       string `json:"scheme"`
}

type serverListResult struct {
	servers []plexServer
	err     error
}

type musicLibrary struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type album struct {
	Title  string `json:"title"`
	Artist string `json:"parentTitle"`
	Year   int    `json:"year"`
	Key    string `json:"key"`
	Thumb  string `json:"thumb"` // Album cover art URL
}

type playlist struct {
	Title        string `json:"title"`
	Key          string `json:"key"`
	LeafCount    int    `json:"leafCount"`
	Duration     int    `json:"duration"`
	PlaylistType string `json:"playlistType"`
}

type track struct {
	Title          string `json:"title"`
	Artist         string `json:"grandparentTitle"`
	Album          string `json:"parentTitle"`
	Duration       int    `json:"duration"`
	TrackNumber    int    `json:"index"`          // Track number on original album
	PlaylistItemID int    `json:"playlistItemID"` // ID in playlist
	Key            string `json:"key"`
	RatingKey      string `json:"ratingKey"`
	Thumb          string `json:"thumb"` // Track/album cover art URL
	Media          []struct {
		Part []struct {
			Key string `json:"key"`
		} `json:"Part"`
	} `json:"Media"`
}

type plexMediaContainer struct {
	Directory []musicLibrary `json:"Directory"`
	Metadata  []album        `json:"Metadata"`
}

type plexPlaylistContainer struct {
	Metadata []playlist `json:"Metadata"`
}

type plexTrackContainer struct {
	Metadata []track `json:"Metadata"`
}

type libraryListResult struct {
	libraries []musicLibrary
	err       error
}

type albumListResult struct {
	albums []album
	err    error
}

type playlistListResult struct {
	playlists []playlist
	err       error
}

type trackListResult struct {
	tracks []track
	err    error
}

type playbackStartResult struct {
	streamer beep.StreamSeekCloser
	format   beep.Format
	err      error
}

type albumArtResult struct {
	img       image.Image
	thumbURL  string
	isPlayback bool // true if this is for playback pane, false if for track list
	err       error
}

type playbackMsg int

const (
	playbackMsgPlay playbackMsg = iota
	playbackMsgPause
	playbackMsgStop
	playbackMsgNext
	playbackMsgPrevious
)

// tickMsg is sent periodically to update playback position
type tickMsg time.Time

// buildStreamURL constructs the URL to stream a track from Plex
func (m model) buildStreamURL(track track) string {
	server := m.servers[m.selectedServer]

	// Use Media.Part.Key if available (actual audio file path)
	if len(track.Media) > 0 && len(track.Media[0].Part) > 0 {
		partKey := track.Media[0].Part[0].Key
		return fmt.Sprintf("%s://%s:%s%s?X-Plex-Token=%s",
			server.Scheme, server.Host, server.Port, partKey, server.AccessToken)
	}

	// Fallback to track.Key (metadata endpoint)
	return fmt.Sprintf("%s://%s:%s%s?X-Plex-Token=%s",
		server.Scheme, server.Host, server.Port, track.Key, server.AccessToken)
}

// isLocalAddress checks if a host is a local/private IP address
func isLocalAddress(host string) bool {
	// Check for localhost
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Parse the IP address
	ip := net.ParseIP(host)
	if ip == nil {
		// If not an IP, it's a hostname - assume remote (use proper TLS)
		return false
	}

	// Check for private IP ranges
	// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, and link-local addresses
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// getHTTPClient returns an HTTP client with appropriate TLS settings
// For local/private IPs, it skips TLS verification (self-signed certs)
// For remote/public IPs, it uses proper TLS verification
func getHTTPClient(host string) *http.Client {
	if isLocalAddress(host) {
		// Local server - skip TLS verification for self-signed certificates
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	// Remote server - use proper TLS verification
	return &http.Client{}
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(homeDir, ".config", "plexmusic-tui")
	return filepath.Join(configDir, "config.json"), nil
}

func loadConfig() (*config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &config{}, nil
		}
		return nil, err
	}

	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfig(cfg *config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}

// Style aliases for convenience - these are defined in internal/ui/styles.go
var (
	titleStyle         = ui.TitleStyle
	focusedStyle       = ui.FocusedStyle
	blurredStyle       = ui.BlurredStyle
	buttonStyle        = ui.ButtonStyle
	buttonBlurredStyle = ui.ButtonBlurredStyle
	errorStyle         = ui.ErrorStyle
	successStyle       = ui.SuccessStyle
)

func initialModel() model {
	usernameInput := textinput.New()
	usernameInput.Placeholder = "Email or username"
	usernameInput.Focus()
	usernameInput.CharLimit = 100
	usernameInput.Width = 40

	passwordInput := textinput.New()
	passwordInput.Placeholder = "Password"
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.EchoCharacter = '•'
	passwordInput.CharLimit = 100
	passwordInput.Width = 40

	m := model{
		state:         loginView,
		usernameInput: usernameInput,
		passwordInput: passwordInput,
		focusIndex:    0,
		authenticator: auth.NewAuthenticator(),
		coordinator:   app.NewCoordinator(),
	}
	
	// Initialize renderers
	m.imgRenderer = termimg.NewRenderer()
	m.playbackImgRenderer = termimg.NewRendererWithProtocol(termimg.ProtocolUnicodeBlocks)
	
	return m
}

func (m model) Init() tea.Cmd {
	if m.state == serverSelectionView {
		return m.fetchServers()
	}
	return textinput.Blink
}

// tickCmd sends a tick message every 100ms to update playback position
func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			// Escape is back navigation
			switch m.state {
			case loginView, successView, errorView:
				// From these states, esc quits
				return m, tea.Quit
			case serverSelectionView:
				// From server selection, go back to login or quit
				return m, tea.Quit
			case mainAppView:
				// NEW: Close queue modal if it's open
				if m.showQueueModal {
					m.showQueueModal = false
					return m, nil
				}
				// Handle back navigation within tabs
				if m.currentContent == playlistTracksContent {
					// Go back from tracks to playlists
					m.currentContent = playlistsContent
					m.tracks = nil // Clear tracks
					m.selectedTrack = 0
					return m, nil
				} else if m.currentContent == albumTracksContent {
					// Go back from tracks to albums
					m.currentContent = recentlyAddedContent
					m.tracks = nil // Clear tracks
					m.selectedTrack = 0
					return m, nil
				}
				// From main app, go back to server selection
				m.state = serverSelectionView
			}
			return m, nil

		case "tab", "shift+tab", "up", "down":
			if m.state == loginView {
				s := msg.String()
				if s == "up" || s == "shift+tab" {
					m.focusIndex--
				} else {
					m.focusIndex++
				}

				if m.focusIndex > 2 {
					m.focusIndex = 0
				} else if m.focusIndex < 0 {
					m.focusIndex = 2
				}

				cmds := make([]tea.Cmd, 2)
				for i := 0; i < 2; i++ {
					if i == m.focusIndex {
						cmds[i] = m.getInput(i).Focus()
					} else {
						m.getInput(i).Blur()
					}
				}
				return m, tea.Batch(cmds...)
			} else if m.state == mainAppView {
				s := msg.String()
				if s == "tab" || s == "shift+tab" {
					// Cycle through horizontal tabs
					if s == "tab" {
						// Forward through tabs
						m.activeTab++
						if m.activeTab > settingsTab {
							m.activeTab = homeTab
						}
					} else {
						// Backward through tabs
						m.activeTab--
						if m.activeTab < homeTab {
							m.activeTab = settingsTab
						}
					}
					// Reset scroll when switching tabs
					m.contentScroll = 0
					return m, nil
				} else if s == "up" || s == "down" {
					// Navigate within current tab content
					switch m.activeTab {
				case libraryTab:
					if m.currentContent == recentlyAddedContent {
						// Navigate albums
						if s == "up" {
							m.selectedAlbum--
							if m.selectedAlbum < 0 {
								m.selectedAlbum = len(m.albums) - 1
							}
						} else {
							m.selectedAlbum++
							if m.selectedAlbum >= len(m.albums) {
								m.selectedAlbum = 0
							}
						}
					} else if m.currentContent == albumTracksContent {
						// Navigate tracks in album
						if s == "up" {
							m.selectedTrack--
							if m.selectedTrack < 0 {
								m.selectedTrack = len(m.tracks) - 1
							}
						} else {
							m.selectedTrack++
							if m.selectedTrack >= len(m.tracks) {
								m.selectedTrack = 0
							}
						}
					}
					case playlistsTab:
						if m.currentContent == playlistsContent {
							// Navigate playlists
							if s == "up" {
								m.selectedPlaylist--
								if m.selectedPlaylist < 0 {
									m.selectedPlaylist = len(m.playlists) - 1
								}
							} else {
								m.selectedPlaylist++
								if m.selectedPlaylist >= len(m.playlists) {
									m.selectedPlaylist = 0
								}
							}
						} else if m.currentContent == playlistTracksContent {
							// Navigate tracks in playlist
							if s == "up" {
								m.selectedTrack--
								if m.selectedTrack < 0 {
									m.selectedTrack = len(m.tracks) - 1
								}
							} else {
								m.selectedTrack++
								if m.selectedTrack >= len(m.tracks) {
									m.selectedTrack = 0
								}
							}
						}
					case queueTab:
						// Navigate queue items
						if s == "up" {
							m.queueIndex--
							if m.queueIndex < 0 {
								m.queueIndex = len(m.queue) - 1
							}
						} else {
							m.queueIndex++
							if m.queueIndex >= len(m.queue) {
								m.queueIndex = 0
							}
						}
					// homeTab, searchTab, settingsTab don't need navigation yet
					}
				}
			} else if m.state == serverSelectionView {
				s := msg.String()
				if s == "up" || s == "shift+tab" {
					m.selectedServer--
					if m.selectedServer < 0 {
						m.selectedServer = len(m.servers) - 1
					}
				} else {
					m.selectedServer++
					if m.selectedServer >= len(m.servers) {
						m.selectedServer = 0
					}
				}
			}

		case " ", "p":
			// Space or 'p' for play/pause (only in main app view)
			if m.state == mainAppView {
				if m.playbackState == playbackPlaying {
					// Pause
					if m.ctrl != nil {
						speaker.Lock()
						m.ctrl.Paused = true
						speaker.Unlock()
						m.playbackState = playbackPaused
					}
				} else if m.playbackState == playbackPaused {
					// Resume
					if m.ctrl != nil {
						speaker.Lock()
						m.ctrl.Paused = false
						speaker.Unlock()
						m.playbackState = playbackPlaying
						// Restart ticker
						return m, tickCmd()
					}
				} else if m.playbackState == playbackStopped {
					// Start playback of selected track
					if m.currentContent == playlistTracksContent && len(m.tracks) > 0 {
						track := m.tracks[m.selectedTrack]
						m.currentTrack = &track
						return m, m.startPlayback(track)
					} else if m.currentContent == albumTracksContent && len(m.tracks) > 0 {
						track := m.tracks[m.selectedTrack]
						m.currentTrack = &track
						return m, m.startPlayback(track)
					}
				}
				return m, nil
			}

		case "q":
			// Toggle queue modal (only in main app view)
			if m.state == mainAppView {
				m.showQueueModal = !m.showQueueModal
				return m, nil
			}

		case "s":
			// Stop playback (only in main app view)
			if m.state == mainAppView && m.playbackState != playbackStopped {
				if m.streamer != nil {
					speaker.Clear()
					m.streamer.Close()
					m.streamer = nil
				}
				m.playbackState = playbackStopped
				m.currentTrack = nil
			}
			// Only consume 's' if we're in mainAppView; otherwise let it pass to text input
			if m.state == mainAppView {
				return m, nil
			}

		case "n":
			// Next track (only in main app view)
			if m.state == mainAppView && len(m.tracks) > 0 {
				// Stop current playback
				if m.streamer != nil {
					speaker.Clear()
					m.streamer.Close()
					m.streamer = nil
				}

				// Move to next track
				m.selectedTrack++
				if m.selectedTrack >= len(m.tracks) {
					m.selectedTrack = 0
				}

				// Start playing next track
				track := m.tracks[m.selectedTrack]
				m.currentTrack = &track
				m.playbackState = playbackStopped // Will be set to playing when stream loads
				return m, m.startPlayback(track)
			}
			// Only consume 'n' if we're in mainAppView; otherwise let it pass to text input
			if m.state == mainAppView {
				return m, nil
			}

		case "b":
			// Previous track (only in main app view)
			if m.state == mainAppView && len(m.tracks) > 0 {
				// Stop current playback
				if m.streamer != nil {
					speaker.Clear()
					m.streamer.Close()
					m.streamer = nil
				}

				// Move to previous track
				m.selectedTrack--
				if m.selectedTrack < 0 {
					m.selectedTrack = len(m.tracks) - 1
				}

				// Start playing previous track
				track := m.tracks[m.selectedTrack]
				m.currentTrack = &track
				m.playbackState = playbackStopped // Will be set to playing when stream loads
				return m, m.startPlayback(track)
			}
			// Only consume 'b' if we're in mainAppView; otherwise let it pass to text input
			if m.state == mainAppView {
				return m, nil
			}

		case "-":
			// Decrease volume (only in main app view)
			if m.state == mainAppView && m.volume != nil {
				newVolume := m.volume.Volume - 0.05
				if newVolume < 0 {
					newVolume = 0
				}
				m.volume.Volume = newVolume
			}
			return m, nil

		case "+", "=":
			// Increase volume (only in main app view)
			// "=" because Shift+= is + on US keyboards
			if m.state == mainAppView && m.volume != nil {
				newVolume := m.volume.Volume + 0.05
				if newVolume > 1 {
					newVolume = 1
				}
				m.volume.Volume = newVolume
			}
			return m, nil

		case "left", "right":
			// NEW: Left/Right arrow keys for tab navigation (only in main app view)
			if m.state == mainAppView && !m.showQueueModal {
				if msg.String() == "left" {
					// Previous tab
					m.activeTab--
					if m.activeTab < homeTab {
						m.activeTab = settingsTab
					}
				} else {
					// Next tab
					m.activeTab++
					if m.activeTab > settingsTab {
						m.activeTab = homeTab
					}
				}
				// Reset scroll when switching tabs
				m.contentScroll = 0

				// Lazy-load playlists when switching to playlists tab
				if m.activeTab == playlistsTab && len(m.playlists) == 0 {
					return m, m.fetchPlaylistsWithCoordinator()
				}
				return m, nil
			}

		case "a":
			// Add current track to queue (only in main app view)
			if m.state == mainAppView {
				// Can add from playlist tracks or album tracks
				if (m.currentContent == playlistTracksContent || m.currentContent == albumTracksContent) && len(m.tracks) > 0 {
					track := m.tracks[m.selectedTrack]
					// Check if track is already in queue to avoid duplicates
					alreadyInQueue := false
					for _, queueTrack := range m.queue {
						if queueTrack.Key == track.Key {
							alreadyInQueue = true
							break
						}
					}
					if !alreadyInQueue {
						m.queue = append(m.queue, track)
					}
				}
				return m, nil
			}

		case "d":
			// Remove selected item from queue (only when on queue tab)
			if m.state == mainAppView && m.activeTab == queueTab && len(m.queue) > 0 {
				// Remove the selected queue item
				if m.queueIndex < len(m.queue) {
					m.queue = append(m.queue[:m.queueIndex], m.queue[m.queueIndex+1:]...)
					// Adjust queue index if necessary
					if m.queueIndex >= len(m.queue) && len(m.queue) > 0 {
						m.queueIndex = len(m.queue) - 1
					}
					if len(m.queue) == 0 {
						m.queueIndex = 0
					}
				}
				return m, nil
			}

		case "c":
			// Clear entire queue (only when on queue tab)
			if m.state == mainAppView && m.activeTab == queueTab && len(m.queue) > 0 {
				m.queue = []track{}
				m.queueIndex = 0
				return m, nil
			}

		case "j", "ctrl+j":
			// Move queue item down (only when on queue tab)
			if m.state == mainAppView && m.activeTab == queueTab && len(m.queue) > 1 && m.queueIndex < len(m.queue)-1 {
				// Swap current item with the one below
				m.queue[m.queueIndex], m.queue[m.queueIndex+1] = m.queue[m.queueIndex+1], m.queue[m.queueIndex]
				m.queueIndex++
				return m, nil
			}

		case "k", "ctrl+k":
			// Move queue item up (only when on queue tab)
			if m.state == mainAppView && m.activeTab == queueTab && len(m.queue) > 1 && m.queueIndex > 0 {
				// Swap current item with the one above
				m.queue[m.queueIndex], m.queue[m.queueIndex-1] = m.queue[m.queueIndex-1], m.queue[m.queueIndex]
				m.queueIndex--
				return m, nil
			}

		case "enter":
			if m.state == loginView && m.focusIndex == 2 {
				m.state = authenticatingView
				return m, m.authenticate()
			}
			if m.state == successView || m.state == errorView {
				return m, tea.Quit
			}
			if m.state == serverSelectionView && len(m.servers) > 0 {
				m.state = mainAppView
				m.activeTab = homeTab  // Start at home tab
				m.currentContent = recentlyAddedContent
				// Save the selected server to config
				cfg, _ := loadConfig()
				if cfg == nil {
					cfg = &config{}
				}
				cfg.AuthToken = m.token
				cfg.LastSelectedServer = m.servers[m.selectedServer].Name
				saveConfig(cfg)
				// Fetch recently added on startup
				return m, m.fetchRecentlyAdded()
			}
			if m.state == mainAppView {
				// Handle Enter key based on active tab
				switch m.activeTab {
				case libraryTab:
					if m.currentContent == recentlyAddedContent && len(m.albums) > 0 {
						// Select album and view tracks
						m.currentContent = albumTracksContent
						m.selectedTrack = 0
						return m, m.fetchAlbumTracks()
					} else if m.currentContent == albumTracksContent && len(m.tracks) > 0 {
						// Play selected track
						track := m.tracks[m.selectedTrack]
						m.currentTrack = &track
						return m, m.startPlayback(track)
					}
				case playlistsTab:
					if m.currentContent == playlistsContent && len(m.playlists) > 0 {
						// Switch to playlist tracks view and fetch tracks
						m.currentContent = playlistTracksContent
						m.selectedTrack = 0
						return m, m.fetchPlaylistTracks()
					} else if m.currentContent == playlistTracksContent && len(m.tracks) > 0 {
						// Play selected track
						track := m.tracks[m.selectedTrack]
						m.currentTrack = &track
						return m, m.startPlayback(track)
					}
				case queueTab:
					// Play selected queue item
					if len(m.queue) > 0 && m.queueIndex < len(m.queue) {
						track := m.queue[m.queueIndex]
						m.currentTrack = &track
						return m, m.startPlayback(track)
					}
				// homeTab, searchTab, settingsTab don't have selection actions yet
				}
			}
		}

	case authResult:
		if msg.err != nil {
			m.state = errorView
			m.err = msg.err
		} else {
			m.state = serverSelectionView
			m.token = msg.token
			// Save token to config
			cfg := &config{AuthToken: msg.token}
			if err := saveConfig(cfg); err != nil {
				m.err = fmt.Errorf("token saved but config write failed: %w", err)
			}
			return m, m.fetchServers()
		}
		return m, nil

	case serverListResult:
		if msg.err != nil {
			m.state = errorView
			m.err = msg.err
		} else {
			m.servers = msg.servers
			if len(m.servers) == 0 {
				m.state = errorView
				m.err = fmt.Errorf("no Plex servers found")
			} else {
				// Try to auto-select the last used server
				cfg, _ := loadConfig()
				if cfg != nil && cfg.LastSelectedServer != "" {
					for i, server := range m.servers {
						if server.Name == cfg.LastSelectedServer {
							m.selectedServer = i
							// Auto-navigate to main app view
							m.state = mainAppView
							m.currentContent = recentlyAddedContent
							// Fetch recently added on startup
							return m, m.fetchRecentlyAdded()
						}
					}
				}
			}
		}
		return m, nil

	case libraryListResult:
		if msg.err != nil {
			m.state = errorView
			m.err = msg.err
		} else {
			m.libraries = msg.libraries
			if len(m.libraries) == 0 {
				m.state = errorView
				m.err = fmt.Errorf("no music libraries found on server")
			}
		}
		return m, nil

	case albumListResult:
		if msg.err != nil {
			m.state = errorView
			m.err = msg.err
		} else {
			m.albums = msg.albums
		}
		return m, nil

	case playlistListResult:
		if msg.err != nil {
			m.state = errorView
			m.err = msg.err
		} else {
			m.playlists = msg.playlists
		}
		return m, nil

	case trackListResult:
		if msg.err != nil {
			m.state = errorView
			m.err = msg.err
		} else {
			m.tracks = msg.tracks
			// Fetch album art asynchronously if we have tracks with thumbnails
			if len(msg.tracks) > 0 && msg.tracks[0].Thumb != "" {
				return m, m.fetchAlbumArtAsync(msg.tracks[0].Thumb, false)
			}
		}
		return m, nil

	case albumArtResult:
		// Update cached album art if fetch was successful
		if msg.err == nil && msg.img != nil {
			if msg.isPlayback {
				// Update playback album art
				m.playbackAlbumArt = msg.img
				m.playbackAlbumArtThumb = msg.thumbURL
			} else {
				// Update track list album art
				m.currentAlbumArt = msg.img
				m.currentAlbumArtThumb = msg.thumbURL
			}
		}
		return m, nil

	case playbackStartResult:
		if msg.err != nil {
			m.err = msg.err
			// Could show error in UI, for now just stop playback
			m.playbackState = playbackStopped
			return m, nil
		}

		// Initialize speaker if needed
		if !m.speakerInit {
			err := speaker.Init(msg.format.SampleRate, msg.format.SampleRate.N(time.Second/10))
			if err != nil {
				m.err = fmt.Errorf("failed to initialize speaker: %w", err)
				m.playbackState = playbackStopped
				if msg.streamer != nil {
					msg.streamer.Close()
				}
				return m, nil
			}
			m.speakerInit = true
		}

		// Stop any existing playback
		if m.streamer != nil {
			speaker.Clear()
			m.streamer.Close()
		}

		// Store the streamer and sample rate
		m.streamer = msg.streamer
		m.sampleRate = msg.format.SampleRate

		// Create control wrapper for pause/resume
		m.ctrl = &beep.Ctrl{Streamer: m.streamer, Paused: false}

		// Create volume control
		m.volume = &effects.Volume{
			Streamer: m.ctrl,
			Base:     2,
			Volume:   0, // 0 = normal volume
			Silent:   false,
		}

		// Start playback
		speaker.Play(m.volume)
		m.playbackState = playbackPlaying

		// Fetch album art asynchronously for playback pane
		var cmds []tea.Cmd
		cmds = append(cmds, tickCmd())
		if m.currentTrack != nil && m.currentTrack.Thumb != "" {
			cmds = append(cmds, m.fetchAlbumArtAsync(m.currentTrack.Thumb, true))
		}
		return m, tea.Batch(cmds...)

	case app.CoordinatorMsg:
		// Handle coordinator messages
		handled := m.coordinator.Dispatch(msg)
		if !handled {
			return m, nil
		}
		
		// After dispatching to coordinator, sync state based on message type
		switch msg.Type {
		case app.MessageSetServers:
			// Convert coordinator servers back to main model
			if servers, ok := msg.Data.([]app.PlexServer); ok {
				var result []plexServer
				for _, s := range servers {
					result = append(result, plexServer{
						Name:         s.Name,
						Host:         s.Host,
						Port:         s.Port,
						AccessToken:  s.AccessToken,
						LocalAddress: s.LocalAddress,
						Scheme:       s.Scheme,
					})
				}
				m.servers = result
				
				if len(m.servers) == 0 {
					m.state = errorView
					m.err = fmt.Errorf("no Plex servers found")
				} else {
					// Try to auto-select the last used server
					cfg, _ := loadConfig()
					if cfg != nil && cfg.LastSelectedServer != "" {
						for i, server := range m.servers {
							if server.Name == cfg.LastSelectedServer {
								m.selectedServer = i
								// Auto-navigate to main app view
								m.state = mainAppView
								m.currentContent = recentlyAddedContent
								// Fetch recently added on startup
								return m, m.fetchRecentlyAdded()
							}
						}
					}
				}
			}

		case app.MessageSetLibraries:
			// Convert coordinator libraries back to main model
			if libs, ok := msg.Data.([]app.MusicLibrary); ok {
				var result []musicLibrary
				for _, l := range libs {
					result = append(result, musicLibrary{
						Key:   l.Key,
						Title: l.Title,
						Type:  l.Type,
					})
				}
				m.libraries = result
				
				if len(m.libraries) == 0 {
					m.state = errorView
					m.err = fmt.Errorf("no music libraries found on server")
				}
			}

		case app.MessageSetAlbums:
			// Convert coordinator albums back to main model
			if albums, ok := msg.Data.([]app.Album); ok {
				var result []album
				for _, a := range albums {
					result = append(result, album{
						Title:  a.Title,
						Artist: a.Artist,
						Year:   a.Year,
						Key:    a.Key,
						Thumb:  a.Thumb,
					})
				}
				m.albums = result
			}

		case app.MessageSetTracks:
			// Convert coordinator tracks back to main model
			if tracks, ok := msg.Data.([]app.Track); ok {
				var result []track
				for _, t := range tracks {
					// Convert app.Track.Media back to track.Media type
					media := make([]struct {
						Part []struct {
							Key string `json:"key"`
						} `json:"Part"`
					}, len(t.Media))
					for i, m := range t.Media {
						media[i].Part = make([]struct {
							Key string `json:"key"`
						}, len(m.Part))
						for j, p := range m.Part {
							media[i].Part[j].Key = p.Key
						}
					}

					result = append(result, track{
						Title:          t.Title,
						Artist:         t.Artist,
						Album:          t.Album,
						Duration:       t.Duration,
						TrackNumber:    t.TrackNumber,
						PlaylistItemID: t.PlaylistItemID,
						Key:            t.Key,
						RatingKey:      t.RatingKey,
						Thumb:          t.Thumb,
						Media:          media,
					})
				}
				m.tracks = result
				// Fetch album art asynchronously if we have tracks with thumbnails
				if len(result) > 0 && result[0].Thumb != "" {
					return m, m.fetchAlbumArtAsync(result[0].Thumb, false)
				}
			}

		case app.MessageSetPlaylists:
			// Convert coordinator playlists back to main model
			if playlists, ok := msg.Data.([]app.Playlist); ok {
				var result []playlist
				for _, p := range playlists {
					result = append(result, playlist{
						Title:        p.Title,
						Key:          p.Key,
						LeafCount:    p.LeafCount,
						Duration:     p.Duration,
						PlaylistType: p.PlaylistType,
					})
				}
				m.playlists = result
			}
		}
		return m, nil

	case tickMsg:
		// Update playback position and schedule next tick
		if m.playbackState == playbackPlaying {
			return m, tickCmd()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	cmd := m.updateInputs(msg)
	return m, cmd
}

func (m *model) getInput(index int) *textinput.Model {
	switch index {
	case 0:
		return &m.usernameInput
	case 1:
		return &m.passwordInput
	default:
		return &m.usernameInput
	}
}

func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 2)

	if m.focusIndex == 0 {
		m.usernameInput, cmds[0] = m.usernameInput.Update(msg)
	} else if m.focusIndex == 1 {
		m.passwordInput, cmds[1] = m.passwordInput.Update(msg)
	}

	return tea.Batch(cmds...)
}

func (m model) authenticate() tea.Cmd {
	return func() tea.Msg {
		username := m.usernameInput.Value()
		password := m.passwordInput.Value()

		token, err := m.authenticator.AuthenticateUser(username, password)
		if err != nil {
			return authResult{err: err}
		}

		return authResult{token: token}
	}
}

func (m model) fetchServersWithCoordinator() tea.Cmd {
	// Create a closure that fetches servers and converts them to app.PlexServer
	fetchFn := func() ([]app.PlexServer, error) {
		servers, err := m.authenticator.FetchServers(m.token)
		if err != nil {
			return nil, err
		}

		// Convert domain.PlexServer to app.PlexServer
		var result []app.PlexServer
		for _, server := range servers {
			result = append(result, app.PlexServer{
				Name:        server.Name,
				Host:        server.Host,
				Port:        server.Port,
				AccessToken: server.AccessToken,
				Scheme:      server.Scheme,
			})
		}
		return result, nil
	}

	return m.coordinator.FetchServersCmd(fetchFn)
}

func (m model) fetchServers() tea.Cmd {
	return func() tea.Msg {
		servers, err := m.authenticator.FetchServers(m.token)
		if err != nil {
			return serverListResult{err: err}
		}

		// Convert domain.PlexServer to plexServer
		var result []plexServer
		for _, server := range servers {
			result = append(result, plexServer{
				Name:        server.Name,
				Host:        server.Host,
				Port:        server.Port,
				AccessToken: server.AccessToken,
				Scheme:      server.Scheme,
			})
		}

		return serverListResult{servers: result}
	}
}

func (m model) fetchLibrariesWithCoordinator() tea.Cmd {
	fetchFn := func() ([]app.MusicLibrary, error) {
		server := m.servers[m.selectedServer]
		url := fmt.Sprintf("%s://%s:%s/library/sections", server.Scheme, server.Host, server.Port)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch libraries: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("library fetch failed (status %d): %s", resp.StatusCode, string(body))
		}

		var container struct {
			MediaContainer plexMediaContainer `json:"MediaContainer"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Filter for music libraries only
		var musicLibs []app.MusicLibrary
		for _, lib := range container.MediaContainer.Directory {
			if lib.Type == "artist" {
				musicLibs = append(musicLibs, app.MusicLibrary{
					Key:   lib.Key,
					Title: lib.Title,
					Type:  lib.Type,
				})
			}
		}

		return musicLibs, nil
	}

	return m.coordinator.FetchLibrariesCmd(fetchFn)
}

func (m model) fetchLibraries() tea.Cmd {
	return func() tea.Msg {
		server := m.servers[m.selectedServer]
		url := fmt.Sprintf("%s://%s:%s/library/sections", server.Scheme, server.Host, server.Port)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return libraryListResult{err: fmt.Errorf("failed to create request: %w", err)}
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return libraryListResult{err: fmt.Errorf("failed to fetch libraries: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return libraryListResult{err: fmt.Errorf("library fetch failed (status %d): %s", resp.StatusCode, string(body))}
		}

		var container struct {
			MediaContainer plexMediaContainer `json:"MediaContainer"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
			return libraryListResult{err: fmt.Errorf("failed to decode response: %w", err)}
		}

		// Filter for music libraries only
		var musicLibs []musicLibrary
		for _, lib := range container.MediaContainer.Directory {
			if lib.Type == "artist" {
				musicLibs = append(musicLibs, lib)
			}
		}

		return libraryListResult{libraries: musicLibs}
	}
}

func (m model) fetchAlbumsWithCoordinator() tea.Cmd {
	fetchFn := func() ([]app.Album, error) {
		server := m.servers[m.selectedServer]
		library := m.libraries[m.selectedLibrary]
		url := fmt.Sprintf("%s://%s:%s/library/sections/%s/albums", server.Scheme, server.Host, server.Port, library.Key)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch albums: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("album fetch failed (status %d): %s", resp.StatusCode, string(body))
		}

		var container struct {
			MediaContainer plexMediaContainer `json:"MediaContainer"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Convert albums to app.Album
		var result []app.Album
		for _, a := range container.MediaContainer.Metadata {
			result = append(result, app.Album{
				Title:  a.Title,
				Artist: a.Artist,
				Year:   a.Year,
				Key:    a.Key,
				Thumb:  a.Thumb,
			})
		}

		return result, nil
	}

	return m.coordinator.FetchAlbumsCmd(fetchFn)
}

func (m model) fetchAlbums() tea.Cmd {
	return func() tea.Msg {
		server := m.servers[m.selectedServer]
		library := m.libraries[m.selectedLibrary]
		url := fmt.Sprintf("%s://%s:%s/library/sections/%s/albums", server.Scheme, server.Host, server.Port, library.Key)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return albumListResult{err: fmt.Errorf("failed to create request: %w", err)}
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return albumListResult{err: fmt.Errorf("failed to fetch albums: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return albumListResult{err: fmt.Errorf("album fetch failed (status %d): %s", resp.StatusCode, string(body))}
		}

		var container struct {
			MediaContainer plexMediaContainer `json:"MediaContainer"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
			return albumListResult{err: fmt.Errorf("failed to decode response: %w", err)}
		}

		return albumListResult{albums: container.MediaContainer.Metadata}
	}
}

func (m model) fetchRecentlyAdded() tea.Cmd {
	return func() tea.Msg {
		server := m.servers[m.selectedServer]
		// Fetch recently added albums from all music libraries
		url := fmt.Sprintf("%s://%s:%s/library/recentlyAdded?type=9", server.Scheme, server.Host, server.Port)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return albumListResult{err: fmt.Errorf("failed to create request: %w", err)}
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return albumListResult{err: fmt.Errorf("failed to fetch recently added: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return albumListResult{err: fmt.Errorf("recently added fetch failed (status %d): %s", resp.StatusCode, string(body))}
		}

		var container struct {
			MediaContainer plexMediaContainer `json:"MediaContainer"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
			return albumListResult{err: fmt.Errorf("failed to decode response: %w", err)}
		}

		return albumListResult{albums: container.MediaContainer.Metadata}
	}
}

func (m model) fetchPlaylists() tea.Cmd {
	return func() tea.Msg {
		server := m.servers[m.selectedServer]
		// Fetch all playlists from the server
		url := fmt.Sprintf("%s://%s:%s/playlists", server.Scheme, server.Host, server.Port)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return playlistListResult{err: fmt.Errorf("failed to create request: %w", err)}
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return playlistListResult{err: fmt.Errorf("failed to fetch playlists: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return playlistListResult{err: fmt.Errorf("playlist fetch failed (status %d): %s", resp.StatusCode, string(body))}
		}

		// Read and log the response body for debugging
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return playlistListResult{err: fmt.Errorf("failed to read response: %w", err)}
		}

		var container struct {
			MediaContainer plexPlaylistContainer `json:"MediaContainer"`
		}
		if err := json.Unmarshal(body, &container); err != nil {
			return playlistListResult{err: fmt.Errorf("failed to decode response: %w (body: %s)", err, string(body))}
		}

		return playlistListResult{playlists: container.MediaContainer.Metadata}
	}
}

// Common function to fetch tracks from a Plex key (album or playlist) via Coordinator
func (m model) fetchTracksWithCoordinator(key string, source string) tea.Cmd {
	fetchFn := func() ([]app.Track, error) {
		server := m.servers[m.selectedServer]
		url := fmt.Sprintf("%s://%s:%s%s", server.Scheme, server.Host, server.Port, key)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s tracks: %w", source, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("%s tracks fetch failed (status %d): %s", source, resp.StatusCode, string(body))
		}

		var container struct {
			MediaContainer plexTrackContainer `json:"MediaContainer"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Convert tracks to app.Track
		var result []app.Track
		for _, t := range container.MediaContainer.Metadata {
			// Convert Media field from track type to app.Track type
			media := make([]struct {
				Part []struct {
					Key string
				}
			}, len(t.Media))
			for i, m := range t.Media {
				media[i].Part = make([]struct {
					Key string
				}, len(m.Part))
				for j, p := range m.Part {
					media[i].Part[j].Key = p.Key
				}
			}

			result = append(result, app.Track{
				Title:          t.Title,
				Artist:         t.Artist,
				Album:          t.Album,
				Duration:       t.Duration,
				TrackNumber:    t.TrackNumber,
				PlaylistItemID: t.PlaylistItemID,
				Key:            t.Key,
				RatingKey:      t.RatingKey,
				Thumb:          t.Thumb,
				Media:          media,
			})
		}

		return result, nil
	}

	return m.coordinator.FetchTracksCmd(fetchFn)
}

// Fetch playlists from the Plex server via Coordinator
func (m model) fetchPlaylistsWithCoordinator() tea.Cmd {
	fetchFn := func() ([]app.Playlist, error) {
		server := m.servers[m.selectedServer]
		// Fetch all playlists from the server
		url := fmt.Sprintf("%s://%s:%s/playlists", server.Scheme, server.Host, server.Port)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch playlists: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("playlist fetch failed (status %d): %s", resp.StatusCode, string(body))
		}

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		var container struct {
			MediaContainer plexPlaylistContainer `json:"MediaContainer"`
		}
		if err := json.Unmarshal(body, &container); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w (body: %s)", err, string(body))
		}

		// Convert playlists to app.Playlist
		var result []app.Playlist
		for _, p := range container.MediaContainer.Metadata {
			result = append(result, app.Playlist{
				Title:        p.Title,
				Key:          p.Key,
				LeafCount:    p.LeafCount,
				Duration:     p.Duration,
				PlaylistType: p.PlaylistType,
			})
		}

		return result, nil
	}

	return m.coordinator.FetchPlaylistsCmd(fetchFn)
}

// Common function to fetch tracks from a Plex key (album or playlist)
func (m model) fetchTracks(key string, source string) tea.Cmd {
	return func() tea.Msg {
		server := m.servers[m.selectedServer]
		url := fmt.Sprintf("%s://%s:%s%s", server.Scheme, server.Host, server.Port, key)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return trackListResult{err: fmt.Errorf("failed to create request: %w", err)}
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return trackListResult{err: fmt.Errorf("failed to fetch %s tracks: %w", source, err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return trackListResult{err: fmt.Errorf("%s tracks fetch failed (status %d): %s", source, resp.StatusCode, string(body))}
		}

		var container struct {
			MediaContainer plexTrackContainer `json:"MediaContainer"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
			return trackListResult{err: fmt.Errorf("failed to decode response: %w", err)}
		}

		return trackListResult{tracks: container.MediaContainer.Metadata}
	}
}

func (m model) fetchPlaylistTracks() tea.Cmd {
	playlist := m.playlists[m.selectedPlaylist]
	return m.fetchTracksWithCoordinator(playlist.Key, "playlist")
}

func (m model) fetchAlbumTracks() tea.Cmd {
	album := m.albums[m.selectedAlbum]
	return m.fetchTracksWithCoordinator(album.Key, "album")
}

// startPlayback fetches and decodes audio from Plex server
func (m model) startPlayback(track track) tea.Cmd {
	return func() tea.Msg {
		streamURL := m.buildStreamURL(track)
		server := m.servers[m.selectedServer]

		req, err := http.NewRequest("GET", streamURL, nil)
		if err != nil {
			return playbackStartResult{err: fmt.Errorf("failed to create request: %w", err)}
		}

		req.Header.Set("X-Plex-Token", server.AccessToken)

		client := getHTTPClient(server.Host)
		resp, err := client.Do(req)
		if err != nil {
			return playbackStartResult{err: fmt.Errorf("failed to fetch audio: %w", err)}
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return playbackStartResult{err: fmt.Errorf("audio fetch failed (status %d)", resp.StatusCode)}
		}

		// Try to decode the audio stream
		// We'll try multiple decoders based on common audio formats
		var streamer beep.StreamSeekCloser
		var format beep.Format

		// Import the decoders dynamically to detect format
		contentType := resp.Header.Get("Content-Type")

		// Try MP3 first (most common)
		if strings.Contains(contentType, "mp3") || strings.Contains(contentType, "mpeg") {
			mp3Streamer, mp3Format, err := mp3.Decode(resp.Body)
			if err == nil {
				streamer = mp3Streamer
				format = mp3Format
			}
		}

		// Try FLAC
		if streamer == nil && (strings.Contains(contentType, "flac") || strings.Contains(contentType, "x-flac")) {
			flacStreamer, flacFormat, err := flac.Decode(resp.Body)
			if err == nil {
				streamer = flacStreamer
				format = flacFormat
			}
		}

		// Try Vorbis/OGG
		if streamer == nil && strings.Contains(contentType, "ogg") {
			vorbisStreamer, vorbisFormat, err := vorbis.Decode(resp.Body)
			if err == nil {
				streamer = vorbisStreamer
				format = vorbisFormat
			}
		}

		// Try WAV
		if streamer == nil && strings.Contains(contentType, "wav") {
			wavStreamer, wavFormat, err := wav.Decode(resp.Body)
			if err == nil {
				streamer = wavStreamer
				format = wavFormat
			}
		}

		// If content type didn't help, try MP3 as default (most common format)
		if streamer == nil {
			resp.Body.Close()
			// Re-fetch for a fresh stream
			resp, err = client.Do(req)
			if err != nil {
				return playbackStartResult{err: fmt.Errorf("failed to re-fetch audio: %w", err)}
			}

			mp3Streamer, mp3Format, err := mp3.Decode(resp.Body)
			if err != nil {
				resp.Body.Close()
				return playbackStartResult{err: fmt.Errorf("failed to decode audio (tried mp3): %w", err)}
			}
			streamer = mp3Streamer
			format = mp3Format
		}

		return playbackStartResult{
			streamer: streamer,
			format:   format,
			err:      nil,
		}
	}
}

// fetchAlbumArt fetches album art from Plex server and returns the image
func (m model) fetchAlbumArt(thumbURL string) (image.Image, error) {
	if thumbURL == "" {
		return nil, fmt.Errorf("no thumb URL provided")
	}

	server := m.servers[m.selectedServer]
	// Build full URL if thumbURL is a relative path
	var fullURL string
	if strings.HasPrefix(thumbURL, "http://") || strings.HasPrefix(thumbURL, "https://") {
		fullURL = thumbURL
	} else {
		fullURL = fmt.Sprintf("%s://%s:%s%s?X-Plex-Token=%s",
			server.Scheme, server.Host, server.Port, thumbURL, server.AccessToken)
	}

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := getHTTPClient(server.Host)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch album art: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("album art fetch failed (status %d)", resp.StatusCode)
	}

	// Read image data into buffer
	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	// Decode image
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return img, nil
}

// fetchAlbumArtAsync fetches album art asynchronously
func (m model) fetchAlbumArtAsync(thumbURL string, isPlayback bool) tea.Cmd {
	return func() tea.Msg {
		img, err := m.fetchAlbumArt(thumbURL)
		return albumArtResult{
			img:        img,
			thumbURL:   thumbURL,
			isPlayback: isPlayback,
			err:        err,
		}
	}
}
func (m model) View() string {
	switch m.state {
	case loginView:
		return m.loginView()
	case authenticatingView:
		return m.authenticatingView()
	case successView:
		return m.successView()
	case errorView:
		return m.errorView()
	case serverSelectionView:
		return m.serverSelectionView()
	case mainAppView:
		return m.mainAppView()
	default:
		return ""
	}
}

func (m model) loginView() string {
	title := titleStyle.Render("Plex Music Player - Login")

	usernameLabel := "Username:"
	if m.focusIndex == 0 {
		usernameLabel = focusedStyle.Render(usernameLabel)
	} else {
		usernameLabel = blurredStyle.Render(usernameLabel)
	}

	passwordLabel := "Password:"
	if m.focusIndex == 1 {
		passwordLabel = focusedStyle.Render(passwordLabel)
	} else {
		passwordLabel = blurredStyle.Render(passwordLabel)
	}

	button := " Login "
	if m.focusIndex == 2 {
		button = buttonStyle.Render(button)
	} else {
		button = buttonBlurredStyle.Render(button)
	}

	help := blurredStyle.Render("\n  Tab: Next • Enter: Login • Ctrl+C: Quit\n")

	return fmt.Sprintf(
		"\n%s\n\n  %s\n  %s\n\n  %s\n  %s\n\n  %s\n%s",
		title,
		usernameLabel,
		m.usernameInput.View(),
		passwordLabel,
		m.passwordInput.View(),
		button,
		help,
	)
}

func (m model) authenticatingView() string {
	vb := ui.NewViewBuilder()
	return vb.RenderLoadingView("Plex Music Player - Authenticating...")
}

func (m model) successView() string {
	vb := ui.NewViewBuilder()
	tokenPreview := m.token
	if len(tokenPreview) > 40 {
		tokenPreview = tokenPreview[:40] + "..."
	}
	return vb.RenderSuccessMessage("Authentication Successful!", fmt.Sprintf("Your Plex token: %s", tokenPreview))
}

func (m model) errorView() string {
	vb := ui.NewViewBuilder()
	return vb.RenderErrorMessage("Authentication Failed", m.err.Error())
}

func (m model) serverSelectionView() string {
	if len(m.servers) == 0 {
		vb := ui.NewViewBuilder()
		return vb.RenderLoadingView("Loading Servers...")
	}

	vb := ui.NewViewBuilder()

	// Check if there's a last selected server
	cfg, _ := loadConfig()
	var lastServerName string
	if cfg != nil {
		lastServerName = cfg.LastSelectedServer
	}

	// Build list of servers with last used indicator
	serverNames := make([]string, len(m.servers))
	for i, server := range m.servers {
		serverName := server.Name
		if lastServerName != "" && server.Name == lastServerName {
			serverName += " (last used)"
		}
		serverNames[i] = serverName
	}

	serverList := vb.RenderList(serverNames, m.selectedServer)
	help := "\n  Up/Down: Navigate • Enter: Select • Ctrl+C: Quit\n"
	return vb.RenderFrame("Select Plex Server", serverList, help)
}

func (m model) playlistsContentView() string {
	if len(m.playlists) == 0 {
		return titleStyle.Render("Playlists") + "\n\nLoading..."
	}

	title := titleStyle.Render("Playlists")
	var playlistList string

	// Calculate max width for playlist info
	maxWidth := m.width - 8  // Account for padding/borders
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Scrolling window: show up to 15 playlists
	visibleCount := 15
	totalCount := len(m.playlists)

	// Calculate scroll offset to keep selected item visible
	startIdx := 0
	if totalCount > visibleCount {
		if m.selectedPlaylist >= visibleCount {
			startIdx = m.selectedPlaylist - visibleCount + 1
		}
		if startIdx < 0 {
			startIdx = 0
		}
	}

	endIdx := startIdx + visibleCount
	if endIdx > totalCount {
		endIdx = totalCount
	}

	for i := startIdx; i < endIdx; i++ {
		playlist := m.playlists[i]
		cursor := "  "
		playlistInfo := playlist.Title
		if playlist.LeafCount > 0 {
			playlistInfo += fmt.Sprintf(" (%d)", playlist.LeafCount)
		}

		// Truncate if too long
		if len(playlistInfo) > maxWidth {
			playlistInfo = playlistInfo[:maxWidth-3] + "..."
		}

		if i == m.selectedPlaylist {
			cursor = focusedStyle.Render("> ")
			playlistList += fmt.Sprintf("%s%s\n", cursor, focusedStyle.Render(playlistInfo))
		} else {
			playlistList += fmt.Sprintf("%s%s\n", cursor, blurredStyle.Render(playlistInfo))
		}
	}

	// Show scroll indicators
	if totalCount > visibleCount {
		showing := fmt.Sprintf("Showing %d-%d of %d", startIdx+1, endIdx, totalCount)
		playlistList += blurredStyle.Render("\n" + showing)
	}

	return fmt.Sprintf("%s\n\n%s", title, playlistList)
}

// Common function to render a track list view
func (m *model) renderTrackListView(title string, tracks []track, selectedTrack int, showArtist bool) string {
	if len(tracks) == 0 {
		return titleStyle.Render(title) + "\n\nLoading..."
	}

	titleText := titleStyle.Render(title)
	var output strings.Builder

	// Get album art if available from cache
	var albumArt string
	if len(tracks) > 0 {
		thumbURL := tracks[0].Thumb
		
		// Calculate album art size (consistent whether we have art or not)
		detailWidth := m.width - 8  // Account for padding/borders
		artWidth := detailWidth - 4 // Leave some padding
		if artWidth > 60 {
			artWidth = 60 // Cap at 60 for reasonable quality (reduced from 80)
		}
		if artWidth < 30 {
			artWidth = 30 // Minimum size (reduced from 40)
		}
		artHeight := artWidth / 2 // Maintain 2:1 ratio for square
		// Further cap height to prevent UI overflow
		if artHeight > 20 {
			artHeight = 20 // Max 20 lines for album art
		}

		// Render album art from cache or placeholder
		if m.currentAlbumArt != nil && m.currentAlbumArtThumb == thumbURL && detailWidth >= 50 {
			albumArt = m.imgRenderer.Render(m.currentAlbumArt, artWidth, artHeight)
		} else if detailWidth >= 50 {
			// Render placeholder at same size as album art would be
			albumArt = m.imgRenderer.RenderPlaceholder(artWidth, artHeight, "Loading...")
		}
	}

	// Add title
	output.WriteString(titleText)
	output.WriteString("\n\n")

	// Add album art if available
	if albumArt != "" {
		output.WriteString(albumArt)
		output.WriteString("\n")
	}

	// Calculate max width for track info
	// Account for cursor (2 chars) and some padding
	maxWidth := m.width - 8  // Account for padding/borders
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Scrolling window: show up to 15 tracks (or fewer if we have album art)
	visibleCount := 15
	if albumArt != "" {
		visibleCount = 6 // Reduce to make room for album art (reduced from 10)
	}
	totalCount := len(tracks)

	// Calculate scroll offset to keep selected item visible
	startIdx := 0
	if totalCount > visibleCount {
		if selectedTrack >= visibleCount {
			startIdx = selectedTrack - visibleCount + 1
		}
		if startIdx < 0 {
			startIdx = 0
		}
	}

	endIdx := startIdx + visibleCount
	if endIdx > totalCount {
		endIdx = totalCount
	}

	for i := startIdx; i < endIdx; i++ {
		track := tracks[i]
		cursor := "  "

		// Format duration
		durationMin := track.Duration / 60000 // Convert ms to minutes
		durationSec := (track.Duration % 60000) / 1000

		var trackInfo string
		if showArtist {
			// Format: "Position. Title - Artist (Duration)"
			artistName := track.Artist
			if artistName == "" {
				artistName = "Unknown Artist"
			}
			trackInfo = fmt.Sprintf("%d. %s - %s (%d:%02d)",
				i+1, track.Title, artistName, durationMin, durationSec)
		} else {
			// Format: "Position. Title (Duration)" - no artist
			trackInfo = fmt.Sprintf("%d. %s (%d:%02d)",
				i+1, track.Title, durationMin, durationSec)
		}

		// Truncate if too long
		if len(trackInfo) > maxWidth {
			trackInfo = trackInfo[:maxWidth-3] + "..."
		}

		if i == selectedTrack {
			cursor = focusedStyle.Render("> ")
			output.WriteString(fmt.Sprintf("%s%s\n", cursor, focusedStyle.Render(trackInfo)))
		} else {
			output.WriteString(fmt.Sprintf("%s%s\n", cursor, blurredStyle.Render(trackInfo)))
		}
	}

	// Show scroll indicators
	if totalCount > visibleCount {
		showing := fmt.Sprintf("Showing %d-%d of %d", startIdx+1, endIdx, totalCount)
		output.WriteString(blurredStyle.Render("\n" + showing))
	}

	// Add help text for queue operations
	help := blurredStyle.Render("\n  Enter: Play • a: Add to Queue")
	output.WriteString(help)

	return output.String()
}

func (m *model) playlistTracksContentView() string {
	return m.renderTrackListView("Playlist Tracks", m.tracks, m.selectedTrack, true)
}

func (m *model) albumTracksContentView() string {
	return m.renderTrackListView("Album Tracks", m.tracks, m.selectedTrack, false)
}

func (m model) searchContentView() string {
	title := titleStyle.Render("Search")
	content := blurredStyle.Render("\nComing soon...")
	return fmt.Sprintf("%s\n%s", title, content)
}

func (m model) settingsContentView() string {
	title := titleStyle.Render("Settings")
	content := blurredStyle.Render("\nComing soon...")
	return fmt.Sprintf("%s\n%s", title, content)
}

// ========== NEW TAB-BASED VIEW FUNCTIONS (Phase 3) ==========

// renderTabBar renders the horizontal tab navigation bar
func (m model) renderTabBar() string {
	tabs := []struct {
		tab   tabType
		label string
	}{
		{homeTab, "Home"},
		{libraryTab, "Library"},
		{playlistsTab, "Playlists"},
		{searchTab, "Search"},
		{queueTab, fmt.Sprintf("Queue (%d)", len(m.queue))},
		{settingsTab, "Settings"},
	}

	var tabItems []string
	for _, t := range tabs {
		var rendered string
		if t.tab == m.activeTab {
			// Active tab - highlighted
			rendered = focusedStyle.Render(fmt.Sprintf(" [%s] ", t.label))
		} else {
			// Inactive tab
			rendered = blurredStyle.Render(fmt.Sprintf("  %s  ", t.label))
		}
		tabItems = append(tabItems, rendered)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabItems...)
}

// renderHomeContent renders the home tab content (recently added albums)
func (m model) renderHomeContent() string {
	// Show recently added albums as the home view
	return m.recentlyAddedContentView()
}

// renderLibraryContent renders the library tab content (albums)
func (m model) renderLibraryContent() string {
	// Check if we're viewing album tracks or the album list
	if m.currentContent == albumTracksContent {
		// Show tracks from selected album
		if len(m.albums) > 0 && m.selectedAlbum < len(m.albums) {
			albumTitle := fmt.Sprintf("%s - %s", m.albums[m.selectedAlbum].Artist, m.albums[m.selectedAlbum].Title)
			return m.renderTrackListView(albumTitle, m.tracks, m.selectedTrack, false)
		}
		return titleStyle.Render("Album Tracks") + "\n\nNo tracks available."
	}
	// Show album list (recently added)
	return m.recentlyAddedContentView()
}

// renderPlaylistsTabContent renders the playlists tab content
func (m model) renderPlaylistsTabContent() string {
	// Check if we're viewing playlist tracks or the playlist list
	if m.currentContent == playlistTracksContent {
		// Show tracks from selected playlist
		if len(m.playlists) > 0 && m.selectedPlaylist < len(m.playlists) {
			playlistTitle := m.playlists[m.selectedPlaylist].Title
			return m.renderTrackListView(playlistTitle, m.tracks, m.selectedTrack, true)
		}
		return titleStyle.Render("Playlist Tracks") + "\n\nNo tracks available."
	}
	// Show playlist list
	return m.playlistsContentView()
}

// renderSearchContent renders the search tab content
func (m model) renderSearchContent() string {
	// Placeholder - will be implemented in Phase 6
	return m.searchContentView()
}

// renderQueueContent renders the queue tab content
func (m model) renderQueueContent() string {
	if len(m.queue) == 0 {
		emptyMsg := blurredStyle.Render("No tracks in queue.\n\nTip: Press 'a' while viewing tracks to add them to the queue.")
		return titleStyle.Render("Queue") + "\n\n" + emptyMsg
	}

	title := titleStyle.Render(fmt.Sprintf("Queue (%d tracks)", len(m.queue)))
	var trackList string

	for i, track := range m.queue {
		cursor := "  "
		trackInfo := fmt.Sprintf("%d. %s - %s", i+1, track.Title, track.Artist)
		
		if i == m.queueIndex {
			cursor = focusedStyle.Render("> ")
			trackList += cursor + focusedStyle.Render(trackInfo) + "\n"
		} else {
			trackList += cursor + blurredStyle.Render(trackInfo) + "\n"
		}
	}

	// Add helpful keyboard shortcuts hint
	help := blurredStyle.Render("\n  Enter: Play • d: Remove • j/k: Move Down/Up • c: Clear All")

	return fmt.Sprintf("%s\n\n%s%s", title, trackList, help)
}

// renderSettingsTabContent renders the settings tab content
func (m model) renderSettingsTabContent() string {
	// Placeholder - will be implemented in Phase 6
	return m.settingsContentView()
}

// renderQueueModal renders the queue as a modal overlay
func (m model) renderQueueModal() string {
	if !m.showQueueModal {
		return ""
	}

	modalWidth := 60
	modalHeight := 20

	queueContent := m.renderQueueContent()
	
	modalStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Height(modalHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF8C00")).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(1).
		Align(lipgloss.Center)

	return modalStyle.Render(queueContent)
}

// overlayModal renders the modal on top of the base view
func (m model) overlayModal(baseView string) string {
	modal := m.renderQueueModal()
	
	// Split both views into lines
	baseLines := strings.Split(baseView, "\n")
	modalLines := strings.Split(modal, "\n")
	
	// Ensure base has enough lines
	for len(baseLines) < m.height {
		baseLines = append(baseLines, "")
	}
	
	// Calculate center position for modal
	modalHeight := len(modalLines)
	
	startRow := (m.height - modalHeight) / 2
	if startRow < 0 {
		startRow = 0
	}
	
	// For each modal line, we need to overlay it onto the base
	// We'll use lipgloss.PlaceHorizontal to center each line
	for i := 0; i < modalHeight && (startRow+i) < len(baseLines); i++ {
		rowIdx := startRow + i
		// Place the modal line centered in the available width
		baseLines[rowIdx] = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, modalLines[i])
	}
	
	return strings.Join(baseLines, "\n")
}

// ========== END NEW TAB-BASED VIEW FUNCTIONS ==========

func (m model) mainAppView() string {
	// ========== NEW TAB-BASED LAYOUT (Phase 4) ==========
	
	// Render tab bar
	tabBar := m.renderTabBar()
	
	// Render content based on active tab
	var mainContent string
	switch m.activeTab {
	case homeTab:
		mainContent = m.renderHomeContent()
	case libraryTab:
		mainContent = m.renderLibraryContent()
	case playlistsTab:
		mainContent = m.renderPlaylistsTabContent()
	case searchTab:
		mainContent = m.renderSearchContent()
	case queueTab:
		mainContent = m.renderQueueContent()
	case settingsTab:
		mainContent = m.renderSettingsTabContent()
	default:
		mainContent = m.renderHomeContent()
	}
	
	// Render playback control pane
	playbackControl := m.playbackControlPane()
	
	// Calculate heights
	playbackHeight := 28
	tabBarHeight := 3 // Tab bar + borders
	
	help := blurredStyle.Render("\nTab/Left/Right: Switch Tabs • Q: Queue • Space/P: Play/Pause • S: Stop • N: Next • B: Previous • Esc: Back • Ctrl+C: Quit\n")
	helpHeight := strings.Count(help, "\n")
	
	reservedHeight := 1 + tabBarHeight + playbackHeight + helpHeight
	contentHeight := m.height - reservedHeight
	if contentHeight < 20 {
		contentHeight = 20
	}
	
	// Style for main content area
	contentStyle := lipgloss.NewStyle().
		Width(m.width - 4).
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444444")).
		Padding(1)
	
	contentRendered := contentStyle.Render(mainContent)
	
	// Build the base view
	baseView := "\n" + tabBar + "\n" + contentRendered + playbackControl + help
	
	// If queue modal is shown, overlay it on top of the base view
	if m.showQueueModal {
		return m.overlayModal(baseView)
	}
	
	return baseView
}
func (m model) recentlyAddedContentView() string {
	if len(m.albums) == 0 {
		return titleStyle.Render("Recently Added") + "\n\nLoading..."
	}

	title := titleStyle.Render("Recently Added")
	var albumList string

	// Calculate max width for album info
	maxWidth := m.width - 8  // Account for padding/borders
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Scrolling window: show up to 15 albums
	visibleCount := 15
	totalCount := len(m.albums)

	// Calculate scroll offset to keep selected item visible
	startIdx := 0
	if totalCount > visibleCount {
		if m.selectedAlbum >= visibleCount {
			startIdx = m.selectedAlbum - visibleCount + 1
		}
		if startIdx < 0 {
			startIdx = 0
		}
	}

	endIdx := startIdx + visibleCount
	if endIdx > totalCount {
		endIdx = totalCount
	}

	for i := startIdx; i < endIdx; i++ {
		album := m.albums[i]
		cursor := "  "
		albumInfo := fmt.Sprintf("%s - %s", album.Artist, album.Title)
		if album.Year > 0 {
			albumInfo += fmt.Sprintf(" (%d)", album.Year)
		}

		// Truncate if too long
		if len(albumInfo) > maxWidth {
			albumInfo = albumInfo[:maxWidth-3] + "..."
		}

		if i == m.selectedAlbum {
			cursor = focusedStyle.Render("> ")
			albumList += fmt.Sprintf("%s%s\n", cursor, focusedStyle.Render(albumInfo))
		} else {
			albumList += fmt.Sprintf("%s%s\n", cursor, blurredStyle.Render(albumInfo))
		}
	}

	// Show scroll indicators
	if totalCount > visibleCount {
		showing := fmt.Sprintf("Showing %d-%d of %d", startIdx+1, endIdx, totalCount)
		albumList += blurredStyle.Render("\n" + showing)
	}

	return fmt.Sprintf("%s\n\n%s", title, albumList)
}
// renderVolumeBar creates a visual volume indicator
// Returns a string like "Volume: ████░░░░░░ 50%"
func (m model) renderVolumeBar(width int) string {
	if m.volume == nil {
		return ""
	}

	// Clamp width to reasonable range
	if width < 10 {
		width = 10
	}
	if width > 30 {
		width = 30
	}

	volumePercent := m.volume.Volume * 100
	filledWidth := int(float64(width) * m.volume.Volume)
	if filledWidth > width {
		filledWidth = width
	}

	bar := ""
	for i := 0; i < width; i++ {
		if i < filledWidth {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	return fmt.Sprintf("Volume: %s %.0f%%", bar, volumePercent)
}

// playbackControlPane renders the bottom playback control pane
func (m *model) playbackControlPane() string {
	// Fixed height for playback pane to prevent UI bumping
	const playbackPaneHeight = 28
	
	var content string
	
	if m.currentTrack == nil || m.playbackState == playbackStopped {
		// Show "Nothing Playing" message when no active playback
		nothingPlaying := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Render("♫ Nothing Playing")
		hint := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Render("Select a track and press Enter to start playback")

		// Simple message for no playback state
		content = fmt.Sprintf("\n%s\n%s", nothingPlaying, hint)
	} else {
		// Generate full playback UI
		content = m.renderActivePlayback()
	}
	
	// Manually pad content to fixed height (lipgloss Height doesn't work with graphics protocols)
	contentLines := strings.Split(content, "\n")
	currentHeight := len(contentLines)
	
	// Add empty lines to reach target height
	if currentHeight < playbackPaneHeight {
		paddingNeeded := playbackPaneHeight - currentHeight
		for i := 0; i < paddingNeeded; i++ {
			content += "\n"
		}
	}
	
	return content
}

// renderActivePlayback renders the playback UI when a track is playing
func (m *model) renderActivePlayback() string {
	// Render album art or placeholder - always show at consistent size
	// NOTE: For playback pane, we force Unicode blocks to avoid issues with
	// frequent re-renders clearing terminal graphics (Kitty/iTerm2/Sixel)
	var albumArt string
	var artWidth int
	// Calculate art dimensions (same logic whether we have art or not)
	if m.width >= 100 {
		artWidth = m.width / 5 // Use ~20% of screen width
		if artWidth > 40 {
			artWidth = 40 // Cap at 40 for playback pane
		}
		if artWidth < 20 {
			artWidth = 20 // Minimum size
		}
		// Ensure artWidth is even for proper 2:1 ratio
		if artWidth%2 != 0 {
			artWidth--
		}
	} else {
		artWidth = 20 // Minimum size for small terminals
	}

	artHeight := artWidth / 2 // Maintain 2:1 ratio for square

	if m.playbackAlbumArt != nil && m.playbackAlbumArtThumb == m.currentTrack.Thumb && m.width >= 100 {
		// Force Unicode blocks for playback pane to avoid re-render issues
		albumArt = m.playbackImgRenderer.Render(m.playbackAlbumArt, artWidth, artHeight)
	} else if m.width >= 100 {
		// Render placeholder at same size as album art would be
		albumArt = m.playbackImgRenderer.RenderPlaceholder(artWidth, artHeight, "No Cover Art")
	}

	// Status icon
	statusIcon := "▶"
	if m.playbackState == playbackPaused {
		statusIcon = "⏸"
	}

	// Track info
	trackInfo := fmt.Sprintf("%s - %s", m.currentTrack.Artist, m.currentTrack.Title)
	if m.currentTrack.Album != "" {
		trackInfo += fmt.Sprintf(" [%s]", m.currentTrack.Album)
	}

	// Calculate current position and total duration
	var elapsed, total time.Duration
	var progressPercent float64

	// Get total duration from track metadata (in milliseconds)
	if m.currentTrack.Duration > 0 {
		total = time.Duration(m.currentTrack.Duration) * time.Millisecond
	}

	if m.streamer != nil && m.sampleRate > 0 {
		// Get current position
		speaker.Lock()
		currentPos := m.streamer.Position()
		speaker.Unlock()

		// Calculate elapsed time
		elapsed = m.sampleRate.D(currentPos)

		// Calculate progress percentage using track duration
		if total > 0 {
			progressPercent = float64(elapsed) / float64(total)
		}
	}

	// Format time as MM:SS
	formatTime := func(d time.Duration) string {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%d:%02d", minutes, seconds)
	}

	elapsedStr := formatTime(elapsed)
	totalStr := formatTime(total)

	// Create progress bar
	availableWidth := m.width
	if albumArt != "" {
		// Reserve space for album art (dynamic width + 2 padding)
		availableWidth -= artWidth + 2
	}

	maxWidth := availableWidth - len(trackInfo) - len(elapsedStr) - len(totalStr) - 20
	if maxWidth < 20 {
		maxWidth = 20
	}

	progressBarWidth := maxWidth
	filledWidth := int(float64(progressBarWidth) * progressPercent)
	if filledWidth > progressBarWidth {
		filledWidth = progressBarWidth
	}

	progressBar := ""
	for i := 0; i < progressBarWidth; i++ {
		if i < filledWidth {
			progressBar += "█"
		} else {
			progressBar += "░"
		}
	}

	// Controls hint
	controlsHint := "[B]ack [S]top "
	if m.playbackState == playbackPlaying {
		controlsHint += "[Space]Pause"
	} else {
		controlsHint += "[Space]Play"
	}
	controlsHint += " [N]ext [-/+]Volume"

	// Volume indicator
	volumeDisplay := m.renderVolumeBar(15)

	// Build the display
	line1 := fmt.Sprintf("%s %s", statusIcon, trackInfo)
	line2 := fmt.Sprintf("%s %s %s / %s", elapsedStr, progressBar, formatTime(elapsed), totalStr)
	line3 := controlsHint
	line4 := volumeDisplay

	// Truncate track info if too long
	maxTrackInfoWidth := availableWidth - 5
	if maxTrackInfoWidth < 40 {
		maxTrackInfoWidth = 40
	}
	if len(line1) > maxTrackInfoWidth {
		line1 = line1[:maxTrackInfoWidth-3] + "..."
	}

	// If we have album art, layout side-by-side
	if albumArt != "" {
		albumArtLines := strings.Split(strings.TrimSuffix(albumArt, "\n"), "\n")
		textLines := []string{
			"",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00")).Bold(true).Render(line1),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Render(line2),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(line3),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#00AA00")).Render(line4),
			"",
		}

		// Combine album art and text side by side
		var combined []string
		maxLines := len(albumArtLines)
		if len(textLines) > maxLines {
			maxLines = len(textLines)
		}

		for i := 0; i < maxLines; i++ {
			artPart := ""
			textPart := ""

			if i < len(albumArtLines) {
				artPart = albumArtLines[i]
			} else {
				artPart = strings.Repeat(" ", artWidth) // Maintain spacing (dynamic width)
			}

			if i < len(textLines) {
				textPart = textLines[i]
			}

			combined = append(combined, artPart+"  "+textPart)
		}

		return "\n" + strings.Join(combined, "\n")
	}

	result := fmt.Sprintf("\n%s\n%s\n%s\n%s",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00")).Bold(true).Render(line1),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Render(line2),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(line3),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#00AA00")).Render(line4))

	return result
}

func main() {
	// Try to load existing config
	cfg, err := loadConfig()
	var initialState model

	if err == nil && cfg.AuthToken != "" {
		// Token exists, skip login and go to server selection
		initialState = model{
			state:         serverSelectionView,
			token:         cfg.AuthToken,
			authenticator: auth.NewAuthenticator(),
			coordinator:   app.NewCoordinator(),
		}
		// Initialize renderers for fast-path
		initialState.imgRenderer = termimg.NewRenderer()
		initialState.playbackImgRenderer = termimg.NewRendererWithProtocol(termimg.ProtocolUnicodeBlocks)
	} else {
		// No token, show login
		initialState = initialModel()
	}

	p := tea.NewProgram(
		initialState,
		tea.WithAltScreen(),       // Use alternate screen buffer to prevent scrolling
		tea.WithMouseCellMotion(), // Enable mouse support (optional)
	)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
