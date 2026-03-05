package root

import (
	"strings"

	"github.com/spf13/cobra"

	"plexmusic-tui/cmd/logs"
	"plexmusic-tui/cmd/run"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/logging"
)

var (
	// Global flags
	debugFlag      bool
	forceRenderer  string
	renderDebug    bool
	dumpViewFlag   bool
	forcedProtocol *domain.Protocol
	rootLogger     logging.Logger
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "plexmusic-tui",
	Short: "Terminal UI for Plex Music Server",
	Long: `Plex Music TUI provides a terminal user interface for browsing
and playing music from a Plex Media Server.

Use 'plexmusic-tui run' to start the interactive TUI, or 'plexmusic-tui logs'
to view recent log entries.`,
	Version: "1.0.0",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Parse force-renderer flag
		if forceRenderer != "" {
			p := parseRendererFlag(forceRenderer)
			forcedProtocol = p
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Default to 'run' if no subcommand specified
		run.Execute(forcedProtocol, renderDebug, dumpViewFlag, rootLogger)
	},
}

// Execute adds all child commands to the root command
func Execute(logger logging.Logger) error {
	if logger != nil {
		rootLogger = logger
	}
	return rootCmd.Execute()
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(run.NewRunCommand(
		func() *domain.Protocol { return forcedProtocol },
		func() bool { return renderDebug },
		func() bool { return dumpViewFlag },
		func() logging.Logger { return rootLogger },
	))
	rootCmd.AddCommand(logs.NewLogsCommand())

	// Global persistent flags
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false,
		"Enable debug logging")
	rootCmd.PersistentFlags().StringVar(&forceRenderer, "force-renderer", "",
		"Force image renderer: kitty|iterm2|sixel|unicode")
	rootCmd.PersistentFlags().BoolVar(&renderDebug, "render-debug", false,
		"Enable detailed image renderer debug logs")
	rootCmd.PersistentFlags().BoolVar(&dumpViewFlag, "dump-view", false,
		"Write raw page view to /tmp/plexmusic_view_debug.txt")
}

// parseRendererFlag parses the force-renderer string to Protocol
func parseRendererFlag(renderer string) *domain.Protocol {
	switch strings.ToLower(renderer) {
	case "kitty":
		p := domain.ProtocolKitty
		return &p
	case "iterm2", "iterm":
		p := domain.ProtocolITerm2
		return &p
	case "sixel":
		p := domain.ProtocolSixel
		return &p
	case "unicode", "blocks":
		p := domain.ProtocolUnicodeBlocks
		return &p
	}
	return nil
}

// GetForcedProtocol returns the forced protocol if set
func GetForcedProtocol() *domain.Protocol {
	return forcedProtocol
}
