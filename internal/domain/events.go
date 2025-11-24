package domain

// LibraryEvent represents library-related events
type LibraryEvent struct {
	Type      string
	Libraries []MusicLibrary
	Albums    []Album
	Playlists []Playlist
	Tracks    []Track
	TotalSize int
	Error     error
}

// AuthEvent represents authentication-related events
type AuthEvent struct {
	Type    string
	Token   string
	Servers []PlexServer
	Error   error
}

// PlaybackEvent represents playback-related events
type PlaybackEvent struct {
	Type       string
	Track      *Track
	State      PlaybackState
	Position   int
	Duration   int
	SampleRate int
	Volume     float64
	Error      error
}
