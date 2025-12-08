package app

import (
	"context"

	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	termimg "plexmusic-tui/internal/image"
	"plexmusic-tui/internal/service"
)

// Services manages the application's service dependencies.
// It provides a clean separation between service orchestration and UI state.
type Services struct {
	// Core services
	authService     service.AuthServicer
	libraryService  service.LibraryServicer
	playbackService service.PlaybackServicer

	// Configuration
	configMgr *config.Manager

	// Event context for pub/sub subscriptions
	ctx    context.Context
	cancel context.CancelFunc

	// Image rendering
	imgRenderer         domain.ImageRenderer
	playbackImgRenderer domain.ImageRenderer
}

// NewServices creates a new services container
func NewServices(
	authSvc service.AuthServicer,
	librarySvc service.LibraryServicer,
	playbackSvc service.PlaybackServicer,
	cfgMgr *config.Manager,
	forceProtocol *domain.Protocol,
	renderDebug bool,
) *Services {
	ctx, cancel := context.WithCancel(context.Background())

	imgRenderer := termimg.NewRenderer()
	playbackImgRenderer := termimg.NewRendererWithProtocol(domain.ProtocolUnicodeBlocks)

	if forceProtocol != nil {
		imgRenderer = termimg.NewRendererWithProtocol(*forceProtocol)
	}
	if renderDebug {
		imgRenderer.SetDebug(true)
		playbackImgRenderer.SetDebug(true)
	}

	return &Services{
		authService:         authSvc,
		libraryService:      librarySvc,
		playbackService:     playbackSvc,
		configMgr:           cfgMgr,
		ctx:                 ctx,
		cancel:              cancel,
		imgRenderer:         imgRenderer,
		playbackImgRenderer: playbackImgRenderer,
	}
}

// Close cancels the context and cleans up resources
func (s *Services) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Auth service

func (s *Services) AuthService() service.AuthServicer {
	return s.authService
}

// Library service

func (s *Services) LibraryService() service.LibraryServicer {
	return s.libraryService
}

func (s *Services) SetLibraryService(svc service.LibraryServicer) {
	s.libraryService = svc
}

// Playback service

func (s *Services) PlaybackService() service.PlaybackServicer {
	return s.playbackService
}

func (s *Services) SetPlaybackService(svc service.PlaybackServicer) {
	s.playbackService = svc
}

// Config

func (s *Services) ConfigManager() *config.Manager {
	return s.configMgr
}

func (s *Services) SetConfigManager(cfg *config.Manager) {
	s.configMgr = cfg
}

// Context

func (s *Services) Context() context.Context {
	return s.ctx
}

// Image renderers

func (s *Services) ImgRenderer() domain.ImageRenderer {
	return s.imgRenderer
}

func (s *Services) PlaybackImgRenderer() domain.ImageRenderer {
	return s.playbackImgRenderer
}
