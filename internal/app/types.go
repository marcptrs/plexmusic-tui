package app

// SessionState represents the current session/screen state
type SessionState int

const (
	LoginView SessionState = iota
	AuthenticatingView
	SuccessView
	ErrorView
	ServerSelectionView
	LibraryView
)

// ContentViewType represents different content views within the library page
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

// TabType represents the different tabs in library page view
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

// MusicLibrary represents a music library in Plex (app-facing type)
type MusicLibrary struct {
	Key   string
	Title string
	Type  string
}

// Album represents an album (app-facing type)
type Album struct {
	Title  string
	Artist string
	Year   int
	Key    string
	Thumb  string
}

// Playlist represents a playlist (app-facing type)
type Playlist struct {
	Title        string
	Key          string
	LeafCount    int
	Duration     int
	PlaylistType string
}

// Hub represents a Plex hub (group of content like stations, recommendations, etc.)
type Hub struct {
	HubIdentifier string
	Title         string
	Type          string
	Context       string
	Size          int
	Playlists     []Playlist
	Albums        []Album
}

// Track represents a music track (app-facing type)
type Track struct {
	Title           string
	Artist          string
	Album           string
	Duration        int
	TrackNumber     int
	PlaylistItemID  int
	PlayQueueItemID int // ID in playQueue (for station continuous playback)
	Key             string
	RatingKey       string
	Thumb           string
	Media           []struct {
		Part []struct {
			Key string
		}
	}
}
