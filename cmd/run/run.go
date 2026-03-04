package run

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"plexmusic-tui/internal/bootstrap"
	"plexmusic-tui/internal/domain"
)

// NewRunCommand creates the 'run' subcommand for starting the TUI
func NewRunCommand(
	getForceRenderer func() *domain.Protocol,
	getRenderDebug func() bool,
	getDumpView func() bool,
) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Start the interactive Plex Music TUI",
		Long:  "Launches the interactive terminal user interface for Plex Music.",
		Run: func(cmd *cobra.Command, args []string) {
			Execute(
				getForceRenderer(),
				getRenderDebug(),
				getDumpView(),
			)
		},
	}
}

// Execute starts the TUI application
func Execute(
	forceRenderer *domain.Protocol,
	renderDebug bool,
	dumpView bool,
) {
	opts := bootstrap.AppOptions{
		ForceRenderer: forceRenderer,
		RenderDebug:   renderDebug,
		DumpView:      dumpView,
	}

	appData := bootstrap.InitializeApp(opts)
	if appData == nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize application\n")
		os.Exit(1)
	}
	defer appData.Close()

	// Start the position updater goroutine on the app's errgroup
	bootstrap.InitializePlaybackPositionUpdater(appData, appData.PlaybackService)

	// Initialize volume from config
	bootstrap.InitializeVolume(appData.ConfigManager, appData.Orchestrator)

	p := tea.NewProgram(appData.Model)

	if _, err := p.Run(); err != nil {
		panic(err)
	}
}
