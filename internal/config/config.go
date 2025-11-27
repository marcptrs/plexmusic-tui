package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	AuthToken          string  `json:"authToken"                    mapstructure:"authToken"`
	LastSelectedServer string  `json:"lastSelectedServer,omitempty" mapstructure:"lastSelectedServer"`
	Volume             float64 `json:"volume,omitempty"             mapstructure:"volume"`
	// CoverArtPosition determines where the cover art is rendered relative to
	// playlists/queue. Valid values: "left" or "right". Defaults to "left".
	CoverArtPosition string `json:"coverArtPosition,omitempty"   mapstructure:"coverArtPosition"`
}

// Manager wraps Viper with convenience methods and maintains backward compatibility
type Manager struct {
	cfg  *Config
	v    *viper.Viper
	path string
}

// NewManager creates a new config manager using Viper
func NewManager() (*Manager, error) {
	v := viper.New()

	// Get config path
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	// Configure Viper
	v.SetConfigFile(configPath)
	v.SetConfigType("json")

	// Set defaults
	v.SetDefault("authToken", "")
	v.SetDefault("lastSelectedServer", "")
	v.SetDefault("volume", 0.0)
	v.SetDefault("coverArtPosition", "left")

	// Bind environment variables with prefix PLEXMUSIC_
	v.SetEnvPrefix("PLEXMUSIC")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Try to load the config file (it's ok if it doesn't exist)
	_ = v.ReadInConfig()

	// Unmarshal config
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	m := &Manager{
		cfg:  cfg,
		v:    v,
		path: configPath,
	}

	return m, nil
}

// GetAuthToken returns the stored auth token (from config or environment)
func (m *Manager) GetAuthToken() string {
	return m.v.GetString("authToken")
}

// SetAuthToken sets the auth token
func (m *Manager) SetAuthToken(token string) {
	m.v.Set("authToken", token)
	m.cfg.AuthToken = token
}

// GetLastSelectedServer returns the last selected server canonical key in the form host/name
func (m *Manager) GetLastSelectedServer() string {
	return m.v.GetString("lastSelectedServer")
}

// SetLastSelectedServer sets the last selected server canonical key in the form host/name
func (m *Manager) SetLastSelectedServer(serverName string) {
	m.v.Set("lastSelectedServer", serverName)
	m.cfg.LastSelectedServer = serverName
}

// GetVolume returns the stored volume level, defaulting to 0.0 if not previously set.
// Volume uses a logarithmic scale where 0 = 100% (no change), with Base:2 in the beep library.
// Positive values increase volume (1 = 200%), negative values decrease (e.g., -1 = 50%).
func (m *Manager) GetVolume() float64 {
	return m.v.GetFloat64("volume")
}

// GetCoverArtPosition returns the configured position of the cover art.
// Valid values are "left" or "right". If unset, default to "left".
func (m *Manager) GetCoverArtPosition() string {
	pos := m.v.GetString("coverArtPosition")
	if pos == "right" {
		return "right"
	}
	return "left"
}

// SetCoverArtPosition sets the cover art position. Use "left" or "right".
func (m *Manager) SetCoverArtPosition(pos string) {
	if pos != "right" {
		pos = "left"
	}
	m.v.Set("coverArtPosition", pos)
	m.cfg.CoverArtPosition = pos
}

// SetVolume sets the volume level
func (m *Manager) SetVolume(volume float64) {
	m.v.Set("volume", volume)
	m.cfg.Volume = volume
}

// Save saves the configuration to disk (via Viper)
func (m *Manager) Save() error {
	// Update the in-memory config struct from Viper
	if err := m.v.Unmarshal(m.cfg); err != nil {
		return fmt.Errorf("failed to unmarshal viper config: %w", err)
	}

	// Write Viper config
	if err := m.v.WriteConfig(); err != nil {
		// If file doesn't exist, create it
		if os.IsNotExist(err) {
			// Ensure directory exists
			dir := filepath.Dir(m.path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}
			// Write the config file
			if err := m.v.WriteConfigAs(m.path); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}
		} else {
			return fmt.Errorf("failed to write viper config: %w", err)
		}
	}

	return nil
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
