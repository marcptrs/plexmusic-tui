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
	SetBaseURL(baseURL string)
	SetToken(token string)
	FetchSectionCounts(ctx context.Context, sectionKey string) (int, int, int, error)
	FetchImage(ctx context.Context, path string) (image.Image, error)
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
	GetState() domain.PlaybackState
	PlayDomainTrack(ctx context.Context, lib interface {
		FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error)
	}, track *domain.Track) error
}
