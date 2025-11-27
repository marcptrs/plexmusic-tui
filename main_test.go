package main

import (
	"os"
	"path/filepath"
	"testing"

	"plexmusic-tui/internal/config"
)

func TestBuildAppModel_UsesServerSelectionWhenTokenPresent(t *testing.T) {
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

	appData := buildAppModel()
	if appData == nil {
		t.Fatalf("buildAppModel returned nil")
	}

	// Check the initial page ID is server selection
	if appData.Model.CurrentPageID() != "server_selection" {
		t.Fatalf("expected initial PageID 'server_selection', got %s", appData.Model.CurrentPageID())
	}

	// Clean up - not strictly necessary since using TempDir
	os.RemoveAll(filepath.Join(tempHome, ".config"))
}
