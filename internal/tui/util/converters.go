package util

import (
	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
)

// AppAlbumToDomain converts an app.Album to domain.Album
func AppAlbumToDomain(a *app.Album) *domain.Album {
	if a == nil {
		return nil
	}
	return &domain.Album{
		Title:  a.Title,
		Artist: a.Artist,
		Year:   a.Year,
		Key:    a.Key,
		Thumb:  a.Thumb,
	}
}

// DomainAlbumToApp converts a domain.Album to app.Album
func DomainAlbumToApp(a *domain.Album) *app.Album {
	if a == nil {
		return nil
	}
	return &app.Album{
		Title:  a.Title,
		Artist: a.Artist,
		Year:   a.Year,
		Key:    a.Key,
		Thumb:  a.Thumb,
	}
}

// AppPlaylistToDomain converts an app.Playlist to domain.Playlist
func AppPlaylistToDomain(p *app.Playlist) *domain.Playlist {
	if p == nil {
		return nil
	}
	return &domain.Playlist{
		Title:        p.Title,
		Key:          p.Key,
		LeafCount:    p.LeafCount,
		Duration:     p.Duration,
		PlaylistType: p.PlaylistType,
	}
}

// DomainPlaylistToApp converts a domain.Playlist to app.Playlist
func DomainPlaylistToApp(p *domain.Playlist) *app.Playlist {
	if p == nil {
		return nil
	}
	return &app.Playlist{
		Title:        p.Title,
		Key:          p.Key,
		LeafCount:    p.LeafCount,
		Duration:     p.Duration,
		PlaylistType: p.PlaylistType,
	}
}

// AppArtistToDomain converts an app.Artist to domain.Artist
func AppArtistToDomain(a *app.Artist) *domain.Artist {
	if a == nil {
		return nil
	}
	return &domain.Artist{
		Title: a.Name,
		Key:   a.Key,
	}
}

// DomainArtistToApp converts a domain.Artist to app.Artist
func DomainArtistToApp(a *domain.Artist) *app.Artist {
	if a == nil {
		return nil
	}
	return &app.Artist{
		Name: a.Title,
		Key:  a.Key,
	}
}

// AppHubToDomain converts an app.Hub to domain.Hub
func AppHubToDomain(h *app.Hub) *domain.Hub {
	if h == nil {
		return nil
	}

	playlists := make([]domain.Playlist, len(h.Playlists))
	for i, p := range h.Playlists {
		if dp := AppPlaylistToDomain(&p); dp != nil {
			playlists[i] = *dp
		}
	}

	albums := make([]domain.Album, len(h.Albums))
	for i, a := range h.Albums {
		if da := AppAlbumToDomain(&a); da != nil {
			albums[i] = *da
		}
	}

	artists := make([]domain.Artist, len(h.Artists))
	for i, a := range h.Artists {
		if da := AppArtistToDomain(&a); da != nil {
			artists[i] = *da
		}
	}

	return &domain.Hub{
		HubIdentifier: h.HubIdentifier,
		Title:         h.Title,
		Type:          h.Type,
		Context:       h.Context,
		Size:          h.Size,
		Playlists:     playlists,
		Albums:        albums,
		Artists:       artists,
	}
}

// DomainHubToApp converts a domain.Hub to app.Hub
func DomainHubToApp(h *domain.Hub) *app.Hub {
	if h == nil {
		return nil
	}

	playlists := make([]app.Playlist, len(h.Playlists))
	for i, p := range h.Playlists {
		if ap := DomainPlaylistToApp(&p); ap != nil {
			playlists[i] = *ap
		}
	}

	albums := make([]app.Album, len(h.Albums))
	for i, a := range h.Albums {
		if aa := DomainAlbumToApp(&a); aa != nil {
			albums[i] = *aa
		}
	}

	artists := make([]app.Artist, len(h.Artists))
	for i, a := range h.Artists {
		if aa := DomainArtistToApp(&a); aa != nil {
			artists[i] = *aa
		}
	}

	return &app.Hub{
		HubIdentifier: h.HubIdentifier,
		Title:         h.Title,
		Type:          h.Type,
		Context:       h.Context,
		Size:          h.Size,
		Playlists:     playlists,
		Albums:        albums,
		Artists:       artists,
	}
}
