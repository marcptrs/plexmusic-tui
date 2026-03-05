package app

import (
	"context"
	"net/http"
	"sync"

	"plexmusic-tui/internal/auth"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/logging"
	"plexmusic-tui/internal/service"
)

// Coordinator is the application's canonical state container.
// It's now a thin facade that composes focused contexts:
// - ViewContext: UI state (dimensions, navigation, notifications)
// - Services: Infrastructure (auth, library, playback services)
// - PlaybackContext: Audio playback state and streaming
// - ContentState: Domain data collections (albums, tracks, playlists)
// - SessionContext: Authentication and server selection
type Coordinator struct {
	*AppContext

	mu     sync.RWMutex
	logger logging.Logger
}

// NewCoordinatorWithServices creates a new coordinator with service dependencies
func NewCoordinatorWithServices(
	authSvc service.AuthServicer,
	librarySvc service.LibraryServicer,
	playbackSvc service.PlaybackServicer,
	cfgMgr *config.Manager,
	forceProtocol *domain.Protocol,
	renderDebug bool,
	dumpView bool,
) *Coordinator {
	services := NewServices(authSvc, librarySvc, playbackSvc, cfgMgr, forceProtocol, renderDebug)
	ctx := NewAppContext(services)
	ctx.View.SetDumpView(dumpView)

	return &Coordinator{
		AppContext: ctx,
	}
}

// NewCoordinator creates a Coordinator wired with default services that are
// suitable for app runtime. Tests can still call NewCoordinatorWithServices
// to inject test doubles.
func NewCoordinator() *Coordinator {
	authGateway := auth.NewAuthenticator(&http.Client{})
	authSvc := service.NewAuthService(authGateway)
	var libSvc service.LibraryServicer = nil
	pbSvc := service.NewPlaybackService()
	return NewCoordinatorWithServices(authSvc, libSvc, pbSvc, nil, nil, false, false)
}

// Close releases resources and cancels event subscriptions
func (c *Coordinator) Close() error {
	if c.Services != nil {
		c.Services.Close()
	}
	return nil
}

// Context returns the coordinator's context for event subscriptions
func (c *Coordinator) Context() context.Context {
	return c.Services.Context()
}

// GetAppContext returns the underlying AppContext
func (c *Coordinator) GetAppContext() *AppContext {
	return c.AppContext
}

// SetLogger sets the logger for the coordinator
func (c *Coordinator) SetLogger(logger logging.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger = logger
}

// GetLogger returns the logger
func (c *Coordinator) GetLogger() logging.Logger {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return logging.GetLoggerOrDefault(c.logger)
}
