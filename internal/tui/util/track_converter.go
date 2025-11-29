package util

import (
	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
)

// AppTrackToDomain converts an app.Track object to a domain.Track for playback
func AppTrackToDomain(at *app.Track) *domain.Track {
	if at == nil {
		return nil
	}
	dt := &domain.Track{
		Title:       at.Title,
		Artist:      at.Artist,
		Album:       at.Album,
		Duration:    at.Duration,
		TrackNumber: at.TrackNumber,
		Key:         at.Key,
		RatingKey:   at.RatingKey,
		Thumb:       at.Thumb,
	}

	if len(at.Media) > 0 {
		dt.Media = make([]struct {
			Part []struct {
				Key string `json:"key"`
			} `json:"Part"`
		}, len(at.Media))
		for i, m := range at.Media {
			if len(m.Part) > 0 {
				dt.Media[i].Part = make([]struct {
					Key string `json:"key"`
				}, len(m.Part))
				for j, part := range m.Part {
					dt.Media[i].Part[j].Key = part.Key
				}
			}
		}
	}
	return dt
}

// DomainTrackToApp converts a domain.Track into an app.Track for UI presentation
func DomainTrackToApp(dt *domain.Track) *app.Track {
	if dt == nil {
		return nil
	}
	at := &app.Track{
		Title:           dt.Title,
		Artist:          dt.Artist,
		Album:           dt.Album,
		Duration:        dt.Duration,
		TrackNumber:     dt.TrackNumber,
		PlaylistItemID:  dt.PlaylistItemID,
		PlayQueueItemID: dt.PlayQueueItemID,
		Key:             dt.Key,
		RatingKey:       dt.RatingKey,
		Thumb:           dt.Thumb,
	}

	if len(dt.Media) > 0 {
		at.Media = make([]struct {
			Part []struct {
				Key string
			}
		}, len(dt.Media))
		for i, m := range dt.Media {
			if len(m.Part) > 0 {
				at.Media[i].Part = make([]struct {
					Key string
				}, len(m.Part))
				for j, part := range m.Part {
					at.Media[i].Part[j].Key = part.Key
				}
			}
		}
	}
	return at
}
