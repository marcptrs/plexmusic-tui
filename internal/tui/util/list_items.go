package util

import (
	"fmt"

	"plexmusic-tui/internal/domain"
)

// AlbumItem adapts domain.Album to list.Item
type AlbumItem struct {
	Album domain.Album
}

func (i AlbumItem) Title() string { return i.Album.Title }

func (i AlbumItem) Description() string { return fmt.Sprintf("%s (%d)", i.Album.Artist, i.Album.Year) }
func (i AlbumItem) FilterValue() string { return i.Album.Title + " " + i.Album.Artist }

// PlaylistItem adapts domain.Playlist to list.Item
type PlaylistItem struct {
	Playlist domain.Playlist
}

func (i PlaylistItem) Title() string       { return i.Playlist.Title }
func (i PlaylistItem) Description() string { return fmt.Sprintf("%d items", i.Playlist.LeafCount) }
func (i PlaylistItem) FilterValue() string { return i.Playlist.Title }

// TrackItem adapts domain.Track to list.Item
type TrackItem struct {
	Track   domain.Track
	Playing bool
}

func (i TrackItem) Title() string {
	// Return raw title string; delegate will apply styles centrally.
	return i.Track.Title
}

func (i TrackItem) Description() string {
	return fmt.Sprintf("%s - %s", i.Track.Artist, i.Track.Album)
}
func (i TrackItem) FilterValue() string { return i.Track.Title + " " + i.Track.Artist }

// QueueItem adapts domain.Track to list.Item for the queue
type QueueItem struct {
	Track   domain.Track
	Index   int
	Playing bool
}

func (i QueueItem) Title() string {
	return i.Track.Title
}

func (i QueueItem) Description() string {
	// Return formatted track duration as description
	return FormatTrackDuration(i.Track.Duration)
}

// Expose playing state so delegate can render appropriately
func (i TrackItem) IsPlaying() bool {
	return i.Playing
}

// Expose playing state for queue items
func (i QueueItem) IsPlaying() bool {
	return i.Playing
}
func (i QueueItem) FilterValue() string { return i.Track.Title + " " + i.Track.Artist }
