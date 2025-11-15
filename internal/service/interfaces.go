package service

import (
	"context"

	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
)

// LibraryServicer provides methods to interact with Plex library data.
type LibraryServicer interface {
	pubsub.Subscriber[domain.Album]

	FetchLibraries(ctx context.Context) ([]domain.MusicLibrary, error)
	FetchAlbums(ctx context.Context, libraryKey string) ([]domain.Album, error)
	FetchRecentlyAdded(ctx context.Context) ([]domain.Album, error)
	FetchPlaylists(ctx context.Context) ([]domain.Playlist, error)
	FetchTracks(ctx context.Context, key string) ([]domain.Track, error)
	BuildStreamURL(track *domain.Track) (string, error)
	SetBaseURL(baseURL string)
	SetToken(token string)
}

// AuthServicer provides authentication methods.
type AuthServicer interface {
	pubsub.Subscriber[AuthEvent]

	AuthenticateUser(ctx context.Context, username, password string) (string, error)
	FetchServers(ctx context.Context, token string) ([]domain.PlexServer, error)
}

// PlaybackServicer manages audio playback.
type PlaybackServicer interface {
	pubsub.Subscriber[domain.PlaybackInfo]

	Play(track *domain.Track) error
	Pause()
	Resume()
	Stop()
	Seek(position int)
	SetVolume(volume float64)
	GetPosition() int
	GetDuration() int
	GetState() domain.PlaybackState
}
