package app

import (
	"context"
	"fmt"
	"image"
	"net/http"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"

	"plexmusic-tui/internal/auth"
	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	termimg "plexmusic-tui/internal/image"
	"plexmusic-tui/internal/service"
)

// Coordinator is the application's canonical state container.
type Coordinator struct {
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
	albums         []domain.Album
	albumsTotal    int
	artistsTotal   int
	playlists      []domain.Playlist
	playlistsTotal int
	tracks         []domain.Track
	tracksTotal    int
	queue          []domain.Track
	queueIndex     int

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
	imgRenderer         domain.ImageRenderer
	playbackImgRenderer domain.ImageRenderer

	// Text inputs for login
	usernameInput textinput.Model
	passwordInput textinput.Model
	focusIndex    int

	// Notification message for transient UI notifications
	notifMsg      string
	notifSeverity string
	notifExpiry   time.Time

	// Playback & stream state - mirror of legacy coordinator (to preserve behavior)
	playbackState  PlaybackState
	currentTrack   *Track
	streamer       beep.StreamSeekCloser
	ctrl           *beep.Ctrl
	volume         *effects.Volume
	speakerInit    bool
	sampleRate     beep.SampleRate
	streamPosition int
	streamLength   int

	// Config Manager for persistence
	configMgr *config.Manager
	// Debug/troubleshooting
	dumpView bool
	// Plex Pass and sonic-enhanced contents
	plexPass     bool
	mixesForYou  []domain.Playlist
	onThisDay    []domain.Album
	moodStations []domain.Track
	libraryHubs  []domain.Hub
	// Indicates the server has sonic analysis available for at least some items
	sonicAvailable bool
}

// NewCoordinatorWithServices creates a new coordinator with service dependencies
func NewCoordinatorWithServices(
	authSvc service.AuthServicer,
	librarySvc service.LibraryServicer,
	playbackSvc service.PlaybackServicer,
	forceProtocol *domain.Protocol,
	renderDebug bool,
	dumpView bool,
) *Coordinator {
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

	c := &Coordinator{
		authService:         authSvc,
		libraryService:      librarySvc,
		playbackService:     playbackSvc,
		ctx:                 ctx,
		cancel:              cancel,
		sessionState:        LoginView,
		usernameInput:       usernameInput,
		passwordInput:       passwordInput,
		imgRenderer:         termimg.NewRenderer(),
		playbackImgRenderer: termimg.NewRendererWithProtocol(domain.ProtocolUnicodeBlocks),
		dumpView:            dumpView,
	}
	// If forcing a protocol, create a renderer with that protocol instead
	if forceProtocol != nil {
		c.imgRenderer = termimg.NewRendererWithProtocol(*forceProtocol)
	}
	if renderDebug {
		c.imgRenderer.SetDebug(true)
		c.playbackImgRenderer.SetDebug(true)
	}
	return c
}

// NewCoordinator creates a Coordinator wired with default services that are
// suitable for app runtime. Tests can still call NewCoordinatorWithServices
// to inject test doubles.
func NewCoordinator() *Coordinator {
	authGateway := auth.NewAuthenticator(&http.Client{})
	authSvc := service.NewAuthService(authGateway)
	var libSvc service.LibraryServicer = nil
	// Playback service should be instantiated as a pointer-backed service
	// so we can satisfy callers expecting a concrete pointer.
	pbSvc := service.NewPlaybackService()
	return NewCoordinatorWithServices(authSvc, libSvc, pbSvc, nil, false, false)
}

// Close releases resources and cancels event subscriptions
func (c *Coordinator) Close() error {
	c.cancel()
	// Services will handle their own cleanup via context
	return nil
}

// Context returns the coordinator's context for event subscriptions
func (c *Coordinator) Context() context.Context {
	return c.ctx
}

// Service accessors
func (c *Coordinator) AuthService() service.AuthServicer {
	return c.authService
}

func (c *Coordinator) LibraryService() service.LibraryServicer {
	return c.libraryService
}

func (c *Coordinator) PlaybackService() service.PlaybackServicer {
	return c.playbackService
}

// Additional compat accessors required by Coordinatorer interface
var _ Coordinatorer = (*Coordinator)(nil)

// ConfigManager setter (not part of interface but used by main.go)
func (c *Coordinator) SetConfigManager(cfg *config.Manager) {
	c.configMgr = cfg
}

// SetDumpView toggles writing raw Page views to the debug dump file
// for debugging. This can be toggled at app start via CLI flag or by the UI.
func (c *Coordinator) SetDumpView(v bool) {
	c.dumpView = v
}

func (c *Coordinator) DumpView() bool {
	return c.dumpView
}

func (c *Coordinator) ConfigManager() *config.Manager {
	return c.configMgr
}

// Playback service concrete wiring
func (c *Coordinator) SetPlaybackService(s service.PlaybackServicer) {
	c.playbackService = s
}

func (c *Coordinator) PlaybackServicePtr() service.PlaybackServicer {
	return c.playbackService
}

// Error helpers
func (c *Coordinator) Error() error {
	return c.err
}

func (c *Coordinator) SetError(err error) {
	c.err = err
}

// Tab helpers: advance or go back one tab (wraps around)
func (c *Coordinator) NextTab() {
	max := int(SettingsTab)
	next := (int(c.activeTab) + 1) % (max + 1)
	c.activeTab = TabType(next)
}

func (c *Coordinator) PreviousTab() {
	max := int(SettingsTab)
	prev := int(c.activeTab) - 1
	if prev < 0 {
		prev = max
	}
	c.activeTab = TabType(prev)
}

// State accessors - reuse patterns from coordinator.go
// (Only showing subset - full implementation would mirror coordinator.go)

func (c *Coordinator) SessionState() SessionState {
	return c.sessionState
}

func (c *Coordinator) SetSessionState(s SessionState) {
	c.sessionState = s
}

func (c *Coordinator) Token() string {
	return c.token
}

func (c *Coordinator) SetToken(token string) {
	c.token = token
}

func (c *Coordinator) IsLoggedIn() bool {
	return c.token != ""
}

func (c *Coordinator) GetToken() string {
	return c.token
}

func (c *Coordinator) GetCurrentServer() *PlexServer {
	if c.selectedServer >= 0 && c.selectedServer < len(c.servers) {
		s := c.servers[c.selectedServer]
		return &PlexServer{
			Name:         s.Name,
			Host:         s.Host,
			Port:         s.Port,
			AccessToken:  s.AccessToken,
			LocalAddress: s.LocalAddress,
			Scheme:       s.Scheme,
		}
	}
	return nil
}

// Libraries
func (c *Coordinator) Libraries() []MusicLibrary {
	out := make([]MusicLibrary, len(c.libraries))
	for i, l := range c.libraries {
		out[i] = MusicLibrary{Key: l.Key, Title: l.Title, Type: l.Type}
	}
	return out
}

func (c *Coordinator) SetLibraries(libs []MusicLibrary) {
	out := make([]domain.MusicLibrary, len(libs))
	for i, l := range libs {
		out[i] = domain.MusicLibrary{Key: l.Key, Title: l.Title, Type: l.Type}
	}
	c.libraries = out
}

func (c *Coordinator) SelectedLibrary() int       { return c.selectedLibrary }
func (c *Coordinator) SetSelectedLibrary(idx int) { c.selectedLibrary = idx }

// Albums
func (c *Coordinator) Albums() []Album {
	out := make([]Album, len(c.albums))
	for i, a := range c.albums {
		out[i] = Album{Title: a.Title, Artist: a.Artist, Year: a.Year, Key: a.Key, Thumb: a.Thumb}
	}
	return out
}

func (c *Coordinator) SetAlbums(albums []Album) {
	out := make([]domain.Album, len(albums))
	for i, a := range albums {
		out[i] = domain.Album{
			Title:  a.Title,
			Artist: a.Artist,
			Year:   a.Year,
			Key:    a.Key,
			Thumb:  a.Thumb,
		}
	}
	c.albums = out
}

func (c *Coordinator) AlbumsTotal() int         { return c.albumsTotal }
func (c *Coordinator) SetAlbumsTotal(total int) { c.albumsTotal = total }

func (c *Coordinator) ArtistsTotal() int         { return c.artistsTotal }
func (c *Coordinator) SetArtistsTotal(total int) { c.artistsTotal = total }

func (c *Coordinator) SelectedAlbum() int       { return c.selectedAlbum }
func (c *Coordinator) SetSelectedAlbum(idx int) { c.selectedAlbum = idx }

// Playlists
func (c *Coordinator) Playlists() []Playlist {
	out := make([]Playlist, len(c.playlists))
	for i, p := range c.playlists {
		out[i] = Playlist{
			Title:        p.Title,
			Key:          p.Key,
			LeafCount:    p.LeafCount,
			Duration:     p.Duration,
			PlaylistType: p.PlaylistType,
		}
	}
	return out
}

func (c *Coordinator) SetPlaylists(playlists []Playlist) {
	out := make([]domain.Playlist, len(playlists))
	for i, p := range playlists {
		out[i] = domain.Playlist{
			Title:        p.Title,
			Key:          p.Key,
			LeafCount:    p.LeafCount,
			Duration:     p.Duration,
			PlaylistType: p.PlaylistType,
		}
	}
	c.playlists = out
}

func (c *Coordinator) PlaylistsTotal() int         { return c.playlistsTotal }
func (c *Coordinator) SetPlaylistsTotal(total int) { c.playlistsTotal = total }

func (c *Coordinator) SelectedPlaylist() int       { return c.selectedPlaylist }
func (c *Coordinator) SetSelectedPlaylist(idx int) { c.selectedPlaylist = idx }

// Plex Pass flag
func (c *Coordinator) HasPlexPass() bool        { return c.plexPass }
func (c *Coordinator) SetPlexPass(enabled bool) { c.plexPass = enabled }

// Stations (formerly Mixes For You)
func (c *Coordinator) MixesForYou() []Playlist {
	out := make([]Playlist, len(c.mixesForYou))
	for i, p := range c.mixesForYou {
		out[i] = Playlist{
			Title:        p.Title,
			Key:          p.Key,
			LeafCount:    p.LeafCount,
			Duration:     p.Duration,
			PlaylistType: p.PlaylistType,
		}
	}
	return out
}

func (c *Coordinator) SetMixesForYou(mixes []Playlist) {
	out := make([]domain.Playlist, len(mixes))
	for i, p := range mixes {
		out[i] = domain.Playlist{
			Title:        p.Title,
			Key:          p.Key,
			LeafCount:    p.LeafCount,
			Duration:     p.Duration,
			PlaylistType: p.PlaylistType,
		}
	}
	c.mixesForYou = out
}

// OnThisDay
func (c *Coordinator) OnThisDay() []Album {
	out := make([]Album, len(c.onThisDay))
	for i, a := range c.onThisDay {
		out[i] = Album{Title: a.Title, Artist: a.Artist, Year: a.Year, Key: a.Key, Thumb: a.Thumb}
	}
	return out
}

func (c *Coordinator) SetOnThisDay(albums []Album) {
	out := make([]domain.Album, len(albums))
	for i, a := range albums {
		out[i] = domain.Album{Title: a.Title, Artist: a.Artist, Year: a.Year, Key: a.Key, Thumb: a.Thumb}
	}
	c.onThisDay = out
}

// MoodStations
func (c *Coordinator) MoodStations() []Track {
	out := make([]Track, len(c.moodStations))
	for i, t := range c.moodStations {
		out[i] = Track{
			Title:       t.Title,
			Artist:      t.Artist,
			Album:       t.Album,
			Duration:    t.Duration,
			TrackNumber: t.TrackNumber,
			Key:         t.Key,
			RatingKey:   t.RatingKey,
			Thumb:       t.Thumb,
		}
	}
	return out
}

func (c *Coordinator) SetMoodStations(tracks []Track) {
	out := make([]domain.Track, len(tracks))
	for i, t := range tracks {
		out[i] = domain.Track{
			Title:       t.Title,
			Artist:      t.Artist,
			Album:       t.Album,
			Duration:    t.Duration,
			TrackNumber: t.TrackNumber,
			Key:         t.Key,
			RatingKey:   t.RatingKey,
			Thumb:       t.Thumb,
		}
	}
	c.moodStations = out
}

// LibraryHubs returns all available hubs for the current library
func (c *Coordinator) LibraryHubs() []Hub {
	out := make([]Hub, len(c.libraryHubs))
	for i, h := range c.libraryHubs {
		// Convert playlists
		playlists := make([]Playlist, len(h.Playlists))
		for j, p := range h.Playlists {
			playlists[j] = Playlist{
				Title:        p.Title,
				Key:          p.Key,
				LeafCount:    p.LeafCount,
				Duration:     p.Duration,
				PlaylistType: p.PlaylistType,
			}
		}
		// Convert albums
		albums := make([]Album, len(h.Albums))
		for j, a := range h.Albums {
			albums[j] = Album{Title: a.Title, Artist: a.Artist, Year: a.Year, Key: a.Key, Thumb: a.Thumb}
		}
		out[i] = Hub{
			HubIdentifier: h.HubIdentifier,
			Title:         h.Title,
			Type:          h.Type,
			Context:       h.Context,
			Size:          h.Size,
			Playlists:     playlists,
			Albums:        albums,
		}
	}
	return out
}

func (c *Coordinator) SetLibraryHubs(hubs []Hub) {
	out := make([]domain.Hub, len(hubs))
	for i, h := range hubs {
		// Convert playlists
		playlists := make([]domain.Playlist, len(h.Playlists))
		for j, p := range h.Playlists {
			playlists[j] = domain.Playlist{
				Title:        p.Title,
				Key:          p.Key,
				LeafCount:    p.LeafCount,
				Duration:     p.Duration,
				PlaylistType: p.PlaylistType,
			}
		}
		// Convert albums
		albums := make([]domain.Album, len(h.Albums))
		for j, a := range h.Albums {
			albums[j] = domain.Album{Title: a.Title, Artist: a.Artist, Year: a.Year, Key: a.Key, Thumb: a.Thumb}
		}
		out[i] = domain.Hub{
			HubIdentifier: h.HubIdentifier,
			Title:         h.Title,
			Type:          h.Type,
			Context:       h.Context,
			Size:          h.Size,
			Playlists:     playlists,
			Albums:        albums,
		}
	}
	c.libraryHubs = out
}

// Sonic analysis available flag
func (c *Coordinator) HasSonicAvailable() bool        { return c.sonicAvailable }
func (c *Coordinator) SetSonicAvailable(enabled bool) { c.sonicAvailable = enabled }

// Tracks
func (c *Coordinator) Tracks() []Track {
	out := make([]Track, len(c.tracks))
	for i, t := range c.tracks {
		out[i] = Track{
			Title:          t.Title,
			Artist:         t.Artist,
			Album:          t.Album,
			Duration:       t.Duration,
			TrackNumber:    t.TrackNumber,
			PlaylistItemID: t.PlaylistItemID,
			Key:            t.Key,
			RatingKey:      t.RatingKey,
			Thumb:          t.Thumb,
		}
		if len(t.Media) > 0 {
			out[i].Media = make([]struct {
				Part []struct {
					Key string
				}
			}, len(t.Media))
			for j, m := range t.Media {
				if len(m.Part) > 0 {
					out[i].Media[j].Part = make([]struct {
						Key string
					}, len(m.Part))
					for k, p := range m.Part {
						out[i].Media[j].Part[k].Key = p.Key
					}
				}
			}
		}
	}
	return out
}

func (c *Coordinator) SetTracks(tracks []Track) {
	out := make([]domain.Track, len(tracks))
	for i, t := range tracks {
		out[i] = domain.Track{
			Title:          t.Title,
			Artist:         t.Artist,
			Album:          t.Album,
			Duration:       t.Duration,
			TrackNumber:    t.TrackNumber,
			PlaylistItemID: t.PlaylistItemID,
			Key:            t.Key,
			RatingKey:      t.RatingKey,
			Thumb:          t.Thumb,
		}
		if len(t.Media) > 0 {
			out[i].Media = make([]struct {
				Part []struct {
					Key string `json:"key"`
				} `json:"Part"`
			}, len(t.Media))
			for j, m := range t.Media {
				if len(m.Part) > 0 {
					out[i].Media[j].Part = make([]struct {
						Key string `json:"key"`
					}, len(m.Part))
					for k, p := range m.Part {
						out[i].Media[j].Part[k].Key = p.Key
					}
				}
			}
		}
	}
	c.tracks = out
}

func (c *Coordinator) TracksTotal() int         { return c.tracksTotal }
func (c *Coordinator) SetTracksTotal(total int) { c.tracksTotal = total }

func (c *Coordinator) SelectedTrack() int       { return c.selectedTrack }
func (c *Coordinator) SetSelectedTrack(idx int) { c.selectedTrack = idx }

func (c *Coordinator) Queue() []Track {
	out := make([]Track, len(c.queue))
	for i, t := range c.queue {
		out[i] = Track{
			Title:          t.Title,
			Artist:         t.Artist,
			Album:          t.Album,
			Duration:       t.Duration,
			TrackNumber:    t.TrackNumber,
			PlaylistItemID: t.PlaylistItemID,
			Key:            t.Key,
			RatingKey:      t.RatingKey,
			Thumb:          t.Thumb,
		}
		if len(t.Media) > 0 {
			out[i].Media = make([]struct {
				Part []struct {
					Key string
				}
			}, len(t.Media))
			for j, m := range t.Media {
				if len(m.Part) > 0 {
					out[i].Media[j].Part = make([]struct {
						Key string
					}, len(m.Part))
					for k, p := range m.Part {
						out[i].Media[j].Part[k].Key = p.Key
					}
				}
			}
		}
	}
	return out
}

func (c *Coordinator) SetQueue(queue []Track) {
	out := make([]domain.Track, len(queue))
	for i, t := range queue {
		out[i] = domain.Track{
			Title:          t.Title,
			Artist:         t.Artist,
			Album:          t.Album,
			Duration:       t.Duration,
			TrackNumber:    t.TrackNumber,
			PlaylistItemID: t.PlaylistItemID,
			Key:            t.Key,
			RatingKey:      t.RatingKey,
			Thumb:          t.Thumb,
		}
		if len(t.Media) > 0 {
			out[i].Media = make([]struct {
				Part []struct {
					Key string `json:"key"`
				} `json:"Part"`
			}, len(t.Media))
			for j, m := range t.Media {
				if len(m.Part) > 0 {
					out[i].Media[j].Part = make([]struct {
						Key string `json:"key"`
					}, len(m.Part))
					for k, p := range m.Part {
						out[i].Media[j].Part[k].Key = p.Key
					}
				}
			}
		}
	}
	c.queue = out
}

func (c *Coordinator) QueueIndex() int       { return c.queueIndex }
func (c *Coordinator) SetQueueIndex(idx int) { c.queueIndex = idx }

// UI & navigation
func (c *Coordinator) ActiveTab() TabType                  { return c.activeTab }
func (c *Coordinator) SetActiveTab(t TabType)              { c.activeTab = t }
func (c *Coordinator) CurrentContent() ContentViewType     { return c.currentContent }
func (c *Coordinator) SetCurrentContent(v ContentViewType) { c.currentContent = v }
func (c *Coordinator) ShowQueueModal() bool                { return c.showQueueModal }
func (c *Coordinator) SetShowQueueModal(s bool)            { c.showQueueModal = s }
func (c *Coordinator) ContentScroll() int                  { return c.contentScroll }
func (c *Coordinator) SetContentScroll(scroll int)         { c.contentScroll = scroll }

// Dimensions
func (c *Coordinator) Width() int      { return c.width }
func (c *Coordinator) SetWidth(w int)  { c.width = w }
func (c *Coordinator) Height() int     { return c.height }
func (c *Coordinator) SetHeight(h int) { c.height = h }

// Login inputs
func (c *Coordinator) UsernameInput() textinput.Model         { return c.usernameInput }
func (c *Coordinator) PasswordInput() textinput.Model         { return c.passwordInput }
func (c *Coordinator) SetUsernameInput(input textinput.Model) { c.usernameInput = input }
func (c *Coordinator) SetPasswordInput(input textinput.Model) { c.passwordInput = input }
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
func (c *Coordinator) FocusIndex() int       { return c.focusIndex }
func (c *Coordinator) SetFocusIndex(idx int) { c.focusIndex = idx }

// Image renderers
func (c *Coordinator) ImgRenderer() domain.ImageRenderer         { return c.imgRenderer }
func (c *Coordinator) PlaybackImgRenderer() domain.ImageRenderer { return c.playbackImgRenderer }

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

// Playback/stream state
func (c *Coordinator) PlaybackState() PlaybackState     { return c.playbackState }
func (c *Coordinator) SetPlaybackState(s PlaybackState) { c.playbackState = s }
func (c *Coordinator) CurrentTrack() *Track             { return c.currentTrack }
func (c *Coordinator) SetCurrentTrack(t *Track)         { c.currentTrack = t }
func (c *Coordinator) HasCurrentTrack() bool            { return c.currentTrack != nil }

func (c *Coordinator) IsPlaying() bool { return c.playbackState == PlaybackPlaying }
func (c *Coordinator) IsPaused() bool  { return c.playbackState == PlaybackPaused }

func (c *Coordinator) IsStopped() bool { return c.playbackState == PlaybackStopped }

// Stream/position info
func (c *Coordinator) StreamPosition() int                 { return c.streamPosition }
func (c *Coordinator) SetStreamPosition(pos int)           { c.streamPosition = pos }
func (c *Coordinator) StreamLength() int                   { return c.streamLength }
func (c *Coordinator) SetStreamLength(l int)               { c.streamLength = l }
func (c *Coordinator) SampleRate() beep.SampleRate         { return c.sampleRate }
func (c *Coordinator) SetSampleRate(sr beep.SampleRate)    { c.sampleRate = sr }
func (c *Coordinator) Volume() *effects.Volume             { return c.volume }
func (c *Coordinator) SetVolume(vol *effects.Volume)       { c.volume = vol }
func (c *Coordinator) Streamer() beep.StreamSeekCloser     { return c.streamer }
func (c *Coordinator) SetStreamer(s beep.StreamSeekCloser) { c.streamer = s }
func (c *Coordinator) Ctrl() *beep.Ctrl                    { return c.ctrl }
func (c *Coordinator) SetCtrl(c2 *beep.Ctrl)               { c.ctrl = c2 }
func (c *Coordinator) SpeakerInit() bool                   { return c.speakerInit }
func (c *Coordinator) SetSpeakerInit(b bool)               { c.speakerInit = b }

// Playback album art
func (c *Coordinator) PlaybackAlbumArt() image.Image { return c.playbackAlbumArt }

func (c *Coordinator) SetPlaybackAlbumArt(img image.Image, thumbURL string) {
	c.playbackAlbumArt = img
	c.playbackAlbumArtThumb = thumbURL
}
func (c *Coordinator) PlaybackAlbumArtThumb() string { return c.playbackAlbumArtThumb }

// Servers accessor/mutators convert between domain and app types
func (c *Coordinator) Servers() []PlexServer {
	out := make([]PlexServer, len(c.servers))
	for i, s := range c.servers {
		out[i] = PlexServer{
			Name:         s.Name,
			Host:         s.Host,
			Port:         s.Port,
			AccessToken:  s.AccessToken,
			LocalAddress: s.LocalAddress,
			Scheme:       s.Scheme,
		}
	}
	return out
}

func (c *Coordinator) SetServers(servers []PlexServer) {
	out := make([]domain.PlexServer, len(servers))
	for i, s := range servers {
		out[i] = domain.PlexServer{
			Name:         s.Name,
			Host:         s.Host,
			Port:         s.Port,
			AccessToken:  s.AccessToken,
			LocalAddress: s.LocalAddress,
			Scheme:       s.Scheme,
		}
	}
	c.servers = out
}

func (c *Coordinator) SelectedServer() int {
	return c.selectedServer
}

func (c *Coordinator) SetSelectedServer(idx int) {
	c.selectedServer = idx
	// If config manager exists, persist selected server canonical key
	if c.configMgr == nil {
		return
	}
	if idx < 0 || idx >= len(c.servers) {
		return
	}
	host := c.servers[idx].Host
	name := c.servers[idx].Name
	var key string
	if host == "" {
		key = name
	} else {
		key = fmt.Sprintf("%s/%s", host, name)
	}
	c.configMgr.SetLastSelectedServer(key)
	_ = c.configMgr.Save()
}

// MoveQueueItem swaps two items in the queue
func (c *Coordinator) MoveQueueItem(from, to int) {
	if from < 0 || from >= len(c.queue) || to < 0 || to >= len(c.queue) {
		return
	}

	// Swap items
	c.queue[from], c.queue[to] = c.queue[to], c.queue[from]

	// Update queue index if necessary
	switch c.queueIndex {
	case from:
		c.queueIndex = to
	case to:
		c.queueIndex = from
	}
}

// RemoveQueueItem removes an item from the queue
func (c *Coordinator) RemoveQueueItem(index int) {
	if index < 0 || index >= len(c.queue) {
		return
	}

	// Remove item
	c.queue = append(c.queue[:index], c.queue[index+1:]...)

	// Update queue index
	if c.queueIndex == index {
		// Removed currently playing item
		c.queueIndex = -1
		c.SetPlaybackState(PlaybackStopped)
		c.SetCurrentTrack(nil)
	} else if c.queueIndex > index {
		c.queueIndex--
	}
}

// Additional accessors follow same pattern as coordinator.go...
// For brevity, showing representative examples. Full migration would
// copy all accessor/mutator patterns.
