package domain

import (
	"context"
	"image"
	"io"

	"plexmusic-tui/internal/pubsub"
)

// LibraryServicer provides methods to interact with Plex library data.
type LibraryServicer interface {
	pubsub.Subscriber[LibraryEvent]

	FetchLibraries(ctx context.Context) ([]MusicLibrary, int, error)
	FetchAlbums(ctx context.Context, libraryKey string) ([]Album, int, error)
	FetchRecentlyAdded(ctx context.Context) ([]Album, int, error)
	FetchPlaylists(ctx context.Context) ([]Playlist, int, error)
	FetchTracks(ctx context.Context, key string) ([]Track, int, error)
	FetchStream(ctx context.Context, track *Track) (io.ReadCloser, string, error)
	BuildStreamURL(track *Track) (string, error)
	SetBaseURL(baseURL string) error
	SetToken(token string) error
	FetchSectionCounts(ctx context.Context, sectionKey string) (int, int, int, error)
	FetchImage(ctx context.Context, path string) (image.Image, error)
	Close() error
}

// AuthServicer provides authentication methods.
type AuthServicer interface {
	pubsub.Subscriber[AuthEvent]

	AuthenticateUser(ctx context.Context, username, password string) (string, error)
	FetchServers(ctx context.Context, token string) ([]PlexServer, error)
}

// PlaybackServicer manages audio playback.
type PlaybackServicer interface {
	pubsub.Subscriber[PlaybackEvent]

	Play(track *Track) error
	Pause() error
	Resume() error
	Stop() error
	Seek(position int) error
	SetVolume(volume float64)
	GetVolume() float64
	GetPosition() int
	GetDuration() int
	GetState() PlaybackState
	PlayDomainTrack(ctx context.Context, lib interface {
		FetchStream(ctx context.Context, track *Track) (io.ReadCloser, string, error)
	}, track *Track) error
}
