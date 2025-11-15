package service

import (
	"context"

	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
)

// LibraryEvent represents library-related events
type LibraryEvent struct {
	Type      string
	Libraries []domain.MusicLibrary
	Albums    []domain.Album
	Playlists []domain.Playlist
	Tracks    []domain.Track
	Error     error
}

// LibraryServiceWithEvents wraps LibraryService with event publishing
type LibraryServiceWithEvents struct {
	*LibraryService
	broker *pubsub.Broker[LibraryEvent]
}

// NewLibraryServiceWithEvents creates a library service with event support
func NewLibraryServiceWithEvents(baseURL, token string) *LibraryServiceWithEvents {
	return &LibraryServiceWithEvents{
		LibraryService: NewLibraryService(baseURL, token),
		broker:         pubsub.NewBroker[LibraryEvent](),
	}
}

// Subscribe returns a channel for receiving library events
func (s *LibraryServiceWithEvents) Subscribe(ctx context.Context) <-chan pubsub.Event[LibraryEvent] {
	return s.broker.Subscribe(ctx)
}

// FetchLibraries fetches libraries and publishes events
func (s *LibraryServiceWithEvents) FetchLibraries(ctx context.Context) ([]domain.MusicLibrary, error) {
	libraries, err := s.LibraryService.FetchLibraries(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "libraries.fetch_failed",
			Payload: LibraryEvent{
				Type:  "libraries.fetch_failed",
				Error: err,
			},
		})
		return nil, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "libraries.loaded",
		Payload: LibraryEvent{
			Type:      "libraries.loaded",
			Libraries: libraries,
		},
	})

	return libraries, nil
}

// FetchAlbums fetches albums and publishes events
func (s *LibraryServiceWithEvents) FetchAlbums(ctx context.Context, libraryKey string) ([]domain.Album, error) {
	albums, err := s.LibraryService.FetchAlbums(ctx, libraryKey)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "albums.fetch_failed",
			Payload: LibraryEvent{
				Type:  "albums.fetch_failed",
				Error: err,
			},
		})
		return nil, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "albums.loaded",
		Payload: LibraryEvent{
			Type:   "albums.loaded",
			Albums: albums,
		},
	})

	return albums, nil
}

// FetchRecentlyAdded fetches recently added albums and publishes events
func (s *LibraryServiceWithEvents) FetchRecentlyAdded(ctx context.Context) ([]domain.Album, error) {
	albums, err := s.LibraryService.FetchRecentlyAdded(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "recently_added.fetch_failed",
			Payload: LibraryEvent{
				Type:  "recently_added.fetch_failed",
				Error: err,
			},
		})
		return nil, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "recently_added.loaded",
		Payload: LibraryEvent{
			Type:   "recently_added.loaded",
			Albums: albums,
		},
	})

	return albums, nil
}

// FetchPlaylists fetches playlists and publishes events
func (s *LibraryServiceWithEvents) FetchPlaylists(ctx context.Context) ([]domain.Playlist, error) {
	playlists, err := s.LibraryService.FetchPlaylists(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "playlists.fetch_failed",
			Payload: LibraryEvent{
				Type:  "playlists.fetch_failed",
				Error: err,
			},
		})
		return nil, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "playlists.loaded",
		Payload: LibraryEvent{
			Type:      "playlists.loaded",
			Playlists: playlists,
		},
	})

	return playlists, nil
}

// FetchTracks fetches tracks and publishes events
func (s *LibraryServiceWithEvents) FetchTracks(ctx context.Context, key string) ([]domain.Track, error) {
	tracks, err := s.LibraryService.FetchTracks(ctx, key)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "tracks.fetch_failed",
			Payload: LibraryEvent{
				Type:  "tracks.fetch_failed",
				Error: err,
			},
		})
		return nil, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "tracks.loaded",
		Payload: LibraryEvent{
			Type:   "tracks.loaded",
			Tracks: tracks,
		},
	})

	return tracks, nil
}

// Close closes the service and releases resources
func (s *LibraryServiceWithEvents) Close() error {
	s.broker.Close()
	return nil
}
