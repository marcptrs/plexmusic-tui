package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config represents the application configuration
type Config struct {
	AuthToken          string  `json:"authToken"`
	LastSelectedServer string  `json:"lastSelectedServer,omitempty"` // Server canonical key in the form host/name
	Volume             float64 `json:"volume,omitempty"`             // Audio volume "stops" (logarithmic scale, Base:2).
	// 0 = 100%, 1 = 200%, -1 = 50%
	// CoverArtPosition determines where the cover art is rendered relative to
	// playlists/queue. Valid values: "left" or "right". Defaults to "left".
	CoverArtPosition string `json:"coverArtPosition,omitempty"`
}

// Manager wraps Config with convenience methods
type Manager struct {
	cfg *Config
}

// NewManager creates a new config manager
func NewManager() (*Manager, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	return &Manager{cfg: cfg}, nil
}

// GetAuthToken returns the stored auth token
func (m *Manager) GetAuthToken() string {
	return m.cfg.AuthToken
}

// SetAuthToken sets the auth token
func (m *Manager) SetAuthToken(token string) {
	m.cfg.AuthToken = token
}

// GetLastSelectedServer returns the last selected server canonical key in the form host/name
func (m *Manager) GetLastSelectedServer() string {
	return m.cfg.LastSelectedServer
}

// SetLastSelectedServer sets the last selected server canonical key in the form host/name
func (m *Manager) SetLastSelectedServer(serverName string) {
	m.cfg.LastSelectedServer = serverName
}

// GetVolume returns the stored volume level, defaulting to 0.0 if not previously set.
// Since Volume is a float64 with omitempty tag, unset values will be 0.0 from JSON unmarshal.
// Volume uses a logarithmic scale where 0 = 100% (no change), with Base:2 in the beep library.
// Positive values increase volume (1 = 200%), negative values decrease (e.g., -1 = 50%).
func (m *Manager) GetVolume() float64 {
	// If Volume is 0.0, it wasn't explicitly set in the config file, return default of 0 (100%)
	if m.cfg.Volume == 0 {
		return 0.0 // Default to 0 (100% display)
	}
	return m.cfg.Volume
}

// GetCoverArtPosition returns the configured position of the cover art.
// Valid values are "left" or "right". If unset, default to "left".
func (m *Manager) GetCoverArtPosition() string {
	if m.cfg.CoverArtPosition == "right" {
		return "right"
	}
	return "left"
}

// SetCoverArtPosition sets the cover art position. Use "left" or "right".
func (m *Manager) SetCoverArtPosition(pos string) {
	if pos != "right" {
		pos = "left"
	}
	m.cfg.CoverArtPosition = pos
}

// SetVolume sets the volume level
func (m *Manager) SetVolume(volume float64) {
	m.cfg.Volume = volume
}

// Save saves the configuration to disk
func (m *Manager) Save() error {
	return Save(m.cfg)
}

// GetConfigPath returns the path to the config file
func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(homeDir, ".config", "plexmusic-tui")
	return filepath.Join(configDir, "config.json"), nil
}

// Load loads the configuration from disk
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save saves the configuration to disk
func Save(cfg *Config) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o600)
}
