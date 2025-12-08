package app

import (
	"sync"

	"plexmusic-tui/internal/domain"
)

// ContentState holds all domain data collections cached from the Plex server.
// This includes albums, playlists, tracks, queue, and Plex-specific features.
type ContentState struct {
	// Content collections
	albums          []domain.Album
	albumsTotal     int
	artistsTotal    int
	playlists       []domain.Playlist
	playlistsTotal  int
	tracks          []domain.Track
	tracksTotal     int
	queue           []domain.Track
	queueIndex      int
	activePlayQueue *domain.ActivePlayQueue

	// Plex Pass and sonic-enhanced content
	plexPass       bool
	mixesForYou    []domain.Playlist
	onThisDay      []domain.Album
	moodStations   []domain.Track
	libraryHubs    []domain.Hub
	sonicAvailable bool

	// Recently played artists (limited to 10 most recent)
	recentlyPlayedArtists []Artist

	// Album art cache (separate from playback art)
	currentAlbumArt      interface{} // image.Image
	currentAlbumArtThumb string

	mu sync.RWMutex
}

// NewContentState creates a new content state
func NewContentState() *ContentState {
	return &ContentState{
		queue: []domain.Track{},
	}
}

// Albums

func (c *ContentState) Albums() []domain.Album {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.albums
}

func (c *ContentState) SetAlbums(albums []domain.Album) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.albums = albums
}

func (c *ContentState) AlbumsTotal() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.albumsTotal
}

func (c *ContentState) SetAlbumsTotal(total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.albumsTotal = total
}

func (c *ContentState) ArtistsTotal() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.artistsTotal
}

func (c *ContentState) SetArtistsTotal(total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.artistsTotal = total
}

// Playlists

func (c *ContentState) Playlists() []domain.Playlist {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.playlists
}

func (c *ContentState) SetPlaylists(playlists []domain.Playlist) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.playlists = playlists
}

func (c *ContentState) PlaylistsTotal() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.playlistsTotal
}

func (c *ContentState) SetPlaylistsTotal(total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.playlistsTotal = total
}

// Tracks

func (c *ContentState) Tracks() []domain.Track {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tracks
}

func (c *ContentState) SetTracks(tracks []domain.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tracks = tracks
}

func (c *ContentState) TracksTotal() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tracksTotal
}

func (c *ContentState) SetTracksTotal(total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tracksTotal = total
}

// Queue

func (c *ContentState) Queue() []domain.Track {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.queue
}

func (c *ContentState) SetQueue(queue []domain.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queue = queue
}

func (c *ContentState) AppendToQueue(tracks []domain.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queue = append(c.queue, tracks...)
}

func (c *ContentState) QueueIndex() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.queueIndex
}

func (c *ContentState) SetQueueIndex(idx int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queueIndex = idx
}

func (c *ContentState) ActivePlayQueue() *domain.ActivePlayQueue {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activePlayQueue
}

func (c *ContentState) SetActivePlayQueue(q *domain.ActivePlayQueue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activePlayQueue = q
}

func (c *ContentState) ClearActivePlayQueue() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activePlayQueue = nil
}

func (c *ContentState) IsStationPlayback() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activePlayQueue != nil && c.activePlayQueue.StationKey != ""
}

func (c *ContentState) MoveQueueItem(from, to int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if from < 0 || from >= len(c.queue) || to < 0 || to >= len(c.queue) {
		return
	}
	item := c.queue[from]
	c.queue = append(c.queue[:from], c.queue[from+1:]...)
	c.queue = append(c.queue[:to], append([]domain.Track{item}, c.queue[to:]...)...)

	// Adjust queue index
	if c.queueIndex == from {
		c.queueIndex = to
	} else if from < c.queueIndex && to >= c.queueIndex {
		c.queueIndex--
	} else if from > c.queueIndex && to <= c.queueIndex {
		c.queueIndex++
	}
}

func (c *ContentState) RemoveQueueItem(index int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.queue) {
		return
	}
	c.queue = append(c.queue[:index], c.queue[index+1:]...)

	// Adjust queue index
	if c.queueIndex == index {
		c.queueIndex = -1
	} else if c.queueIndex > index {
		c.queueIndex--
	}
}

// Plex features

func (c *ContentState) HasPlexPass() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.plexPass
}

func (c *ContentState) SetPlexPass(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.plexPass = enabled
}

func (c *ContentState) MixesForYou() []domain.Playlist {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mixesForYou
}

func (c *ContentState) SetMixesForYou(playlists []domain.Playlist) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mixesForYou = playlists
}

func (c *ContentState) OnThisDay() []domain.Album {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.onThisDay
}

func (c *ContentState) SetOnThisDay(albums []domain.Album) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onThisDay = albums
}

func (c *ContentState) MoodStations() []domain.Track {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.moodStations
}

func (c *ContentState) SetMoodStations(tracks []domain.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.moodStations = tracks
}

func (c *ContentState) LibraryHubs() []domain.Hub {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.libraryHubs
}

func (c *ContentState) SetLibraryHubs(hubs []domain.Hub) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.libraryHubs = hubs
}

func (c *ContentState) HasSonicAvailable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sonicAvailable
}

func (c *ContentState) SetSonicAvailable(available bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sonicAvailable = available
}

// Recently played artists

func (c *ContentState) RecentlyPlayedArtists() []Artist {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Artist, len(c.recentlyPlayedArtists))
	copy(out, c.recentlyPlayedArtists)
	return out
}

func (c *ContentState) AddRecentlyPlayedArtist(artist Artist) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already in list
	for i, a := range c.recentlyPlayedArtists {
		if a.Key == artist.Key {
			// Move to front
			c.recentlyPlayedArtists = append(
				[]Artist{artist},
				append(c.recentlyPlayedArtists[:i], c.recentlyPlayedArtists[i+1:]...)...)
			return
		}
	}

	// Add to front
	c.recentlyPlayedArtists = append([]Artist{artist}, c.recentlyPlayedArtists...)

	// Limit to 10
	if len(c.recentlyPlayedArtists) > 10 {
		c.recentlyPlayedArtists = c.recentlyPlayedArtists[:10]
	}
}

// Album art cache (for browsing, not playback)

func (c *ContentState) CurrentAlbumArt() interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentAlbumArt
}

func (c *ContentState) CurrentAlbumArtThumb() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentAlbumArtThumb
}

func (c *ContentState) SetCurrentAlbumArt(img interface{}, thumbURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentAlbumArt = img
	c.currentAlbumArtThumb = thumbURL
}
