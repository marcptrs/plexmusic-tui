package pages

import (
	"fmt"

	"plexmusic-tui/internal/domain"
)

// AlbumItem adapts domain.Album to list.Item
type AlbumItem struct {
	Album domain.Album
}

func (i AlbumItem) Title() string       { return i.Album.Title }
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
	Track domain.Track
}

func (i TrackItem) Title() string { return i.Track.Title }
func (i TrackItem) Description() string {
	return fmt.Sprintf("%s — %s", i.Track.Artist, i.Track.Album)
}
func (i TrackItem) FilterValue() string { return i.Track.Title + " " + i.Track.Artist }

// QueueItem adapts domain.Track to list.Item for the queue
type QueueItem struct {
	Track domain.Track
	Index int
}

func (i QueueItem) Title() string { return i.Track.Title }
func (i QueueItem) Description() string {
	return fmt.Sprintf("%s — %s", i.Track.Artist, i.Track.Album)
}
func (i QueueItem) FilterValue() string { return i.Track.Title + " " + i.Track.Artist }
