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

	"plexmusic-tui/internal/auth"
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
	homeView
	recentlyAddedView
	playlistsView
	searchView
	settingsView
	librarySelectionView
	albumListView
	mainAppView // New unified view with panes
)

type paneType int

const (
	navigationPane paneType = iota
	contentPane
	detailPane
)

type contentViewType int

const (
	recentlyAddedContent contentViewType = iota
	albumTracksContent
	playlistsContent
	playlistTracksContent
	searchContent
	settingsContent
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

	// Multi-pane UI state
	focusedPane    paneType
	currentContent contentViewType
	navMenuIndex   int // Index in navigation menu

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

	return model{
		state:         loginView,
		usernameInput: usernameInput,
		passwordInput: passwordInput,
		focusIndex:    0,
		authenticator: auth.NewAuthenticator(),
	}
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
				// Handle back navigation within panes
				if m.currentContent == playlistTracksContent {
					// Go back from tracks to playlists
					m.currentContent = playlistsContent
					m.tracks = nil // Clear tracks
					m.selectedTrack = 0
					m.focusedPane = contentPane
					return m, nil
				} else if m.currentContent == albumTracksContent {
					// Go back from tracks to albums
					m.currentContent = recentlyAddedContent
					m.tracks = nil // Clear tracks
					m.selectedTrack = 0
					m.focusedPane = contentPane
					return m, nil
				}
				// From main app, go back to server selection
				m.state = serverSelectionView
			case homeView:
				m.state = serverSelectionView
			case recentlyAddedView, playlistsView, searchView, settingsView:
				m.state = homeView
			case librarySelectionView:
				m.state = homeView
			case albumListView:
				m.state = librarySelectionView
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
					// Cycle through panes
					if s == "tab" {
						// Forward: nav -> content -> detail -> nav
						if m.focusedPane == navigationPane {
							m.focusedPane = contentPane
						} else if m.focusedPane == contentPane {
							m.focusedPane = detailPane
						} else {
							m.focusedPane = navigationPane
						}
					} else {
						// Backward: nav -> detail -> content -> nav
						if m.focusedPane == navigationPane {
							m.focusedPane = detailPane
						} else if m.focusedPane == detailPane {
							m.focusedPane = contentPane
						} else {
							m.focusedPane = navigationPane
						}
					}
				} else if s == "up" || s == "down" {
					// Navigate within panes
					if m.focusedPane == navigationPane {
						// Navigate menu
						if s == "up" {
							m.navMenuIndex--
							if m.navMenuIndex < 0 {
								m.navMenuIndex = 3 // 4 menu items (0-3)
							}
						} else {
							m.navMenuIndex++
							if m.navMenuIndex > 3 {
								m.navMenuIndex = 0
							}
						}
					} else if m.focusedPane == contentPane {
						// Navigate content pane (playlists, albums, etc.)
						if m.currentContent == recentlyAddedContent {
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
						} else if m.currentContent == playlistsContent || m.currentContent == playlistTracksContent {
							// Navigate playlists (even when viewing tracks)
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
						}
					} else if m.focusedPane == detailPane {
						// Navigate detail pane (tracks)
						if m.currentContent == playlistTracksContent || m.currentContent == albumTracksContent {
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
			} else if m.state == homeView {
				s := msg.String()
				if s == "up" || s == "shift+tab" {
					m.selectedHome--
					if m.selectedHome < 0 {
						m.selectedHome = 4 // 5 menu items (0-4)
					}
				} else {
					m.selectedHome++
					if m.selectedHome > 4 {
						m.selectedHome = 0
					}
				}
			} else if m.state == librarySelectionView {
				s := msg.String()
				if s == "up" || s == "shift+tab" {
					m.selectedLibrary--
					if m.selectedLibrary < 0 {
						m.selectedLibrary = len(m.libraries) - 1
					}
				} else {
					m.selectedLibrary++
					if m.selectedLibrary >= len(m.libraries) {
						m.selectedLibrary = 0
					}
				}
			} else if m.state == albumListView || m.state == recentlyAddedView {
				s := msg.String()
				if s == "up" || s == "shift+tab" {
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
			} else if m.state == playlistsView {
				s := msg.String()
				if s == "up" || s == "shift+tab" {
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
				m.focusedPane = navigationPane
				m.currentContent = recentlyAddedContent
				m.navMenuIndex = 0
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
				if m.focusedPane == navigationPane {
					// Switch content based on navigation menu selection
					switch m.navMenuIndex {
					case 0: // Recently Added
						m.currentContent = recentlyAddedContent
						m.selectedAlbum = 0
						if len(m.albums) == 0 {
							return m, m.fetchRecentlyAdded()
						}
					case 1: // Playlists
						m.currentContent = playlistsContent
						m.selectedPlaylist = 0
						if len(m.playlists) == 0 {
							return m, m.fetchPlaylists()
						}
					case 2: // Search
						m.currentContent = searchContent
					case 3: // Settings
						m.currentContent = settingsContent
					}
					// Switch focus to content pane after selection
					m.focusedPane = contentPane
				} else if m.focusedPane == contentPane {
					// Content pane - handle item selection
					if m.currentContent == recentlyAddedContent && len(m.albums) > 0 {
						// Switch to album tracks view and fetch tracks
						m.currentContent = albumTracksContent
						m.selectedTrack = 0
						m.focusedPane = detailPane // Move focus to detail pane
						return m, m.fetchAlbumTracks()
					} else if m.currentContent == playlistsContent && len(m.playlists) > 0 {
						// Switch to playlist tracks view and fetch tracks
						m.currentContent = playlistTracksContent
						m.selectedTrack = 0
						m.focusedPane = detailPane // Move focus to detail pane
						return m, m.fetchPlaylistTracks()
					}
				} else if m.focusedPane == detailPane {
					// Detail pane - play selected track
					if (m.currentContent == playlistTracksContent || m.currentContent == albumTracksContent) && len(m.tracks) > 0 {
						track := m.tracks[m.selectedTrack]
						m.currentTrack = &track
						return m, m.startPlayback(track)
					}
				}
			}
			if m.state == homeView {
				switch m.selectedHome {
				case 0: // Recently Added
					m.state = recentlyAddedView
					return m, m.fetchRecentlyAdded()
				case 1: // Playlists
					m.state = playlistsView
					m.selectedPlaylist = 0
					return m, m.fetchPlaylists()
				case 2: // Search
					m.state = searchView
					// TODO: Implement search UI
					return m, nil
				case 3: // Settings
					m.state = settingsView
					return m, nil
				case 4: // Exit
					return m, tea.Quit
				}
			}
			if m.state == librarySelectionView && len(m.libraries) > 0 {
				m.state = albumListView
				return m, m.fetchAlbums()
			}
			if m.state == albumListView && len(m.albums) > 0 {
				// TODO: Show album details/tracks
				return m, nil
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
							m.focusedPane = navigationPane
							m.currentContent = recentlyAddedContent
							m.navMenuIndex = 0
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

		// Start ticker to update position
		return m, tickCmd()

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
	return m.fetchTracks(playlist.Key, "playlist")
}

func (m model) fetchAlbumTracks() tea.Cmd {
	album := m.albums[m.selectedAlbum]
	return m.fetchTracks(album.Key, "album")
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

// imageProtocol represents the supported terminal image protocols
type imageProtocol int

const (
	protocolUnicodeBlocks imageProtocol = iota // Fallback using Unicode half-blocks
	protocolKitty                              // Kitty graphics protocol
	protocolITerm2                             // iTerm2 inline images
	protocolSixel                              // Sixel graphics
)

// detectImageProtocol detects which image protocol the terminal supports
func detectImageProtocol() imageProtocol {
	// Check for Kitty terminal
	if os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return protocolKitty
	}

	// Check for iTerm2
	if strings.Contains(os.Getenv("TERM_PROGRAM"), "iTerm") {
		return protocolITerm2
	}

	// Check for Sixel support via TERM environment variable
	term := os.Getenv("TERM")
	if strings.Contains(term, "sixel") || term == "mlterm" || term == "yaft-256color" {
		return protocolSixel
	}

	// Check for xterm with sixel support (some modern xterms)
	if strings.Contains(term, "xterm") {
		// Could query terminal capabilities here, but for simplicity
		// we'll default to Unicode blocks for xterm
		return protocolUnicodeBlocks
	}

	// Default to Unicode blocks for maximum compatibility
	return protocolUnicodeBlocks
}

// createPlaceholderImage creates a simple placeholder image for when no album art is available
func createPlaceholderImage(width, height int, text string) image.Image {
	// Create a simple gray image with a music note symbol
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with dark gray background
	gray := uint8(40)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, image.NewRGBA(image.Rect(0, 0, 1, 1)).At(0, 0))
			img.SetRGBA(x, y, struct{ R, G, B, A uint8 }{gray, gray, gray, 255})
		}
	}

	return img
}

// renderPlaceholder renders a text-based placeholder at a fixed size
func renderPlaceholder(width, height int, message string) string {
	return ui.RenderPlaceholder(width, height, message)
}

// renderImageKitty renders an image using the Kitty graphics protocol
func renderImageKitty(img image.Image, width, height int) string {
	return ui.RenderImageKitty(img, width, height)
}

// renderImageITerm2 renders an image using iTerm2's inline image protocol
func renderImageITerm2(img image.Image, width, height int) string {
	return ui.RenderImageITerm2(img, width, height)
}

// renderImageSixel renders an image using the Sixel protocol
func renderImageSixel(img image.Image, width, height int) string {
	return ui.RenderImageSixel(img, width, height)
}

// renderImageUnicodeBlocks renders an image using Unicode half-block characters (fallback)
func renderImageUnicodeBlocks(img image.Image, width, height int) string {
	return ui.RenderImageUnicodeBlocks(img, width, height)
}

// renderImageToTerminal converts an image to terminal output using the best available protocol
// Returns a string that can be displayed in the terminal
func renderImageToTerminal(img image.Image, width, height int) string {
	if img == nil {
		return ""
	}

	protocol := detectImageProtocol()

	switch protocol {
	case protocolKitty:
		return renderImageKitty(img, width, height)
	case protocolITerm2:
		return renderImageITerm2(img, width, height)
	case protocolSixel:
		return renderImageSixel(img, width, height)
	default:
		return renderImageUnicodeBlocks(img, width, height)
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
	case homeView:
		return m.homeView()
	case recentlyAddedView:
		return m.recentlyAddedView()
	case playlistsView:
		return m.playlistsView()
	case searchView:
		return m.searchView()
	case settingsView:
		return m.settingsView()
	case librarySelectionView:
		return m.librarySelectionView()
	case albumListView:
		return m.albumListView()
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

func (m model) homeView() string {
	vb := ui.NewViewBuilder()

	menuItems := []string{
		"Recently Added",
		"Playlists",
		"Search",
		"Settings",
		"Exit",
	}

	menu := vb.RenderList(menuItems, m.selectedHome)
	help := "\n  Up/Down: Navigate • Enter: Select • Esc: Back • Ctrl+C: Quit\n"
	title := fmt.Sprintf("Plex Music Player - %s", m.servers[m.selectedServer].Name)
	return vb.RenderFrame(title, menu, help)
}

func (m model) recentlyAddedView() string {
	if len(m.albums) == 0 {
		vb := ui.NewViewBuilder()
		return vb.RenderLoadingView("Loading Recently Added...")
	}

	title := titleStyle.Render("Recently Added Albums")
	var albumList string

	// Calculate max width for album info
	maxWidth := m.getContentPaneWidth() - 4
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Scrolling window: show up to 15 albums
	visibleCount := 15
	totalCount := len(m.albums)

	// Calculate scroll offset to keep selected item visible
	startIdx := 0
	if totalCount > visibleCount {
		// Keep selected item in view with some context
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

		if i == m.selectedAlbum && m.focusedPane == contentPane {
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

func (m model) playlistsContentView() string {
	if len(m.playlists) == 0 {
		return titleStyle.Render("Playlists") + "\n\nLoading..."
	}

	title := titleStyle.Render("Playlists")
	var playlistList string

	// Calculate max width for playlist info
	maxWidth := m.getContentPaneWidth() - 4
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

		if i == m.selectedPlaylist && m.focusedPane == contentPane {
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

	// Get album art if available and not already cached
	var albumArt string
	if len(tracks) > 0 {
		thumbURL := tracks[0].Thumb
		// Check if we need to fetch/render the album art
		if thumbURL != "" && thumbURL != m.currentAlbumArtThumb {
			// Fetch and cache the album art
			if img, err := m.fetchAlbumArt(thumbURL); err == nil {
				m.currentAlbumArt = img
				m.currentAlbumArtThumb = thumbURL
			}
		}

		// Calculate album art size (consistent whether we have art or not)
		detailWidth := m.getDetailPaneWidth()
		artWidth := detailWidth - 4 // Leave some padding
		if artWidth > 80 {
			artWidth = 80 // Cap at 80 for reasonable quality
		}
		if artWidth < 40 {
			artWidth = 40 // Minimum size
		}
		artHeight := artWidth / 2 // Maintain 2:1 ratio for square

		// Render album art or placeholder - always at consistent size
		if m.currentAlbumArt != nil && m.currentAlbumArtThumb == thumbURL && detailWidth >= 50 {
			albumArt = renderImageToTerminal(m.currentAlbumArt, artWidth, artHeight)
		} else if detailWidth >= 50 {
			// Render placeholder at same size as album art would be
			albumArt = renderPlaceholder(artWidth, artHeight, "Loading...")
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
	maxWidth := m.getDetailPaneWidth() - 4
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Scrolling window: show up to 15 tracks (or fewer if we have album art)
	visibleCount := 15
	if albumArt != "" {
		visibleCount = 10 // Reduce to make room for album art
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

		if i == selectedTrack && m.focusedPane == detailPane {
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

// Helper methods to calculate pane widths
func (m model) getNavPaneWidth() int {
	totalWidth := m.width
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	navWidth := usableWidth * 20 / 100
	if navWidth < 20 {
		navWidth = 20
	}
	return navWidth
}

func (m model) getContentPaneWidth() int {
	totalWidth := m.width
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	contentWidth := usableWidth * 30 / 100
	if contentWidth < 30 {
		contentWidth = 30
	}
	return contentWidth
}

func (m model) getDetailPaneWidth() int {
	totalWidth := m.width
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	detailWidth := usableWidth * 40 / 100
	if detailWidth < 40 {
		detailWidth = 40
	}
	return detailWidth
}

func (m model) mainAppView() string {
	navPaneContent := m.navigationPaneView()
	contentPaneContent := m.contentPaneView()
	detailPaneContent := m.detailPaneView()

	// Get pane widths using helper methods
	navWidth := m.getNavPaneWidth()
	contentWidth := m.getContentPaneWidth()
	detailWidth := m.getDetailPaneWidth()

	// Render playback control pane first to know its actual height
	playbackControl := m.playbackControlPane()

	// Reserve a fixed height for playback pane to prevent bumping
	// Max album art is 20 lines tall (40 chars / 2), plus spacing
	// Add extra buffer to be safe - always reserve this even when nothing is playing
	playbackHeight := 28

	// Render help text to count its lines too
	help := blurredStyle.Render("\nSpace/P: Play/Pause • S: Stop • N: Next • B: Previous • Tab: Switch Pane • Up/Down: Navigate • Enter: Select • Esc: Back • Ctrl+C: Quit\n")
	helpHeight := strings.Count(help, "\n")

	// Calculate dynamic height - reserve space for all non-pane content
	// +1 for the initial "\n" in the return statement
	reservedHeight := 1 + playbackHeight + helpHeight

	paneHeight := m.height - reservedHeight
	if paneHeight < 20 {
		paneHeight = 20
	}

	// Create layout styles with dynamic sizes
	navStyle := lipgloss.NewStyle().
		Width(navWidth).
		Height(paneHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444444"))

	contentStyle := lipgloss.NewStyle().
		Width(contentWidth).
		Height(paneHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444444"))

	detailStyle := lipgloss.NewStyle().
		Width(detailWidth).
		Height(paneHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444444"))

	// Highlight focused pane
	focusedColor := lipgloss.Color("#FF8C00")

	if m.focusedPane == navigationPane {
		navStyle = navStyle.BorderForeground(focusedColor)
	} else if m.focusedPane == contentPane {
		contentStyle = contentStyle.BorderForeground(focusedColor)
	} else if m.focusedPane == detailPane {
		detailStyle = detailStyle.BorderForeground(focusedColor)
	}

	// Render panes with styles
	navRendered := navStyle.Render(navPaneContent)
	contentRendered := contentStyle.Render(contentPaneContent)
	detailRendered := detailStyle.Render(detailPaneContent)

	// Join panes side by side
	combined := lipgloss.JoinHorizontal(lipgloss.Top, navRendered, contentRendered, detailRendered)

	return "\n" + combined + playbackControl + help
}

func (m model) navigationPaneView() string {
	title := titleStyle.Render("Menu")
	menuItems := []string{
		"Recently Added",
		"Playlists",
		"Search",
		"Settings",
	}

	var menu string
	for i, item := range menuItems {
		cursor := "  "
		if i == m.navMenuIndex {
			if m.focusedPane == navigationPane {
				cursor = focusedStyle.Render("> ")
				menu += fmt.Sprintf("%s%s\n", cursor, focusedStyle.Render(item))
			} else {
				cursor = "> "
				menu += fmt.Sprintf("%s%s\n", cursor, item)
			}
		} else {
			menu += fmt.Sprintf("%s%s\n", cursor, blurredStyle.Render(item))
		}
	}

	return fmt.Sprintf("%s\n\n%s", title, menu)
}

func (m model) contentPaneView() string {
	switch m.currentContent {
	case recentlyAddedContent:
		return m.recentlyAddedContentView()
	case albumTracksContent:
		// Show album list while tracks are in detail pane
		return m.recentlyAddedContentView()
	case playlistsContent:
		return m.playlistsContentView()
	case playlistTracksContent:
		// Show playlists while tracks are in detail pane
		return m.playlistsContentView()
	case searchContent:
		return m.searchContentView()
	case settingsContent:
		return m.settingsContentView()
	default:
		return ""
	}
}

func (m model) recentlyAddedContentView() string {
	if len(m.albums) == 0 {
		return titleStyle.Render("Recently Added") + "\n\nLoading..."
	}

	title := titleStyle.Render("Recently Added")
	var albumList string

	// Calculate max width for album info
	maxWidth := m.getContentPaneWidth() - 4
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

		if i == m.selectedAlbum && m.focusedPane == contentPane {
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

func (m model) detailPaneView() string {
	switch m.currentContent {
	case playlistTracksContent:
		return m.playlistTracksContentView()
	case albumTracksContent:
		return m.albumTracksContentView()
	default:
		return titleStyle.Render("Details") + "\n\n" + blurredStyle.Render("Select an item to view details")
	}
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
	if m.currentTrack == nil || m.playbackState == playbackStopped {
		// Show "Nothing Playing" message when no active playback
		nothingPlaying := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Render("♫ Nothing Playing")
		hint := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Render("Select a track and press Enter to start playback")

		// Return simple message - height will be managed by reserving space
		return fmt.Sprintf("\n%s\n%s", nothingPlaying, hint)
	}

	// Fetch and cache album art if we don't have it or if track changed
	var albumArt string
	if m.currentTrack.Thumb != "" && m.playbackAlbumArtThumb != m.currentTrack.Thumb {
		if img, err := m.fetchAlbumArt(m.currentTrack.Thumb); err == nil {
			m.playbackAlbumArt = img
			m.playbackAlbumArtThumb = m.currentTrack.Thumb
		}
	}

	// Render album art or placeholder - always show at consistent size
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
		// Render actual album art
		albumArt = renderImageToTerminal(m.playbackAlbumArt, artWidth, artHeight)
	} else if m.width >= 100 {
		// Render placeholder at same size as album art would be
		albumArt = renderPlaceholder(artWidth, artHeight, "No Cover Art")
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

func (m model) playlistsView() string {
	if len(m.playlists) == 0 {
		title := titleStyle.Render("Loading Playlists...")
		return fmt.Sprintf("\n%s\n\n  Please wait...\n", title)
	}

	title := titleStyle.Render("Playlists")
	var playlistList string

	for i, playlist := range m.playlists {
		cursor := "  "
		playlistInfo := playlist.Title
		if playlist.LeafCount > 0 {
			playlistInfo += fmt.Sprintf(" (%d)", playlist.LeafCount)
		}

		if i == m.selectedPlaylist {
			cursor = focusedStyle.Render("> ")
			playlistList += fmt.Sprintf("%s%s\n", cursor, focusedStyle.Render(playlistInfo))
		} else {
			playlistList += fmt.Sprintf("%s%s\n", cursor, blurredStyle.Render(playlistInfo))
		}
	}

	help := blurredStyle.Render("\n  Up/Down: Navigate • Enter: View Tracks • Esc: Back • Ctrl+C: Quit\n")

	return fmt.Sprintf("\n%s\n\n%s%s", title, playlistList, help)
}

func (m model) searchView() string {
	title := titleStyle.Render("Search")
	content := blurredStyle.Render("\n  Coming soon...\n")
	help := blurredStyle.Render("\n  Esc: Back • Ctrl+C: Quit\n")
	return fmt.Sprintf("\n%s%s%s", title, content, help)
}

func (m model) settingsView() string {
	title := titleStyle.Render("Settings")
	content := blurredStyle.Render("\n  Coming soon...\n")
	help := blurredStyle.Render("\n  Esc: Back • Ctrl+C: Quit\n")
	return fmt.Sprintf("\n%s%s%s", title, content, help)
}

func (m model) librarySelectionView() string {
	if len(m.libraries) == 0 {
		title := titleStyle.Render("Loading Libraries...")
		return fmt.Sprintf("\n%s\n\n  Please wait...\n", title)
	}

	title := titleStyle.Render("Select Music Library")
	var libraryList string

	for i, library := range m.libraries {
		cursor := "  "
		if i == m.selectedLibrary {
			cursor = focusedStyle.Render("> ")
			libraryList += fmt.Sprintf("%s%s\n", cursor, focusedStyle.Render(library.Title))
		} else {
			libraryList += fmt.Sprintf("%s%s\n", cursor, blurredStyle.Render(library.Title))
		}
	}

	help := blurredStyle.Render("\n  Up/Down: Navigate • Enter: Select • Esc: Back • Ctrl+C: Quit\n")

	return fmt.Sprintf("\n%s\n\n%s%s", title, libraryList, help)
}

func (m model) albumListView() string {
	if len(m.albums) == 0 {
		title := titleStyle.Render("Loading Albums...")
		return fmt.Sprintf("\n%s\n\n  Please wait...\n", title)
	}

	title := titleStyle.Render("Albums")
	var albumList string

	// Show up to 20 albums
	displayCount := len(m.albums)
	if displayCount > 20 {
		displayCount = 20
	}

	for i := 0; i < displayCount; i++ {
		album := m.albums[i]
		cursor := "  "
		albumInfo := fmt.Sprintf("%s - %s", album.Artist, album.Title)
		if album.Year > 0 {
			albumInfo += fmt.Sprintf(" (%d)", album.Year)
		}

		if i == m.selectedAlbum {
			cursor = focusedStyle.Render("> ")
			albumList += fmt.Sprintf("%s%s\n", cursor, focusedStyle.Render(albumInfo))
		} else {
			albumList += fmt.Sprintf("%s%s\n", cursor, blurredStyle.Render(albumInfo))
		}
	}

	if len(m.albums) > 20 {
		albumList += blurredStyle.Render(fmt.Sprintf("\n  ...and %d more albums\n", len(m.albums)-20))
	}

	help := blurredStyle.Render("\n  Up/Down: Navigate • Enter: View Tracks • Esc: Back • Ctrl+C: Quit\n")

	return fmt.Sprintf("\n%s\n\n%s%s", title, albumList, help)
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
		}
	} else {
		// No token, show login
		initialState = initialModel()
	}

	p := tea.NewProgram(initialState)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
