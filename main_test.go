package main

import (
	"os"
	"path/filepath"
	"testing"

	"plexmusic-tui/internal/bootstrap"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/logging"
)

func TestInitializeApp_UsesServerSelectionWhenTokenPresent(t *testing.T) {
	// Create a temporary HOME dir so we can write config without touching user files
	tempHome := t.TempDir()
	os.Setenv("HOME", tempHome)
	// Ensure the config directory is created and a config saved with token
	cfg := &config.Config{
		AuthToken:          "test-token",
		LastSelectedServer: "testhost/server-name",
	}
	// Save config to path: $HOME/.config/plexmusic-tui/config.json via Save
	if err := config.Save(cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	// Initialize logger for test
	logCfg := logging.DefaultConfig()
	logger := logging.SetupWithAdapter(logCfg)

	appData := bootstrap.InitializeApp(bootstrap.AppOptions{}, logger)
	if appData == nil {
		t.Fatalf("InitializeApp returned nil")
	}

	// Check the initial page ID is server selection
	if appData.Model.CurrentPageID() != "server_selection" {
		t.Fatalf("expected initial PageID 'server_selection', got %s", appData.Model.CurrentPageID())
	}

	// Clean up - not strictly necessary since using TempDir
	os.RemoveAll(filepath.Join(tempHome, ".config"))
}
