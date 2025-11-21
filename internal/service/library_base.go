package service

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"net/http"
	"net/url"
	"strings"

	log "github.com/charmbracelet/log/v2"

	"plexmusic-tui/internal/domain"
	httpclient "plexmusic-tui/internal/http"
)

// decodePlexMediaContainer attempts to decode a body into the canonical
// PlexMediaContainer structure. Some Plex servers may return a wrapper; this
// helper handles common wrapper keys and attempts to extract the inner
// MediaContainer structure when present.
func decodePlexMediaContainer(body []byte, out *domain.PlexMediaContainer) error {
	// Try direct decode first
	if err := json.Unmarshal(body, out); err == nil && (len(out.Directory) > 0 || len(out.Metadata) > 0) {
		return nil
	}

	// Attempt to find a nested object that contains the MediaContainer
	var topObj map[string]json.RawMessage
	if err := json.Unmarshal(body, &topObj); err != nil {
		return err
	}

	// Common wrapper keys when using different Plex endpoints
	candidates := []string{"MediaContainer", "mediaContainer", "Response", "response", "Media"}
	for _, k := range candidates {
		if raw, ok := topObj[k]; ok {
			var inner domain.PlexMediaContainer
			if err := json.Unmarshal(raw, &inner); err == nil && (len(inner.Directory) > 0 || len(inner.Metadata) > 0) {
				*out = inner
				return nil
			}
		}
	}

	return fmt.Errorf("no valid PlexMediaContainer found")
}

// decodePlexPlaylistContainer handles playlist containers similarly.
func decodePlexPlaylistContainer(body []byte, out *domain.PlexPlaylistContainer) error {
	// Try direct decode first
	if err := json.Unmarshal(body, out); err == nil && len(out.Metadata) > 0 {
		return nil
	}

	var topObj map[string]json.RawMessage
	if err := json.Unmarshal(body, &topObj); err != nil {
		return err
	}

	candidates := []string{"MediaContainer", "mediaContainer", "Response", "response", "Metadata", "Playlist"}
	for _, k := range candidates {
		if raw, ok := topObj[k]; ok {
			var inner domain.PlexPlaylistContainer
			if err := json.Unmarshal(raw, &inner); err == nil && len(inner.Metadata) > 0 {
				*out = inner
				return nil
			}
		}
	}

	// Also support the structure used by some plex servers (top-level Playlist key)
	var alt struct {
		Playlist []domain.Playlist `json:"Playlist"`
	}
	if err := json.Unmarshal(body, &alt); err == nil && len(alt.Playlist) > 0 {
		out.Metadata = alt.Playlist
		return nil
	}

	return fmt.Errorf("no valid PlexPlaylistContainer found")
}

// decodePlexTrackContainer handles track containers similarly to other
// container decoders, supporting common wrappers like MediaContainer
// or Response keys.
func decodePlexTrackContainer(body []byte, out *domain.PlexTrackContainer) error {
	// Try direct decode
	if err := json.Unmarshal(body, out); err == nil && len(out.Metadata) > 0 {
		return nil
	}

	// Try detecting wrapper keys
	var topObj map[string]json.RawMessage
	if err := json.Unmarshal(body, &topObj); err != nil {
		return err
	}

	candidates := []string{"MediaContainer", "mediaContainer", "Response", "response", "Metadata"}
	for _, k := range candidates {
		if raw, ok := topObj[k]; ok {
			var inner domain.PlexTrackContainer
			if err := json.Unmarshal(raw, &inner); err == nil && len(inner.Metadata) > 0 {
				*out = inner
				return nil
			}
		}
	}

	// Some servers may respond with a top-level 'Metadata' array directly
	var alt struct {
		Metadata []domain.Track `json:"Metadata"`
	}
	if err := json.Unmarshal(body, &alt); err == nil && len(alt.Metadata) > 0 {
		out.Metadata = alt.Metadata
		return nil
	}

	return fmt.Errorf("no valid PlexTrackContainer found")
}

// LibraryService provides methods to fetch library data from Plex servers.
type LibraryService struct {
	baseURL    string // e.g., "https://192.168.1.100:32400"
	token      string
	httpClient *httpclient.Client
}

// NewLibraryService creates a new library service for a Plex server.
// baseURL should be the full URL to the Plex server (scheme + host + port)
// e.g., "https://192.168.1.100:32400" or "http://localhost:32400"
func NewLibraryService(baseURL, token string) *LibraryService {
	// Extract host from baseURL for intelligent TLS handling
	u, err := url.Parse(baseURL)
	if err != nil {
		// Fallback to standard client if URL parsing fails
		return &LibraryService{
			baseURL:    baseURL,
			token:      token,
			httpClient: httpclient.New(),
		}
	}

	return &LibraryService{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpclient.GetForHost(u.Host),
	}
}

// FetchLibraries fetches the list of music libraries from the Plex server.
func (s *LibraryService) FetchLibraries(ctx context.Context) ([]domain.MusicLibrary, error) {
	endpoint := fmt.Sprintf("%s/library/sections?type=8", s.baseURL)
	log.Debug("FetchLibraries", "endpoint", endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Error("FetchLibraries: HTTP request failed", "endpoint", endpoint, "error", err)
		return nil, fmt.Errorf("failed to fetch libraries: %w", err)
	}
	defer resp.Body.Close()
	log.Debug("LibraryService.FetchLibraries", "status", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read body bytes to support both direct media container and wrapped responses
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read libraries response: %w", err)
	}

	var container domain.PlexMediaContainer
	// Try direct decoding of the typical Plex MediaContainer structure.
	if err := json.Unmarshal(body, &container); err != nil {
		// Certain Plex responses wrap the MediaContainer inside another object,
		// or use a top-level 'Response' object. Attempt to discover the nested
		// MediaContainer field using common keys and re-decode.
		var wrapper map[string]json.RawMessage
		if uwErr := json.Unmarshal(body, &wrapper); uwErr == nil {
			for _, key := range []string{"MediaContainer", "mediaContainer", "Response", "response", "PlexContainer"} {
				if raw, ok := wrapper[key]; ok {
					if err := json.Unmarshal(raw, &container); err == nil {
						goto parsed
					}
				}
			}
		}
		return nil, fmt.Errorf("failed to decode libraries: %w", err)
	}
parsed:

	// Filter to music libraries only. Use a whitelist of known music section
	// types to avoid including TV/movie/photo sections that some servers
	// may return unexpectedly.
	allowed := map[string]bool{"artist": true, "music": true, "album": true, "collection": true, "music_video": true}
	var libraries []domain.MusicLibrary
	for _, dir := range container.Directory {
		if allowed[strings.ToLower(dir.Type)] {
			libraries = append(libraries, dir)
		}
	}

	return libraries, nil
}

// FetchAlbums fetches all albums from a specific library.
func (s *LibraryService) FetchAlbums(ctx context.Context, libraryKey string) ([]domain.Album, error) {
	endpoint := fmt.Sprintf("%s/library/sections/%s/albums", s.baseURL, libraryKey)
	return s.fetchAlbums(ctx, endpoint)
}

// FetchRecentlyAdded fetches recently added albums for the server.
func (s *LibraryService) FetchRecentlyAdded(ctx context.Context) ([]domain.Album, error) {
	endpoint := fmt.Sprintf("%s/library/recentlyAdded?type=9", s.baseURL)
	log.Debug("FetchRecentlyAdded", "endpoint", endpoint)
	return s.fetchAlbums(ctx, endpoint)
}

// FetchRecentlyAddedInLibrary fetches recently added albums scoped to a specific library.
func (s *LibraryService) FetchRecentlyAddedInLibrary(ctx context.Context, libraryKey string) ([]domain.Album, error) {
	endpoint := fmt.Sprintf("%s/library/sections/%s/recentlyAdded?type=9", s.baseURL, libraryKey)
	return s.fetchAlbums(ctx, endpoint)
}

// fetchAlbums is a helper for fetching albums from any endpoint.
func (s *LibraryService) fetchAlbums(ctx context.Context, endpoint string) ([]domain.Album, error) {
	log.Debug("fetchAlbums: starting request", "endpoint", endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Error("fetchAlbums: HTTP request failed", "endpoint", endpoint, "error", err)
		return nil, fmt.Errorf("failed to fetch albums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Robust decode to handle Plex responses that may be wrapped in MediaContainer
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read albums response: %w", err)
	}

	var container domain.PlexMediaContainer
	if err := decodePlexMediaContainer(body, &container); err != nil {
		return nil, fmt.Errorf("failed to decode albums: %w", err)
	}

	return container.Metadata, nil
}

// FetchPlaylists fetches all playlists from the server.
func (s *LibraryService) FetchPlaylists(ctx context.Context) ([]domain.Playlist, error) {
	endpoint := s.baseURL + "/playlists"
	log.Debug("FetchPlaylists", "endpoint", endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Error("FetchPlaylists: HTTP request failed", "endpoint", endpoint, "error", err)
		return nil, fmt.Errorf("failed to fetch playlists: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read body bytes and decode using the robust helper
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var container domain.PlexPlaylistContainer
	if err := decodePlexPlaylistContainer(body, &container); err != nil {
		return nil, fmt.Errorf("failed to decode playlists: %w", err)
	}

	return container.Metadata, nil
}

// FetchTracks fetches tracks from a specific album or playlist.
// The key parameter should be the media key (e.g., /library/metadata/{id} or /playlists/{id})
func (s *LibraryService) FetchTracks(ctx context.Context, key string) ([]domain.Track, error) {
	// We will construct endpoints and perform the fetch inline below.

	// Normalize key: determine the path to inspect for fallback logic and
	// compute an endpoint for the primary fetch. If key is an absolute URL
	// (starts with http:// or https://), prefer using it as provided; else
	// treat it as a relative path and prepend the base URL.
	var endpoint string
	pathOnly := key
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		// Absolute URL - use as-is for the primary fetch, and use its Path
		// for fallback detection.
		if u, err := url.Parse(key); err == nil {
			pathOnly = u.Path
			endpoint = u.String()
		} else {
			// If parse fails, fall back to concatenating the base URL.
			endpoint = s.baseURL + key
		}
	} else {
		endpoint = s.baseURL + key
	}

	// Attempt primary fetch
	tracks, err := func() ([]domain.Track, error) {
		// Use endpoint variable instead of constructing from baseURL
		req, rerr := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if rerr != nil {
			return nil, fmt.Errorf("failed to create request: %w", rerr)
		}
		s.addPlexHeaders(req)
		resp, derr := s.httpClient.Do(req)
		if derr != nil {
			return nil, fmt.Errorf("failed to fetch tracks: %w", derr)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
		}
		// Read body and use decode helper to support wrapped responses
		body, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			return nil, fmt.Errorf("failed to read tracks response: %w", rerr)
		}
		var container domain.PlexTrackContainer
		if derr := decodePlexTrackContainer(body, &container); derr != nil {
			return nil, fmt.Errorf("failed to decode tracks: %w", derr)
		}
		return container.Metadata, nil
	}()
	// If primary fetch returned no tracks, try a fallback endpoint that some
	// Plex servers use for album children
	if err != nil || len(tracks) == 0 {
		if strings.HasPrefix(pathOnly, "/library/metadata/") {
			alt := pathOnly
			if !strings.HasSuffix(alt, "/children") {
				alt = alt + "/children"
			}
			// Build the alternate endpoint using the base URL, which ensures
			// we query the current configured server rather than any absolute
			// host that may have been embedded in the key.
			altEndpoint := s.baseURL + alt
			altTracks, altErr := func() ([]domain.Track, error) {
				req, rerr := http.NewRequestWithContext(ctx, "GET", altEndpoint, nil)
				if rerr != nil {
					return nil, fmt.Errorf("failed to create request: %w", rerr)
				}
				s.addPlexHeaders(req)
				resp, derr := s.httpClient.Do(req)
				if derr != nil {
					return nil, fmt.Errorf("failed to fetch tracks: %w", derr)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(resp.Body)
					return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
				}
				body, rerr := io.ReadAll(resp.Body)
				if rerr != nil {
					return nil, fmt.Errorf("failed to read tracks response: %w", rerr)
				}
				var container domain.PlexTrackContainer
				if derr := decodePlexTrackContainer(body, &container); derr != nil {
					return nil, fmt.Errorf("failed to decode tracks: %w", derr)
				}
				return container.Metadata, nil
			}()
			if altErr == nil && len(altTracks) > 0 {
				return altTracks, nil
			}
		}
	}
	return tracks, err
}

// BuildStreamURL constructs the URL for streaming an audio track.
func (s *LibraryService) BuildStreamURL(track *domain.Track) (string, error) {
	if track == nil {
		return "", fmt.Errorf("track is nil")
	}

	var key string

	// Prefer the full media part key
	if len(track.Media) > 0 && len(track.Media[0].Part) > 0 {
		key = track.Media[0].Part[0].Key
	} else {
		key = track.Key
	}

	if key == "" {
		return "", fmt.Errorf("no playable key found for track")
	}

	// Build full URL - prefer adding X-Plex-Token as query param but do not
	// duplicate it if present.
	streamURL := s.baseURL + key
	u, err := url.Parse(streamURL)
	if err != nil {
		return "", fmt.Errorf("invalid stream url: %w", err)
	}
	q := u.Query()
	if q.Get("X-Plex-Token") == "" && s.token != "" {
		q.Set("X-Plex-Token", s.token)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// FetchStream fetches the audio stream for a track.
func (s *LibraryService) FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error) {
	urlStr, err := s.BuildStreamURL(track)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, "", err
	}
	// Do not use addPlexHeaders as it sets Accept: application/json which can confuse Plex
	// when fetching media streams.
	if s.token != "" {
		req.Header.Set("X-Plex-Token", s.token)
	}
	req.Header.Set("User-Agent", "plexmusic-tui")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("failed to fetch stream: status %d", resp.StatusCode)
	}

	return resp.Body, resp.Header.Get("Content-Type"), nil
}

// FetchImage fetches and decodes an image from the Plex server.
func (s *LibraryService) FetchImage(ctx context.Context, path string) (image.Image, error) {
	// Construct URL
	var urlStr string
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		urlStr = path
	} else {
		urlStr = s.baseURL + path
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch image: status %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return img, nil
}

// addPlexHeaders adds standard Plex API headers to a request.
func (s *LibraryService) addPlexHeaders(req *http.Request) {
	req.Header.Set("X-Plex-Token", s.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "plexmusic-tui")
	// Also add the token as a query parameter (some server configurations
	// prefer receiving the token via URL when Host header parsing is different)
	if s.token != "" && req.URL != nil {
		q := req.URL.Query()
		if q.Get("X-Plex-Token") == "" {
			q.Set("X-Plex-Token", s.token)
			req.URL.RawQuery = q.Encode()
		}
	}
}

// SetBaseURL updates the base URL (useful for switching servers).
func (s *LibraryService) SetBaseURL(baseURL string) {
	s.baseURL = baseURL
	// Update HTTP client for new host
	u, err := url.Parse(baseURL)
	if err == nil {
		s.httpClient = httpclient.GetForHost(u.Host)
	}
}

// SetToken updates the authentication token.
func (s *LibraryService) SetToken(token string) {
	s.token = token
}
