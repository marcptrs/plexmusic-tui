package app

import (
	"context"
	"image"

	"github.com/charmbracelet/bubbles/textinput"

	"plexmusic-tui/internal/domain"
	termimg "plexmusic-tui/internal/image"
	"plexmusic-tui/internal/service"
)

// CoordinatorRefactored manages application state using service interfaces
// This is the new architecture - gradually migrate from coordinator.go
type CoordinatorRefactored struct {
	// Services (interfaces for testability)
	authService     service.AuthServicer
	libraryService  service.LibraryServicer
	playbackService service.PlaybackServicer

	// Event context for pub/sub subscriptions
	ctx    context.Context
	cancel context.CancelFunc

	// Session and authentication
	sessionState SessionState
	err          error
	token        string

	// Servers and libraries (domain models from service layer)
	servers         []domain.PlexServer
	selectedServer  int
	libraries       []domain.MusicLibrary
	selectedLibrary int

	// Content management
	currentContent ContentViewType
	activeTab      TabType

	// Navigation state
	selectedHome     int
	selectedAlbum    int
	selectedPlaylist int
	selectedTrack    int
	contentScroll    int

	// Content collections
	albums     []domain.Album
	playlists  []domain.Playlist
	tracks     []domain.Track
	queue      []domain.Track
	queueIndex int

	// UI state
	showQueueModal bool
	width          int
	height         int

	// Album art caching
	currentAlbumArt       image.Image
	currentAlbumArtThumb  string
	playbackAlbumArt      image.Image
	playbackAlbumArtThumb string

	// Image renderers
	imgRenderer         *termimg.Renderer
	playbackImgRenderer *termimg.Renderer

	// Text inputs for login
	usernameInput textinput.Model
	passwordInput textinput.Model
	focusIndex    int
}

// NewCoordinatorRefactored creates a new coordinator with service dependencies
func NewCoordinatorRefactored(
	authSvc service.AuthServicer,
	librarySvc service.LibraryServicer,
	playbackSvc service.PlaybackServicer,
) *CoordinatorRefactored {
	ctx, cancel := context.WithCancel(context.Background())

	usernameInput := textinput.New()
	usernameInput.Placeholder = "Email or username"
	usernameInput.Focus()
	usernameInput.CharLimit = 100
	usernameInput.Width = 40

	passwordInput := textinput.New()
	passwordInput.Placeholder = "Password"
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.EchoCharacter = '•'
	passwordInput.CharLimit = 100
	passwordInput.Width = 40

	return &CoordinatorRefactored{
		authService:         authSvc,
		libraryService:      librarySvc,
		playbackService:     playbackSvc,
		ctx:                 ctx,
		cancel:              cancel,
		sessionState:        LoginView,
		usernameInput:       usernameInput,
		passwordInput:       passwordInput,
		imgRenderer:         termimg.NewRenderer(),
		playbackImgRenderer: termimg.NewRendererWithProtocol(termimg.ProtocolUnicodeBlocks),
	}
}

// Close releases resources and cancels event subscriptions
func (c *CoordinatorRefactored) Close() error {
	c.cancel()
	// Services will handle their own cleanup via context
	return nil
}

// Context returns the coordinator's context for event subscriptions
func (c *CoordinatorRefactored) Context() context.Context {
	return c.ctx
}

// Service accessors
func (c *CoordinatorRefactored) AuthService() service.AuthServicer {
	return c.authService
}

func (c *CoordinatorRefactored) LibraryService() service.LibraryServicer {
	return c.libraryService
}

func (c *CoordinatorRefactored) PlaybackService() service.PlaybackServicer {
	return c.playbackService
}

// State accessors - reuse patterns from coordinator.go
// (Only showing subset - full implementation would mirror coordinator.go)

func (c *CoordinatorRefactored) SessionState() SessionState {
	return c.sessionState
}

func (c *CoordinatorRefactored) SetSessionState(s SessionState) {
	c.sessionState = s
}

func (c *CoordinatorRefactored) Token() string {
	return c.token
}

func (c *CoordinatorRefactored) SetToken(token string) {
	c.token = token
}

func (c *CoordinatorRefactored) IsLoggedIn() bool {
	return c.token != ""
}

// Additional accessors follow same pattern as coordinator.go...
// For brevity, showing representative examples. Full migration would
// copy all accessor/mutator patterns.
