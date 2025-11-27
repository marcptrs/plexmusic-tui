package bootstrap

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/auth"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui"
	"plexmusic-tui/internal/tui/pages"
)

// AppServices bundles the core application services
type AppServices struct {
	AuthService     service.AuthServicer
	LibraryService  service.LibraryServicer
	PlaybackService service.PlaybackServicer
}

// AppOptions contains configuration options for app initialization
type AppOptions struct {
	ForceRenderer *domain.Protocol
	RenderDebug   bool
	DumpView      bool
}

// App bundles the initialized application components
type App struct {
	Model           *tui.AppModel
	Orchestrator    *tui.Orchestrator
	PlaybackService *service.PlaybackService
	ConfigManager   *config.Manager
	ctx             context.Context
	cancel          context.CancelFunc
	eg              *errgroup.Group
}

// Context returns the application's context for background tasks
func (a *App) Context() context.Context {
	return a.ctx
}

// Close shuts down background goroutines and releases resources
func (a *App) Close() error {
	a.cancel()
	return a.eg.Wait()
}

// provideHTTPClient provides a standard HTTP client
func provideHTTPClient() *http.Client {
	return &http.Client{}
}

// provideAuthenticator provides a Plex authenticator
func provideAuthenticator(client *http.Client) *auth.Authenticator {
	return auth.NewAuthenticator(client)
}

// provideAuthService provides the authentication service
func provideAuthService(authenticator *auth.Authenticator) service.AuthServicer {
	return service.NewAuthService(authenticator)
}

// provideLibraryService provides the library service
func provideLibraryService() service.LibraryServicer {
	return nil
}

// providePlaybackService provides the playback service
func providePlaybackService() *service.PlaybackService {
	return service.NewPlaybackService()
}

// providePlaybackServicer casts the concrete PlaybackService to the interface
func providePlaybackServicer(pb *service.PlaybackService) service.PlaybackServicer {
	return pb
}

// provideAppServices bundles all services
func provideAppServices(
	authService service.AuthServicer,
	libraryService service.LibraryServicer,
	playbackService service.PlaybackServicer,
) AppServices {
	return AppServices{
		AuthService:     authService,
		LibraryService:  libraryService,
		PlaybackService: playbackService,
	}
}

// provideConfigManager provides the configuration manager
func provideConfigManager() *config.Manager {
	// Create with default error handling; app startup will gracefully continue
	// if config loading fails
	cfgMgr, _ := config.NewManager()
	return cfgMgr
}

// provideCoordinator creates the coordinator with services
func provideCoordinator(
	services AppServices,
	cfgMgr *config.Manager,
	opts AppOptions,
) *app.Coordinator {
	coord := app.NewCoordinatorWithServices(
		services.AuthService,
		services.LibraryService,
		services.PlaybackService,
		opts.ForceRenderer,
		opts.RenderDebug,
		opts.DumpView,
	)
	coord.SetPlaybackService(services.PlaybackService)
	if cfgMgr != nil {
		coord.SetConfigManager(cfgMgr)
	}
	return coord
}

// provideOrchestrator creates the playback orchestrator
func provideOrchestrator(
	coord *app.Coordinator,
	playbackService *service.PlaybackService,
) *tui.Orchestrator {
	return tui.NewOrchestrator(coord, nil, playbackService)
}

// provideKeyMap provides the default key map
func provideKeyMap() tui.KeyMap {
	return tui.DefaultKeyMap()
}

// provideRouter creates the initial router based on auth state
func provideRouter(
	cfgMgr *config.Manager,
	coord *app.Coordinator,
	authService service.AuthServicer,
) *tui.Router {
	var initialPage tui.Page
	var initialID tui.PageID

	token := ""
	if cfgMgr != nil {
		token = cfgMgr.GetAuthToken()
	}

	if token != "" {
		initialPage = pages.NewServerSelectionPage(coord, authService, cfgMgr)
		initialID = tui.ServerSelectionPageID
		coord.SetToken(token)
	} else {
		initialPage = pages.NewLoginPageWithConfig(coord, authService, cfgMgr)
		initialID = tui.LoginPageID
	}

	return tui.NewRouter(initialPage, initialID)
}

// providePageFactory creates a page factory callback
func providePageFactory(
	coord *app.Coordinator,
	authService service.AuthServicer,
	cfgMgr *config.Manager,
) tui.PageFactoryFn {
	return func(id tui.PageID) tui.Page {
		switch id {
		case tui.LoginPageID:
			return pages.NewLoginPageWithConfig(coord, authService, cfgMgr)
		case tui.ServerSelectionPageID:
			return pages.NewServerSelectionPage(coord, authService, cfgMgr)
		case tui.LibraryPageID:
			return pages.NewLibraryPageWithAuth(coord, authService)
		default:
			return nil
		}
	}
}

// provideAppModel creates the main app model
func provideAppModel(
	router *tui.Router,
	coord *app.Coordinator,
	authService service.AuthServicer,
	cfgMgr *config.Manager,
	keyMap tui.KeyMap,
	pageFactory tui.PageFactoryFn,
) *tui.AppModel {
	return tui.NewAppModel(router, coord, authService, cfgMgr, keyMap, pageFactory)
}

// provideApp bundles the application components into a single App struct
func provideApp(
	model *tui.AppModel,
	orch *tui.Orchestrator,
	pbSvc *service.PlaybackService,
	cfgMgr *config.Manager,
) *App {
	ctx, cancel := context.WithCancel(context.Background())
	eg, egCtx := errgroup.WithContext(ctx)
	return &App{
		Model:           model,
		Orchestrator:    orch,
		PlaybackService: pbSvc,
		ConfigManager:   cfgMgr,
		ctx:             egCtx,
		cancel:          cancel,
		eg:              eg,
	}
}

// InitializePlaybackPositionUpdater starts the background position updater
// It schedules the position update task on the app's errgroup for managed lifecycle
func InitializePlaybackPositionUpdater(appData *App, playbackService *service.PlaybackService) {
	appData.eg.Go(func() error {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-appData.ctx.Done():
				return appData.ctx.Err()
			case <-ticker.C:
				playbackService.UpdatePosition()
			}
		}
	})
}

// InitializeVolume loads and applies saved volume from config
func InitializeVolume(
	cfgMgr *config.Manager,
	orch *tui.Orchestrator,
) {
	if cfgMgr != nil {
		savedVolume := cfgMgr.GetVolume()
		orch.SetVolume(savedVolume)
	}
}
