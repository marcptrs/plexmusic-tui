package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"

	"plexmusic-tui/internal/logging"

	log "github.com/charmbracelet/log/v2"

	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	"plexmusic-tui/internal/tui/pages"
)

func buildAppModel() *tui.AppModel {
	coord := app.NewCoordinator()
	authSvc := service.NewAuthService()
	// Create a singleton playback service and wire it to the coordinator so
	// all pages reuse the same instance.
	pbSvc := service.NewPlaybackService()
	coord.SetPlaybackService(pbSvc)
	// Start a background ticker to keep the playback service updating its
	// position for UI progress reporting. This runs for the life of the app.
	go func(svc *service.PlaybackService) {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			svc.UpdatePosition()
		}
	}(pbSvc)
	cfgMgr, _ := config.NewManager()
	coord.SetConfigManager(cfgMgr)
	// Load and apply saved volume from config
	if cfgMgr != nil {
		savedVolume := cfgMgr.GetVolume()
		pbSvc.SetVolume(savedVolume)
	}

	keyMap := tui.DefaultKeyMap()

	// Start with LoginPage
	loginPage := pages.NewLoginPageWithConfig(coord, authSvc, cfgMgr)
	router := tui.NewRouter(loginPage, tui.LoginPageID)

	// pageFactory returns a Page for a PageID; used by AppModel to create pages
	pageFactory := func(id tui.PageID) tui.Page {
		switch id {
		case tui.LoginPageID:
			return pages.NewLoginPageWithConfig(coord, authSvc, cfgMgr)
		case tui.ServerSelectionPageID:
			return pages.NewServerSelectionPage(coord, authSvc, cfgMgr)
		case tui.MainAppPageID:
			return pages.NewMainAppPageWithAuth(coord, authSvc)
		default:
			return nil
		}
	}

	return tui.NewAppModel(router, coord, authSvc, cfgMgr, keyMap, pageFactory)
}

func main() {
	// Define command-line flags
	showLogs := flag.Bool("logs", false, "Show recent log entries and exit")
	tailLines := flag.Int("tail", 50, "Number of log lines to show (default: 50)")
	showHelp := flag.Bool("help", false, "Show help message")
	flag.Parse()

	// Handle help flag
	if *showHelp {
		showUsage()
		os.Exit(0)
	}

	// Handle logs command
	if *showLogs {
		displayLogs("/tmp/plexmusic-startup.log", *tailLines)
		os.Exit(0)
	}

	// Determine log level from environment variable or default to Info
	logLevel := log.InfoLevel
	if debugEnv := os.Getenv("PLEXMUSIC_DEBUG"); debugEnv != "" {
		logLevel = log.DebugLevel
	}

	// Initialize file-backed logger for startup diagnostics
	logger, err := logging.InitFileLogger("/tmp/plexmusic-startup.log", logLevel)
	if err != nil {
		log.Error("Failed to initialize startup logger", "path", "/tmp/plexmusic-startup.log", "err", err)
	} else {
		logging.SetDefaultLogger(logger)
	}
	log.Info("main() started")

	appModel := buildAppModel()

	p := tea.NewProgram(
		appModel,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	log.Info("NewProgram() completed, calling Run()")

	if _, err := p.Run(); err != nil {
		log.Error("Run() failed", "error", err)
		panic(err)
	}

	log.Info("Run() completed successfully")
}

// displayLogs reads and displays the last N lines from the log file
func displayLogs(logFile string, tailLines int) {
	file, err := os.Open(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not open log file at %s\n", logFile)
		fmt.Fprintf(os.Stderr, "Details: %v\n", err)
		return
	}
	defer file.Close()

	// Read all lines from the file
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading log file: %v\n", err)
		return
	}

	// Determine which lines to display
	startIdx := 0
	if len(lines) > tailLines {
		startIdx = len(lines) - tailLines
		fmt.Fprintf(os.Stderr, "Showing last %d lines. Full logs at: %s\n\n", tailLines, logFile)
	}

	// Print the tail lines
	for i := startIdx; i < len(lines); i++ {
		fmt.Println(lines[i])
	}

	if len(lines) == 0 {
		fmt.Fprintf(os.Stderr, "No log entries found in %s\n", logFile)
	}
}

// showUsage displays the command-line help message
func showUsage() {
	fmt.Fprintf(os.Stderr, `Plex Music TUI - Terminal UI for Plex Music Server

USAGE:
  plexmusic-tui [OPTIONS]

OPTIONS:
  -logs               Show recent log entries and exit
  -tail N             Number of log lines to show (default: 50)
  -help               Show this help message

EXAMPLES:
  # Run the interactive TUI
  plexmusic-tui

  # Show last 50 log entries
  plexmusic-tui -logs

  # Show last 100 log entries
  plexmusic-tui -logs -tail 100

  # Show help
  plexmusic-tui -help

LOGS:
  Application logs are stored at: /tmp/plexmusic-startup.log

  Use -logs to view recent entries when debugging connection issues.

`)
}
