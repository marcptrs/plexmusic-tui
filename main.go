package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	termimg "plexmusic-tui/internal/image"

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
	// default options: no forced renderer; no render debug; no dump view
	return buildAppModelWithOptions(nil, false, false)
}

func buildAppModelWithOptions(
	forceRenderer *termimg.Protocol,
	renderDebug bool,
	dumpView bool,
) *tui.AppModel {
	// Create service instances first then pass to coordinator so pages can use them
	authSvc := service.NewAuthService()
	var libSvc service.LibraryServicer = nil
	// Create a singleton playback service now and pass to coordinator
	pbSvc := service.NewPlaybackService()
	// pass through rendering options to the coordinator so pages can use them
	coord := app.NewCoordinatorWithServices(
		authSvc,
		libSvc,
		pbSvc,
		forceRenderer,
		renderDebug,
		dumpView,
	)
	// Playback service has already been created above; wire it to the coordinator
	coord.SetPlaybackService(pbSvc)
	// Create orchestrator: used for playback and initial bootstrapping.
	orch := tui.NewOrchestrator(coord, nil, pbSvc)
	// Background ticker updates playback position for UI progress reporting.
	go func(svc *service.PlaybackService) {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			svc.UpdatePosition()
		}
	}(pbSvc)
	cfgMgr, _ := config.NewManager()
	coord.SetConfigManager(cfgMgr)
	// Load and apply saved volume from config (persisted setting)
	if cfgMgr != nil {
		savedVolume := cfgMgr.GetVolume()
		// Use orchestrator to set volume so any orchestration logic or config
		// persistence behaves consistently with UI actions.
		orch.SetVolume(savedVolume)
	}

	keyMap := tui.DefaultKeyMap()

	// Choose initial page based on saved auth token.
	var initialPage tui.Page
	var initialID tui.PageID
	token := ""
	if cfgMgr != nil {
		token = cfgMgr.GetAuthToken()
	}
	if token != "" {
		// We have a saved token — prefer server selection as initial page
		initialPage = pages.NewServerSelectionPage(coord, authSvc, cfgMgr)
		initialID = tui.ServerSelectionPageID
		coord.SetToken(token)
	} else {
		initialPage = pages.NewLoginPageWithConfig(coord, authSvc, cfgMgr)
		initialID = tui.LoginPageID
	}
	router := tui.NewRouter(initialPage, initialID)

	// pageFactory returns a Page for a PageID; used by AppModel to create pages
	pageFactory := func(id tui.PageID) tui.Page {
		switch id {
		case tui.LoginPageID:
			return pages.NewLoginPageWithConfig(coord, authSvc, cfgMgr)
		case tui.ServerSelectionPageID:
			return pages.NewServerSelectionPage(coord, authSvc, cfgMgr)
		case tui.LibraryPageID:
			return pages.NewLibraryPageWithAuth(coord, authSvc)
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
	debugFlag := flag.Bool("debug", false, "Enable debug logging (overrides default info level)")
	forceRenderer := flag.String(
		"force-renderer",
		"",
		"Force the image renderer: kitty|iterm2|sixel|unicode",
	)
	renderDebug := flag.Bool("render-debug", false, "Enable detailed image renderer debug logs")
	dumpViewFlag := flag.Bool(
		"dump-view",
		false,
		"Write raw page view to /tmp/plexmusic_view_debug.txt for debugging",
	)
	flag.Parse()

	if *showHelp {
		showUsage()
		os.Exit(0)
	}

	if *showLogs {
		displayLogs("/tmp/plexmusic-startup.log", *tailLines)
		os.Exit(0)
	}

	logLevel := log.InfoLevel
	if *debugFlag {
		logLevel = log.DebugLevel
	}

	// Initialize file-backed logger for startup diagnostics
	logger, err := logging.InitFileLogger("/tmp/plexmusic-startup.log", logLevel)
	if err != nil {
		log.Error(
			"Failed to initialize startup logger",
			"path",
			"/tmp/plexmusic-startup.log",
			"err",
			err,
		)
	} else {
		logging.SetDefaultLogger(logger)
	}
	log.Info("main() started")

	// Map forceRenderer string to a protocol constant (if provided)
	var forcedProtocol *termimg.Protocol
	if *forceRenderer != "" {
		switch strings.ToLower(*forceRenderer) {
		case "kitty":
			p := termimg.ProtocolKitty
			forcedProtocol = &p
		case "iterm2", "iterm":
			p := termimg.ProtocolITerm2
			forcedProtocol = &p
		case "sixel":
			p := termimg.ProtocolSixel
			forcedProtocol = &p
		case "unicode", "blocks":
			p := termimg.ProtocolUnicodeBlocks
			forcedProtocol = &p
		}
	}
	appModel := buildAppModelWithOptions(forcedProtocol, *renderDebug, *dumpViewFlag)

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
	-debug              Enable debug logging
	-force-renderer STR Force image renderer: kitty | iterm2 | sixel | unicode
	-render-debug       Enable detailed image renderer debug logs
	-dump-view          Enable raw View() dumps for debugging (writes /tmp/plexmusic_view_debug.txt)

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
