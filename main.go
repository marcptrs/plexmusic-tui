package main

import (
	"os"

	"plexmusic-tui/cmd/root"
	"plexmusic-tui/internal/logging"
)

func main() {
	// Initialize file logger for TUI (avoid stdout interference)
	// Use plain text format for human-readable logs
	logger := logging.SetupWithFileLoggingText()

	if err := root.Execute(logger); err != nil {
		logger.Error("Application error", "error", err)
		os.Exit(1)
	}
}
