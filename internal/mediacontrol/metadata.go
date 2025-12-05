package mediacontrol

import "time"

// Metadata represents track metadata for OS media controls
type Metadata struct {
	// Title is the track title
	Title string

	// Artist is the track artist
	Artist string

	// Album is the album name
	Album string

	// Duration is the track duration
	Duration time.Duration

	// AlbumArtist is the album artist (optional)
	AlbumArtist string

	// TrackNumber is the track number in the album (optional)
	TrackNumber int

	// Year is the release year (optional)
	Year int
}

// PlaybackState represents the current playback state
type PlaybackState int

const (
	// StateStopped indicates playback is stopped
	StateStopped PlaybackState = iota

	// StatePlaying indicates playback is active
	StatePlaying

	// StatePaused indicates playback is paused
	StatePaused
)

// String returns a string representation of the playback state
func (s PlaybackState) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StatePlaying:
		return "playing"
	case StatePaused:
		return "paused"
	default:
		return "unknown"
	}
}
