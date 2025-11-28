package service

import (
	"context"

	log "github.com/charmbracelet/log/v2"

	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
)

// LibraryServiceWithEvents wraps LibraryService with event publishing
type LibraryServiceWithEvents struct {
	*LibraryService
	broker *pubsub.Broker[domain.LibraryEvent]
}

// NewLibraryServiceWithEvents creates a library service with event support
func NewLibraryServiceWithEvents(baseURL, token string, factory domain.HTTPClientFactory) *LibraryServiceWithEvents {
	return &LibraryServiceWithEvents{
		LibraryService: NewLibraryService(baseURL, token, factory),
		broker:         pubsub.NewBroker[domain.LibraryEvent](),
	}
}

// Subscribe returns a channel for receiving library events
func (s *LibraryServiceWithEvents) Subscribe(
	ctx context.Context,
) <-chan pubsub.Event[domain.LibraryEvent] {
	return s.broker.Subscribe(ctx)
}

// FetchLibraries fetches libraries and publishes events
func (s *LibraryServiceWithEvents) FetchLibraries(
	ctx context.Context,
) ([]domain.MusicLibrary, int, error) {
	libraries, total, err := s.LibraryService.FetchLibraries(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type: "libraries.fetch_failed",
			Payload: domain.LibraryEvent{
				Type:  "libraries.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type: "libraries.loaded",
		Payload: domain.LibraryEvent{
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
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type: "albums.fetch_failed",
			Payload: domain.LibraryEvent{
				Type:  "albums.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type: "albums.loaded",
		Payload: domain.LibraryEvent{
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
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type: "recently_added.fetch_failed",
			Payload: domain.LibraryEvent{
				Type:  "recently_added.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type: "recently_added.loaded",
		Payload: domain.LibraryEvent{
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
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type: "playlists.fetch_failed",
			Payload: domain.LibraryEvent{
				Type:  "playlists.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type: "playlists.loaded",
		Payload: domain.LibraryEvent{
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
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type: "tracks.fetch_failed",
			Payload: domain.LibraryEvent{
				Type:  "tracks.fetch_failed",
				Error: err,
			},
		})
		return nil, 0, err
	}

	log.Info("LibraryServiceWithEvents: publishing tracks.loaded", "key", key, "trackCount", len(tracks))
	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type: "tracks.loaded",
		Payload: domain.LibraryEvent{
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

// HasPlexPass checks if the current server supports Plex Pass
func (s *LibraryServiceWithEvents) HasPlexPass(ctx context.Context) (bool, error) {
	ok, err := s.LibraryService.HasPlexPass(ctx)
	if err == nil && ok {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type: "server.plexpass",
			Payload: domain.LibraryEvent{
				Type:  "server.plexpass",
				Error: nil,
			},
		})
	}
	return ok, err
}

// HasSonicAnalysis asks whether sonic analysis is available for the library.
func (s *LibraryServiceWithEvents) HasSonicAnalysis(ctx context.Context) (bool, error) {
	ok, err := s.LibraryService.HasSonicAnalysis(ctx)
	if err == nil && ok {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type: "library.sonic_analyzed",
			Payload: domain.LibraryEvent{
				Type:  "library.sonic_analyzed",
				Error: nil,
			},
		})
	}
	return ok, err
}

func (s *LibraryServiceWithEvents) FetchSonicallySimilarTracks(
	ctx context.Context,
	ratingKey string,
	limit int,
	maxDistance float64,
) ([]domain.Track, int, error) {
	tracks, total, err := s.LibraryService.FetchSonicallySimilarTracks(ctx, ratingKey, limit, maxDistance)
	// Optional: publish events if needed
	if err != nil {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type:    "similar_tracks.fetch_failed",
			Payload: domain.LibraryEvent{Type: "similar_tracks.fetch_failed", Error: err},
		})
		return nil, 0, err
	}
	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type:    "similar_tracks.loaded",
		Payload: domain.LibraryEvent{Type: "similar_tracks.loaded", Tracks: tracks, TotalSize: total},
	})
	return tracks, total, nil
}

func (s *LibraryServiceWithEvents) FetchSonicAdventure(
	ctx context.Context,
	start, end string,
) ([]domain.Track, int, error) {
	tracks, total, err := s.LibraryService.FetchSonicAdventure(ctx, start, end)
	if err != nil {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type:    "sonic_adventure.fetch_failed",
			Payload: domain.LibraryEvent{Type: "sonic_adventure.fetch_failed", Error: err},
		})
		return nil, 0, err
	}
	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type:    "sonic_adventure.loaded",
		Payload: domain.LibraryEvent{Type: "sonic_adventure.loaded", Tracks: tracks, TotalSize: total},
	})
	return tracks, total, nil
}

func (s *LibraryServiceWithEvents) FetchLibraryHubs(ctx context.Context, sectionKey string) ([]domain.Hub, error) {
	hubs, err := s.LibraryService.FetchLibraryHubs(ctx, sectionKey)
	if err != nil {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type:    "hubs.fetch_failed",
			Payload: domain.LibraryEvent{Type: "hubs.fetch_failed", Error: err},
		})
		return nil, err
	}
	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type:    "hubs.loaded",
		Payload: domain.LibraryEvent{Type: "hubs.loaded", Hubs: hubs},
	})
	return hubs, nil
}

func (s *LibraryServiceWithEvents) FetchMixesForYou(ctx context.Context) ([]domain.Playlist, int, error) {
	playlists, total, err := s.LibraryService.FetchMixesForYou(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type:    "mixes.fetch_failed",
			Payload: domain.LibraryEvent{Type: "mixes.fetch_failed", Error: err},
		})
		return nil, 0, err
	}
	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type:    "mixes.loaded",
		Payload: domain.LibraryEvent{Type: "mixes.loaded", Playlists: playlists, TotalSize: total},
	})
	return playlists, total, nil
}

func (s *LibraryServiceWithEvents) FetchOnThisDay(ctx context.Context) ([]domain.Album, int, error) {
	albums, total, err := s.LibraryService.FetchOnThisDay(ctx)
	if err != nil {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type:    "onthisday.fetch_failed",
			Payload: domain.LibraryEvent{Type: "onthisday.fetch_failed", Error: err},
		})
		return nil, 0, err
	}
	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type:    "onthisday.loaded",
		Payload: domain.LibraryEvent{Type: "onthisday.loaded", Albums: albums, TotalSize: total},
	})
	return albums, total, nil
}

func (s *LibraryServiceWithEvents) FetchMoodStation(
	ctx context.Context,
	station string,
	limit int,
) ([]domain.Track, int, error) {
	tracks, total, err := s.LibraryService.FetchMoodStation(ctx, station, limit)
	if err != nil {
		s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
			Type:    "moodstation.fetch_failed",
			Payload: domain.LibraryEvent{Type: "moodstation.fetch_failed", Error: err},
		})
		return nil, 0, err
	}
	s.broker.Publish(pubsub.Event[domain.LibraryEvent]{
		Type:    "moodstation.loaded",
		Payload: domain.LibraryEvent{Type: "moodstation.loaded", Tracks: tracks, TotalSize: total},
	})
	return tracks, total, nil
}
