package service

import (
	"context"
	"io"

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
	FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error)
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
	pubsub.Subscriber[PlaybackEvent]

	Play(track *domain.Track) error
	Pause() error
	Resume() error
	Stop() error
	Seek(position int) error
	SetVolume(volume float64)
	GetVolume() float64
	GetPosition() int
	GetDuration() int
	GetState() domain.PlaybackState
	PlayDomainTrack(ctx context.Context, lib interface {
		FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error)
	}, track *domain.Track) error
}
