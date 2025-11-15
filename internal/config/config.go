package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config represents the application configuration
type Config struct {
	AuthToken          string `json:"authToken"`
	LastSelectedServer string `json:"lastSelectedServer,omitempty"` // Server canonical key in the form host/name
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
