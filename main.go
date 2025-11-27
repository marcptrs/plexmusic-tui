package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"plexmusic-tui/internal/bootstrap"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/logging"

	log "github.com/charmbracelet/log/v2"

	tea "github.com/charmbracelet/bubbletea"
)

func buildAppModel() *bootstrap.App {
	// default options: no forced renderer; no render debug; no dump view
	return buildAppModelWithOptions(nil, false, false)
}

func buildAppModelWithOptions(
	forceRenderer *domain.Protocol,
	renderDebug bool,
	dumpView bool,
) *bootstrap.App {
	opts := bootstrap.AppOptions{
		ForceRenderer: forceRenderer,
		RenderDebug:   renderDebug,
		DumpView:      dumpView,
	}
	return bootstrap.InitializeApp(opts)
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
		displayLogs(logging.GetLogFilePath(), *tailLines)
		os.Exit(0)
	}

	logLevel := log.InfoLevel
	if *debugFlag {
		logLevel = log.DebugLevel
	}

	// Initialize file-backed logger for startup diagnostics
	logPath := logging.GetLogFilePath()
	logger, err := logging.InitFileLogger(logPath, logLevel)
	if err != nil {
		log.Error(
			"Failed to initialize startup logger",
			"path",
			logPath,
			"err",
			err,
		)
	} else {
		logging.SetDefaultLogger(logger)
	}
	log.Info("main() started")

	// Map forceRenderer string to a protocol constant (if provided)
	var forcedProtocol *domain.Protocol
	if *forceRenderer != "" {
		switch strings.ToLower(*forceRenderer) {
		case "kitty":
			p := domain.ProtocolKitty
			forcedProtocol = &p
		case "iterm2", "iterm":
			p := domain.ProtocolITerm2
			forcedProtocol = &p
		case "sixel":
			p := domain.ProtocolSixel
			forcedProtocol = &p
		case "unicode", "blocks":
			p := domain.ProtocolUnicodeBlocks
			forcedProtocol = &p
		}
	}
	appData := buildAppModelWithOptions(forcedProtocol, *renderDebug, *dumpViewFlag)

	// Start the position updater goroutine
	bootstrap.InitializePlaybackPositionUpdater(appData.PlaybackService)

	// Initialize volume from config
	bootstrap.InitializeVolume(appData.ConfigManager, appData.Orchestrator)

	p := tea.NewProgram(
		appData.Model,
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
	-dump-view          Enable raw View() dumps for debugging (writes %s)

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
  Application logs are stored at: %s

  Use -logs to view recent entries when debugging connection issues.

`, logging.GetDebugDumpFilePath(), logging.GetLogFilePath())
}
