package domain

import (
	"encoding/json"
	"strconv"

	"github.com/faiface/beep"
)

// FlexInt is an int that can be unmarshaled from either a JSON number or string.
// Plex sometimes returns numeric fields as strings.
type FlexInt int

func (fi *FlexInt) UnmarshalJSON(data []byte) error {
	// Try as number first
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*fi = FlexInt(n)
		return nil
	}
	// Try as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*fi = 0
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		*fi = FlexInt(n)
		return nil
	}
	return nil // Default to 0
}

// SessionState represents the current state of the application
type SessionState int

const (
	LoginView SessionState = iota
	AuthenticatingView
	SuccessView
	ErrorView
	ServerSelectionView
	HomeView
	RecentlyAddedView
	PlaylistsView
	SearchView
	SettingsView
	LibrarySelectionView
	AlbumListView
	LibraryView // Unified view with panes
)

// PaneType represents the focused pane in the library page view
type PaneType int

const (
	NavigationPane PaneType = iota
	ContentPane
	DetailPane
)

// ContentViewType represents what content is being displayed
type ContentViewType int

const (
	RecentlyAddedContent ContentViewType = iota
	AlbumTracksContent
	PlaylistsContent
	PlaylistTracksContent
	SearchContent
	SettingsContent
)

// PlaybackState represents the current playback status
type PlaybackState int

const (
	PlaybackStopped PlaybackState = iota
	PlaybackPlaying
	PlaybackPaused
)

// PlaybackMsg represents different playback commands
type PlaybackMsg int

const (
	PlaybackMsgPlay PlaybackMsg = iota
	PlaybackMsgPause
	PlaybackMsgStop
	PlaybackMsgNext
	PlaybackMsgPrevious
)

// Protocol represents the supported terminal image protocols
type Protocol int

const (
	ProtocolUnicodeBlocks Protocol = iota // Fallback using Unicode half-blocks
	ProtocolKitty                         // Kitty graphics protocol
	ProtocolITerm2                        // iTerm2 inline images
	ProtocolSixel                         // Sixel graphics
)

// String returns the string representation of the protocol
func (p Protocol) String() string {
	switch p {
	case ProtocolUnicodeBlocks:
		return "UnicodeBlocks"
	case ProtocolKitty:
		return "Kitty"
	case ProtocolITerm2:
		return "iTerm2"
	case ProtocolSixel:
		return "Sixel"
	default:
		return "Unknown"
	}
}

// PlexAuthResponse is the response from Plex authentication
type PlexAuthResponse struct {
	User struct {
		AuthToken string `json:"authToken"`
	} `json:"user"`
}

// PlexServer represents a Plex Media Server
type PlexServer struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         string `json:"port"`
	AccessToken  string `json:"accessToken"`
	LocalAddress string `json:"localAddresses"`
	Scheme       string `json:"scheme"`
	// PlexPass indicates whether this server supports Plex Pass features
	PlexPass bool `json:"plexPass"`
}

// PlexResourceResponse is the response from Plex resources API
type PlexResourceResponse struct {
	Name        string `json:"name"`
	Provides    string `json:"provides"`
	AccessToken string `json:"accessToken"`
	Owned       bool   `json:"owned"`
	Connections []struct {
		Protocol string `json:"protocol"`
		Address  string `json:"address"`
		Port     int    `json:"port"`
		Local    bool   `json:"local"`
	} `json:"connections"`
}

// MusicLibrary represents a music library on a Plex server
type MusicLibrary struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// Album represents a music album
type Album struct {
	Title  string `json:"title"`
	Artist string `json:"parentTitle"`
	Year   int    `json:"year"`
	Key    string `json:"key"`
	Thumb  string `json:"thumb"` // Album cover art URL
	// Distance is the sonic distance (if part of a similarity response)
	Distance float64 `json:"distance"`
}

// Playlist represents a music playlist
type Playlist struct {
	Title        string `json:"title"`
	Key          string `json:"key"`
	LeafCount    int    `json:"leafCount"`
	Duration     int    `json:"duration"`
	PlaylistType string `json:"playlistType"`
}

// Hub represents a Plex hub (group of content like stations, mixes, etc.)
type Hub struct {
	HubIdentifier string     `json:"hubIdentifier"`
	Title         string     `json:"title"`
	Type          string     `json:"type"`
	Context       string     `json:"context"`
	Style         string     `json:"style"`
	Size          int        `json:"size"`
	Playlists     []Playlist `json:"Metadata"` // Playlists/stations in the hub
	Albums        []Album    // For album-based hubs (like On This Day)
	Tracks        []Track    // For track-based hubs
}

// Track represents a music track
type Track struct {
	Title          string `json:"title"`
	Artist         string `json:"grandparentTitle"`
	Album          string `json:"parentTitle"`
	Duration       int    `json:"duration"`
	TrackNumber    int    `json:"index"`          // Track number on original album
	PlaylistItemID int    `json:"playlistItemID"` // ID in playlist
	Key            string `json:"key"`
	RatingKey      string `json:"ratingKey"`
	Thumb          string `json:"thumb"` // Track/album cover art URL
	Media          []struct {
		Part []struct {
			Key string `json:"key"`
		} `json:"Part"`
	} `json:"Media"`
	// Sonic analysis fields
	HasSonicAnalysis     bool    `json:"hasSonicAnalysis"`
	MusicAnalysisVersion FlexInt `json:"musicAnalysisVersion"`
	Distance             float64 `json:"distance"`
	Moods                []Mood  `json:"moods"`
}

// Mood represents a detected mood tag in Plex sonic analysis
type Mood struct {
	Title string `json:"title"`
	Key   string `json:"key"`
}

// PlexMediaContainer represents a media container response from Plex
type PlexMediaContainer struct {
	Size      int            `json:"size"`
	TotalSize int            `json:"totalSize"`
	Directory []MusicLibrary `json:"Directory" xml:"Directory"`
	Metadata  []Album        `json:"Metadata"  xml:"Metadata"`
}

// PlexPlaylistContainer represents a playlist container response
type PlexPlaylistContainer struct {
	Size      int        `json:"size"`
	TotalSize int        `json:"totalSize"`
	Metadata  []Playlist `json:"Metadata"  xml:"Metadata"`
}

// PlexTrackContainer represents a track container response
type PlexTrackContainer struct {
	Size      int     `json:"size"`
	TotalSize int     `json:"totalSize"`
	Metadata  []Track `json:"Metadata"  xml:"Metadata"`
}

// PlaybackInfo holds information about current playback state
type PlaybackInfo struct {
	State          PlaybackState
	CurrentTrack   *Track
	Streamer       beep.StreamSeekCloser
	Ctrl           *beep.Ctrl
	Volume         beep.Streamer
	SpeakerInit    bool
	SampleRate     beep.SampleRate
	StreamPosition int
	StreamLength   int
}

// AppState represents the complete application state
type AppState struct {
	SessionState          SessionState
	FocusedPane           PaneType
	CurrentContent        ContentViewType
	NavMenuIndex          int
	SelectedServer        int
	SelectedHome          int
	SelectedLibrary       int
	SelectedAlbum         int
	SelectedPlaylist      int
	SelectedTrack         int
	Width                 int
	Height                int
	Token                 string
	Error                 error
	Servers               []PlexServer
	Libraries             []MusicLibrary
	Albums                []Album
	Playlists             []Playlist
	Tracks                []Track
	Playback              PlaybackInfo
	CurrentAlbumArt       interface{} // image.Image
	CurrentAlbumArtThumb  string
	PlaybackAlbumArt      interface{} // image.Image
	PlaybackAlbumArtThumb string
}

// AuthResult is the result message from authentication
type AuthResult struct {
	Token string
	Err   error
}

// ServerListResult is the result message from server fetch
type ServerListResult struct {
	Servers []PlexServer
	Err     error
}

// LibraryListResult is the result message from library fetch
type LibraryListResult struct {
	Libraries []MusicLibrary
	Err       error
}

// AlbumListResult is the result message from album fetch
type AlbumListResult struct {
	Albums []Album
	Err    error
}

// PlaylistListResult is the result message from playlist fetch
type PlaylistListResult struct {
	Playlists []Playlist
	Err       error
}

// TrackListResult is the result message from track fetch
type TrackListResult struct {
	Tracks []Track
	Err    error
}

// PlaybackStartResult is the result message from playback start
type PlaybackStartResult struct {
	Streamer beep.StreamSeekCloser
	Format   beep.Format
	Err      error
}
