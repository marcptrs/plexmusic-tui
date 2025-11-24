package app

import (
	"image"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"

	"plexmusic-tui/internal/config"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/service"
)

// Coordinatorer defines the public methods pages and components use to
// access and update application state. Start small and expand as pages
// require additional methods — keep it interface-led to allow testing.
type Coordinatorer interface {
	// Session / auth
	SessionState() SessionState
	SetSessionState(SessionState)
	GetToken() string
	SetToken(token string)
	IsLoggedIn() bool

	// Servers & libraries
	Servers() []PlexServer
	SetServers([]PlexServer)
	SelectedServer() int
	SetSelectedServer(idx int)
	Libraries() []MusicLibrary
	SetLibraries([]MusicLibrary)
	SelectedLibrary() int
	SetSelectedLibrary(idx int)

	// Content collections
	Albums() []Album
	SetAlbums([]Album)
	AlbumsTotal() int
	SetAlbumsTotal(int)
	ArtistsTotal() int
	SetArtistsTotal(int)
	SelectedAlbum() int
	SetSelectedAlbum(idx int)
	Tracks() []Track
	SetTracks([]Track)
	TracksTotal() int
	SetTracksTotal(int)
	SelectedTrack() int
	SetSelectedTrack(idx int)
	Playlists() []Playlist
	SetPlaylists([]Playlist)
	PlaylistsTotal() int
	SetPlaylistsTotal(int)
	SelectedPlaylist() int
	SetSelectedPlaylist(idx int)
	Queue() []Track
	SetQueue([]Track)
	QueueIndex() int
	SetQueueIndex(idx int)

	// UI & navigation
	ActiveTab() TabType
	SetActiveTab(TabType)
	CurrentContent() ContentViewType
	SetCurrentContent(ContentViewType)
	ShowQueueModal() bool
	SetShowQueueModal(show bool)
	ContentScroll() int
	SetContentScroll(scroll int)

	// Dimensions
	Width() int
	SetWidth(w int)
	Height() int
	SetHeight(h int)

	// Login inputs
	UsernameInput() textinput.Model
	PasswordInput() textinput.Model
	SetUsernameInput(input textinput.Model)
	SetPasswordInput(input textinput.Model)
	GetInput(index int) textinput.Model
	UpdateInput(index int, input textinput.Model)
	FocusIndex() int
	SetFocusIndex(idx int)

	// Image renderers
	ImgRenderer() domain.ImageRenderer
	PlaybackImgRenderer() domain.ImageRenderer

	// Notifications
	SetNotification(msg, severity string, duration time.Duration)
	Notification() (string, string, time.Time)
	ClearNotification()
	NotificationActive() bool

	// Playback service (singleton wiring)
	PlaybackService() service.PlaybackServicer
	SetPlaybackService(service.PlaybackServicer)

	// Playback/stream state (mapped from old coordinator)
	PlaybackState() PlaybackState
	SetPlaybackState(PlaybackState)
	CurrentTrack() *Track
	SetCurrentTrack(track *Track)
	HasCurrentTrack() bool
	IsPlaying() bool
	IsPaused() bool
	IsStopped() bool

	// Stream/position info
	StreamPosition() int
	SetStreamPosition(pos int)
	StreamLength() int
	SetStreamLength(len int)
	SampleRate() beep.SampleRate
	SetSampleRate(sr beep.SampleRate)
	Volume() *effects.Volume
	SetVolume(vol *effects.Volume)
	Streamer() beep.StreamSeekCloser
	SetStreamer(s beep.StreamSeekCloser)
	Ctrl() *beep.Ctrl
	SetCtrl(c *beep.Ctrl)
	SpeakerInit() bool
	SetSpeakerInit(bool)

	// Playback album art
	PlaybackAlbumArt() image.Image
	SetPlaybackAlbumArt(img image.Image, thumbURL string)
	PlaybackAlbumArtThumb() string

	// Server and helper accessors
	GetCurrentServer() *PlexServer
	ConfigManager() *config.Manager
	// Debug/troubleshooting toggles
	SetDumpView(bool)
	DumpView() bool

	// Queue manipulation
	MoveQueueItem(from, to int)
	RemoveQueueItem(index int)
}
