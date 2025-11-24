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
	TotalSize int
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
func (s *LibraryServiceWithEvents) Subscribe(
	ctx context.Context,
) <-chan pubsub.Event[LibraryEvent] {
	return s.broker.Subscribe(ctx)
}

// FetchLibraries fetches libraries and publishes events
func (s *LibraryServiceWithEvents) FetchLibraries(
	ctx context.Context,
) ([]domain.MusicLibrary, int, error) {
	libraries, total, err := s.LibraryService.FetchLibraries(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "libraries.fetch_failed",
			Payload: LibraryEvent{
				Type:  "libraries.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "libraries.loaded",
		Payload: LibraryEvent{
			Type:      "libraries.loaded",
			Libraries: libraries,
			TotalSize: total,
		},
	})

	return libraries, total, nil
}

// FetchAlbums fetches albums and publishes events
func (s *LibraryServiceWithEvents) FetchAlbums(
	ctx context.Context,
	libraryKey string,
) ([]domain.Album, int, error) {
	albums, total, err := s.LibraryService.FetchAlbums(ctx, libraryKey)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "albums.fetch_failed",
			Payload: LibraryEvent{
				Type:  "albums.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "albums.loaded",
		Payload: LibraryEvent{
			Type:      "albums.loaded",
			Albums:    albums,
			TotalSize: total,
		},
	})

	return albums, total, nil
}

// FetchRecentlyAdded fetches recently added albums and publishes events
func (s *LibraryServiceWithEvents) FetchRecentlyAdded(
	ctx context.Context,
) ([]domain.Album, int, error) {
	albums, total, err := s.LibraryService.FetchRecentlyAdded(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "recently_added.fetch_failed",
			Payload: LibraryEvent{
				Type:  "recently_added.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "recently_added.loaded",
		Payload: LibraryEvent{
			Type:      "recently_added.loaded",
			Albums:    albums,
			TotalSize: total,
		},
	})

	return albums, total, nil
}

// FetchPlaylists fetches playlists and publishes events
func (s *LibraryServiceWithEvents) FetchPlaylists(
	ctx context.Context,
) ([]domain.Playlist, int, error) {
	playlists, total, err := s.LibraryService.FetchPlaylists(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "playlists.fetch_failed",
			Payload: LibraryEvent{
				Type:  "playlists.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "playlists.loaded",
		Payload: LibraryEvent{
			Type:      "playlists.loaded",
			Playlists: playlists,
			TotalSize: total,
		},
	})

	return playlists, total, nil
}

// FetchTracks fetches tracks and publishes events
func (s *LibraryServiceWithEvents) FetchTracks(
	ctx context.Context,
	key string,
) ([]domain.Track, int, error) {
	tracks, total, err := s.LibraryService.FetchTracks(ctx, key)
	if err != nil {
		s.broker.Publish(pubsub.Event[LibraryEvent]{
			Type: "tracks.fetch_failed",
			Payload: LibraryEvent{
				Type:  "tracks.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[LibraryEvent]{
		Type: "tracks.loaded",
		Payload: LibraryEvent{
			Type:      "tracks.loaded",
			Tracks:    tracks,
			TotalSize: total,
		},
	})

	return tracks, total, nil
}

// FetchSectionCounts fetches counts and publishes events (optional, mostly for UI update)
func (s *LibraryServiceWithEvents) FetchSectionCounts(
	ctx context.Context,
	sectionKey string,
) (int, int, int, error) {
	artists, albums, tracks, err := s.LibraryService.FetchSectionCounts(ctx, sectionKey)
	if err != nil {
		// We can publish a generic error or specific one
		return 0, 0, 0, err
	}
	// We don't have a specific event for counts yet, but we can return them directly
	// or add a new event type if needed. For now, direct return is fine as this is
	// likely called during initialization or refresh.
	return artists, albums, tracks, nil
}

// Close closes the service and releases resources
func (s *LibraryServiceWithEvents) Close() error {
	s.broker.Close()
	return nil
}
