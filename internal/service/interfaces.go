package service

import (
	"context"
	"image"
	"io"

	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
)

// LibraryServicer provides methods to interact with Plex library data.
type LibraryServicer interface {
	pubsub.Subscriber[domain.LibraryEvent]

	FetchLibraries(ctx context.Context) ([]domain.MusicLibrary, int, error)
	FetchAlbums(ctx context.Context, libraryKey string) ([]domain.Album, int, error)
	FetchRecentlyAdded(ctx context.Context) ([]domain.Album, int, error)
	FetchPlaylists(ctx context.Context) ([]domain.Playlist, int, error)
	FetchTracks(ctx context.Context, key string) ([]domain.Track, int, error)
	FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error)
	BuildStreamURL(track *domain.Track) (string, error)
	SetBaseURL(baseURL string) error
	SetToken(token string) error
	FetchSectionCounts(ctx context.Context, sectionKey string) (int, int, int, error)
	FetchImage(ctx context.Context, path string) (image.Image, error)
	// HasPlexPass returns true if the connected server supports Plex Pass features
	HasPlexPass(ctx context.Context) (bool, error)
	// HasSonicAnalysis returns true if sonic analysis data is present for the server/library
	HasSonicAnalysis(ctx context.Context) (bool, error)
	// FetchSonicallySimilarTracks returns a list of tracks sonically similar to the given ratingKey
	FetchSonicallySimilarTracks(
		ctx context.Context,
		ratingKey string,
		limit int,
		maxDistance float64,
	) ([]domain.Track, int, error)
	// FetchSonicAdventure returns a sonic adventure (tracks path) between two track ratingKeys
	FetchSonicAdventure(ctx context.Context, start, end string) ([]domain.Track, int, error)
	// FetchLibraryHubs returns all hubs for a music library section (stations, mixes, etc.)
	FetchLibraryHubs(ctx context.Context, sectionKey string) ([]domain.Hub, error)
	// FetchMixesForYou returns personalized mix playlists for the current user/server (deprecated - use FetchLibraryHubs)
	FetchMixesForYou(ctx context.Context) ([]domain.Playlist, int, error)
	// FetchOnThisDay returns albums that were released on today's date in history
	FetchOnThisDay(ctx context.Context) ([]domain.Album, int, error)
	// FetchMoodStation returns tracks for a named mood station (e.g., "90s Alternative")
	FetchMoodStation(ctx context.Context, station string, limit int) ([]domain.Track, int, error)
	// StartStationPlayback creates a playQueue for a station and returns tracks + playQueue info.
	// The returned ActivePlayQueue should be stored to enable continuous playback.
	StartStationPlayback(ctx context.Context, stationKey string) ([]domain.Track, *domain.ActivePlayQueue, error)
	// RefreshPlayQueue updates and fetches tracks from an active playQueue (for station continuous playback).
	// selectedItemID is the playQueueItemID of the current track - tells Plex what's playing to get more tracks.
	// Pass 0 to just fetch current state without updating.
	// Returns all tracks currently in the playQueue and the new version number.
	RefreshPlayQueue(ctx context.Context, playQueueID int, selectedItemID int) ([]domain.Track, int, error)
	Close() error
}

// AuthServicer provides authentication methods.
type AuthServicer interface {
	pubsub.Subscriber[domain.AuthEvent]

	AuthenticateUser(ctx context.Context, username, password string) (string, error)
	FetchServers(ctx context.Context, token string) ([]domain.PlexServer, error)
}

// PlaybackServicer manages audio playback.
type PlaybackServicer interface {
	pubsub.Subscriber[domain.PlaybackEvent]

	Play(track *domain.Track) error
	Pause() error
	Resume() error
	Stop() error
	Seek(position int) error
	SetVolume(volume float64)
	GetVolume() float64
	GetPosition() int
	GetDuration() int
	SampleRate() int
	GetState() domain.PlaybackState
	PlayDomainTrack(ctx context.Context, lib interface {
		FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error)
	}, track *domain.Track) error
}

// ErrValidation represents a validation error from user input
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string {
	return e.Message
}
