package main

import (
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
	// Initialize file-backed logger for startup diagnostics
	logger, err := logging.InitFileLogger("/tmp/plexmusic-startup.log", log.InfoLevel)
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
