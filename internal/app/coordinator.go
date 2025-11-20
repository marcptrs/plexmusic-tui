package app

import (
	"fmt"
	"image"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"

	"plexmusic-tui/internal/auth"
	"plexmusic-tui/internal/config"
	termimg "plexmusic-tui/internal/image"
	"plexmusic-tui/internal/plex"
	"plexmusic-tui/internal/service"

	log "github.com/charmbracelet/log/v2"
)

// SessionState represents the current session/screen state
type SessionState int

const (
	LoginView SessionState = iota
	AuthenticatingView
	SuccessView
	ErrorView
	ServerSelectionView
	MainAppView
)

// ContentViewType represents different content views within the main app
type ContentViewType int

const (
	RecentlyAddedContent ContentViewType = iota
	AlbumTracksContent
	PlaylistsContent
	PlaylistTracksContent
	SearchContent
	SettingsContent
	QueueContent
)

// TabType represents the different tabs in main app view
type TabType int

const (
	HomeTab TabType = iota
	LibraryTab
	PlaylistsTab
	SearchTab
	QueueTab
	SettingsTab
)

// PlaybackState represents the current playback state
type PlaybackState int

const (
	PlaybackStopped PlaybackState = iota
	PlaybackPlaying
	PlaybackPaused
)

// PlexServer represents a Plex server
type PlexServer struct {
	Name         string
	Host         string
	Port         string
	AccessToken  string
	LocalAddress string
	Scheme       string
}

// MusicLibrary represents a music library in Plex
type MusicLibrary struct {
	Key   string
	Title string
	Type  string
}

// Album represents an album
type Album struct {
	Title  string
	Artist string
	Year   int
	Key    string
	Thumb  string
}

// Playlist represents a playlist
type Playlist struct {
	Title        string
	Key          string
	LeafCount    int
	Duration     int
	PlaylistType string
}

// Track represents a music track
type Track struct {
	Title          string
	Artist         string
	Album          string
	Duration       int
	TrackNumber    int
	PlaylistItemID int
	Key            string
	RatingKey      string
	Thumb          string
	Media          []struct {
		Part []struct {
			Key string
		}
	}
}

// Coordinator manages application state and business logic
type Coordinator struct {
	// Session and authentication
	sessionState  SessionState
	err           error
	token         string
	configMgr     *config.Manager
	authenticator *auth.Authenticator
	plexClient    *plex.Client

	// Servers and libraries
	servers         []PlexServer
	selectedServer  int
	libraries       []MusicLibrary
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

	// Playback state
	playbackState  PlaybackState
	currentTrack   *Track
	streamer       beep.StreamSeekCloser
	ctrl           *beep.Ctrl
	volume         *effects.Volume
	speakerInit    bool
	sampleRate     beep.SampleRate
	streamPosition int
	streamLength   int

	// Content collections
	albums     []Album
	playlists  []Playlist
	tracks     []Track
	queue      []Track
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

	// Notification message for transient UI notifications
	notifMsg      string
	notifSeverity string
	notifExpiry   time.Time

	// PlaybackService is a singleton service for the app lifecycle. Pages should
	// reuse this instance instead of creating their own to avoid event
	// subscription mismatches and duplicate audio pipelines.
	playbackService *service.PlaybackService
}

// NewCoordinator creates a new coordinator instance
func NewCoordinator() *Coordinator {
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

	coord := &Coordinator{
		sessionState:        LoginView,
		usernameInput:       usernameInput,
		passwordInput:       passwordInput,
		focusIndex:          0,
		authenticator:       auth.NewAuthenticator(),
		imgRenderer:         termimg.NewRenderer(),
		playbackImgRenderer: termimg.NewRendererWithProtocol(termimg.ProtocolUnicodeBlocks),
	}

	return coord
}

// State accessors and mutators
func (c *Coordinator) SessionState() SessionState {
	return c.sessionState
}

func (c *Coordinator) SetSessionState(s SessionState) {
	c.sessionState = s
}

func (c *Coordinator) Error() error {
	return c.err
}

func (c *Coordinator) SetError(err error) {
	c.err = err
}

func (c *Coordinator) Token() string {
	return c.token
}

func (c *Coordinator) GetToken() string {
	return c.token
}

func (c *Coordinator) SetToken(token string) {
	c.token = token
	// Persist token to disk via the config manager when available
	if c.configMgr != nil {
		c.configMgr.SetAuthToken(token)
		if err := c.configMgr.Save(); err != nil {
			// Log a warning if save fails but do not disrupt program flow
			log.Warn("Coordinator.SetToken: failed to save auth token to config", "err", err)
		}
	}
}

func (c *Coordinator) Authenticator() *auth.Authenticator {
	return c.authenticator
}

// SetConfigManager wires the config manager into the coordinator so that
// configuration changes (like auth token persistence) can be persisted.
func (c *Coordinator) SetConfigManager(cfg *config.Manager) {
	c.configMgr = cfg
}

func (c *Coordinator) PlexClient() *plex.Client {
	return c.plexClient
}

func (c *Coordinator) SetPlexClient(client *plex.Client) {
	c.plexClient = client
}

// Server management
func (c *Coordinator) Servers() []PlexServer {
	return c.servers
}

func (c *Coordinator) SetServers(servers []PlexServer) {
	c.servers = servers
}

func (c *Coordinator) SelectedServer() int {
	return c.selectedServer
}

func (c *Coordinator) SetSelectedServer(idx int) {
	// Always update in-memory index
	c.selectedServer = idx

	// Persist the selected server to disk using a canonical key.
	// Preferred form: host/name
	// If host is empty, persist name-only (fallback). This preserves
	// older saved values that only contain a name and avoids saving
	// a leading-slash key like "/servername".
	//
	// TODO: Update README.md and AGENTS.md to explicitly show the
	// canonical host/name format saved in LastSelectedServer.
	if c.configMgr == nil {
		return
	}
	if idx < 0 || idx >= len(c.servers) {
		// Invalid index - do not attempt to persist a selection.
		return
	}

	// Build a stable key for the selected server: use host/name when available,
	// otherwise fall back to name-only.
	host := c.servers[idx].Host
	name := c.servers[idx].Name
	var key string
	if host == "" {
		key = name
	} else {
		key = fmt.Sprintf("%s/%s", host, name)
	}
	c.configMgr.SetLastSelectedServer(key)
	if err := c.configMgr.Save(); err != nil {
		// Non-fatal: log and continue.
		log.Warn("Coordinator.SetSelectedServer: failed to save last selected server", "err", err)
	}
}

// Library management
func (c *Coordinator) Libraries() []MusicLibrary {
	return c.libraries
}

func (c *Coordinator) SetLibraries(libs []MusicLibrary) {
	c.libraries = libs
}

func (c *Coordinator) SelectedLibrary() int {
	return c.selectedLibrary
}

func (c *Coordinator) SetSelectedLibrary(idx int) {
	c.selectedLibrary = idx
}

// Album management
func (c *Coordinator) Albums() []Album {
	return c.albums
}

func (c *Coordinator) SetAlbums(albums []Album) {
	c.albums = albums
}

func (c *Coordinator) SelectedAlbum() int {
	return c.selectedAlbum
}

func (c *Coordinator) SetSelectedAlbum(idx int) {
	c.selectedAlbum = idx
}

// Track management
func (c *Coordinator) Tracks() []Track {
	return c.tracks
}

func (c *Coordinator) SetTracks(tracks []Track) {
	c.tracks = tracks
}

func (c *Coordinator) SelectedTrack() int {
	return c.selectedTrack
}

func (c *Coordinator) SetSelectedTrack(idx int) {
	c.selectedTrack = idx
}

func (c *Coordinator) CurrentTrack() *Track {
	return c.currentTrack
}

func (c *Coordinator) SetCurrentTrack(track *Track) {
	c.currentTrack = track
}

// Playlist management
func (c *Coordinator) Playlists() []Playlist {
	return c.playlists
}

func (c *Coordinator) SetPlaylists(playlists []Playlist) {
	c.playlists = playlists
}

func (c *Coordinator) SelectedPlaylist() int {
	return c.selectedPlaylist
}

func (c *Coordinator) SetSelectedPlaylist(idx int) {
	c.selectedPlaylist = idx
}

// Queue management
func (c *Coordinator) Queue() []Track {
	return c.queue
}

func (c *Coordinator) SetQueue(queue []Track) {
	c.queue = queue
}

func (c *Coordinator) QueueIndex() int {
	return c.queueIndex
}

func (c *Coordinator) SetQueueIndex(idx int) {
	c.queueIndex = idx
}

// UI state management
func (c *Coordinator) ActiveTab() TabType {
	return c.activeTab
}

func (c *Coordinator) SetActiveTab(tab TabType) {
	c.activeTab = tab
}

func (c *Coordinator) CurrentContent() ContentViewType {
	return c.currentContent
}

func (c *Coordinator) SetCurrentContent(content ContentViewType) {
	c.currentContent = content
}

func (c *Coordinator) ShowQueueModal() bool {
	return c.showQueueModal
}

func (c *Coordinator) SetShowQueueModal(show bool) {
	c.showQueueModal = show
}

func (c *Coordinator) ContentScroll() int {
	return c.contentScroll
}

func (c *Coordinator) SetContentScroll(scroll int) {
	c.contentScroll = scroll
}

// Playback state
func (c *Coordinator) PlaybackState() PlaybackState {
	return c.playbackState
}

func (c *Coordinator) SetPlaybackState(state PlaybackState) {
	c.playbackState = state
}

// Terminal dimensions
func (c *Coordinator) Width() int {
	return c.width
}

func (c *Coordinator) SetWidth(w int) {
	c.width = w
}

func (c *Coordinator) Height() int {
	return c.height
}

func (c *Coordinator) SetHeight(h int) {
	c.height = h
}

// Album art cache
func (c *Coordinator) CurrentAlbumArt() image.Image {
	return c.currentAlbumArt
}

func (c *Coordinator) SetCurrentAlbumArt(img image.Image, thumbURL string) {
	c.currentAlbumArt = img
	c.currentAlbumArtThumb = thumbURL
}

func (c *Coordinator) CurrentAlbumArtThumb() string {
	return c.currentAlbumArtThumb
}

func (c *Coordinator) PlaybackAlbumArt() image.Image {
	return c.playbackAlbumArt
}

func (c *Coordinator) SetPlaybackAlbumArt(img image.Image, thumbURL string) {
	c.playbackAlbumArt = img
	c.playbackAlbumArtThumb = thumbURL
}

func (c *Coordinator) PlaybackAlbumArtThumb() string {
	return c.playbackAlbumArtThumb
}

// Notifications
func (c *Coordinator) SetNotification(msg, severity string, duration time.Duration) {
	c.notifMsg = msg
	c.notifSeverity = severity
	if duration > 0 {
		c.notifExpiry = time.Now().Add(duration)
	} else {
		c.notifExpiry = time.Time{}
	}
}

func (c *Coordinator) Notification() (string, string, time.Time) {
	return c.notifMsg, c.notifSeverity, c.notifExpiry
}

func (c *Coordinator) ClearNotification() {
	c.notifMsg = ""
	c.notifSeverity = ""
	c.notifExpiry = time.Time{}
}

func (c *Coordinator) NotificationActive() bool {
	if c.notifMsg == "" {
		return false
	}
	if c.notifExpiry.IsZero() {
		return true
	}
	return time.Now().Before(c.notifExpiry)
}

// Image renderers
func (c *Coordinator) ImgRenderer() *termimg.Renderer {
	return c.imgRenderer
}

func (c *Coordinator) PlaybackImgRenderer() *termimg.Renderer {
	return c.playbackImgRenderer
}

// Text input management
func (c *Coordinator) UsernameInput() textinput.Model {
	return c.usernameInput
}

func (c *Coordinator) PasswordInput() textinput.Model {
	return c.passwordInput
}

func (c *Coordinator) SetUsernameInput(input textinput.Model) {
	c.usernameInput = input
}

func (c *Coordinator) SetPasswordInput(input textinput.Model) {
	c.passwordInput = input
}

func (c *Coordinator) FocusIndex() int {
	return c.focusIndex
}

func (c *Coordinator) SetFocusIndex(idx int) {
	c.focusIndex = idx
}

func (c *Coordinator) GetInput(index int) textinput.Model {
	if index == 0 {
		return c.usernameInput
	}
	return c.passwordInput
}

func (c *Coordinator) UpdateInput(index int, input textinput.Model) {
	if index == 0 {
		c.usernameInput = input
	} else {
		c.passwordInput = input
	}
}

// Navigation helpers
func (c *Coordinator) SelectedHome() int {
	return c.selectedHome
}

func (c *Coordinator) SetSelectedHome(idx int) {
	c.selectedHome = idx
}

// Playback stream management
func (c *Coordinator) Streamer() beep.StreamSeekCloser {
	return c.streamer
}

func (c *Coordinator) SetStreamer(s beep.StreamSeekCloser) {
	c.streamer = s
}

func (c *Coordinator) Ctrl() *beep.Ctrl {
	return c.ctrl
}

func (c *Coordinator) SetCtrl(ctrl *beep.Ctrl) {
	c.ctrl = ctrl
}

func (c *Coordinator) Volume() *effects.Volume {
	return c.volume
}

func (c *Coordinator) SetVolume(vol *effects.Volume) {
	c.volume = vol
}

func (c *Coordinator) SpeakerInit() bool {
	return c.speakerInit
}

func (c *Coordinator) SetSpeakerInit(init bool) {
	c.speakerInit = init
}

func (c *Coordinator) SampleRate() beep.SampleRate {
	return c.sampleRate
}

func (c *Coordinator) SetSampleRate(sr beep.SampleRate) {
	c.sampleRate = sr
}

func (c *Coordinator) StreamPosition() int {
	return c.streamPosition
}

func (c *Coordinator) SetStreamPosition(pos int) {
	c.streamPosition = pos
}

func (c *Coordinator) StreamLength() int {
	return c.streamLength
}

func (c *Coordinator) SetStreamLength(len int) {
	c.streamLength = len
}

// Business Logic Helpers

// IsLoggedIn checks if user is authenticated
func (c *Coordinator) IsLoggedIn() bool {
	return c.token != ""
}

// HasServers checks if any servers are available
func (c *Coordinator) HasServers() bool {
	return len(c.servers) > 0
}

// GetCurrentServer returns the currently selected server
func (c *Coordinator) GetCurrentServer() *PlexServer {
	if c.selectedServer >= 0 && c.selectedServer < len(c.servers) {
		return &c.servers[c.selectedServer]
	}
	return nil
}

// GetCurrentAlbum returns the currently selected album
func (c *Coordinator) GetCurrentAlbum() *Album {
	if c.selectedAlbum >= 0 && c.selectedAlbum < len(c.albums) {
		return &c.albums[c.selectedAlbum]
	}
	return nil
}

// GetCurrentPlaylist returns the currently selected playlist
func (c *Coordinator) GetCurrentPlaylist() *Playlist {
	if c.selectedPlaylist >= 0 && c.selectedPlaylist < len(c.playlists) {
		return &c.playlists[c.selectedPlaylist]
	}
	return nil
}

// HasTracks checks if tracks are available
func (c *Coordinator) HasTracks() bool {
	return len(c.tracks) > 0
}

// HasAlbums checks if albums are available
func (c *Coordinator) HasAlbums() bool {
	return len(c.albums) > 0
}

// HasPlaylists checks if playlists are available
func (c *Coordinator) HasPlaylists() bool {
	return len(c.playlists) > 0
}

// IsPlaying checks if playback is active
func (c *Coordinator) IsPlaying() bool {
	return c.playbackState == PlaybackPlaying
}

// IsPaused checks if playback is paused
func (c *Coordinator) IsPaused() bool {
	return c.playbackState == PlaybackPaused
}

// IsStopped checks if playback is stopped
func (c *Coordinator) IsStopped() bool {
	return c.playbackState == PlaybackStopped
}

// HasCurrentTrack checks if a track is currently playing
func (c *Coordinator) HasCurrentTrack() bool {
	return c.currentTrack != nil
}

// NextTab cycles to the next tab
func (c *Coordinator) NextTab() {
	c.activeTab++
	if c.activeTab > SettingsTab {
		c.activeTab = HomeTab
	}
}

// PreviousTab cycles to the previous tab
func (c *Coordinator) PreviousTab() {
	c.activeTab--
	if c.activeTab < HomeTab {
		c.activeTab = SettingsTab
	}
}

// ClearTracks clears the track list and resets selection
func (c *Coordinator) ClearTracks() {
	c.tracks = nil
	c.selectedTrack = 0
}

// ClearQueue clears the queue and resets position
func (c *Coordinator) ClearQueue() {
	c.queue = nil
	c.queueIndex = 0
}

// PlaybackService accessors
func (c *Coordinator) PlaybackService() *service.PlaybackService {
	return c.playbackService
}

func (c *Coordinator) SetPlaybackService(s *service.PlaybackService) {
	c.playbackService = s
}
