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
	"sync"

	log "github.com/charmbracelet/log/v2"

	"plexmusic-tui/internal/domain"
)

// decodePlexMediaContainer attempts to decode a body into the canonical
// PlexMediaContainer structure. Some Plex servers may return a wrapper; this
// helper handles common wrapper keys and attempts to extract the inner
// MediaContainer structure when present.
func decodePlexMediaContainer(body []byte, out *domain.PlexMediaContainer) error {
	// Parse into a map first to inspect structure
	var topObj map[string]json.RawMessage
	if err := json.Unmarshal(body, &topObj); err != nil {
		return err
	}

	// 1. Check for wrapper keys
	candidates := []string{"MediaContainer", "mediaContainer", "Response", "response", "Media"}
	for _, k := range candidates {
		if raw, ok := topObj[k]; ok {
			var inner domain.PlexMediaContainer
			if err := json.Unmarshal(raw, &inner); err == nil {
				*out = inner
				return nil
			}
		}
	}

	// 2. If no wrapper, check if the object itself looks like a MediaContainer
	// We check for presence of known keys.
	_, hasSize := topObj["size"]
	_, hasTotalSize := topObj["totalSize"]
	_, hasMetadata := topObj["Metadata"]
	_, hasDirectory := topObj["Directory"]

	if hasSize || hasTotalSize || hasMetadata || hasDirectory {
		if err := json.Unmarshal(body, out); err == nil {
			return nil
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

	candidates := []string{
		"MediaContainer",
		"mediaContainer",
		"Response",
		"response",
		"Metadata",
		"Playlist",
	}
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

	candidates := []string{
		"MediaContainer",
		"mediaContainer",
		"Response",
		"response",
		"Metadata",
		"Hub",
		"hub",
		"items",
		"Items",
		"Tracks",
		"tracks",
	}
	for _, k := range candidates {
		if raw, ok := topObj[k]; ok {
			var inner domain.PlexTrackContainer
			if err := json.Unmarshal(raw, &inner); err == nil && len(inner.Metadata) > 0 {
				*out = inner
				return nil
			}
			// If raw is an array of objects (e.g. Hub: [ {Metadata: [...]}, ... ]), try each element
			var arrRaw []json.RawMessage
			if err := json.Unmarshal(raw, &arrRaw); err == nil && len(arrRaw) > 0 {
				var combined []domain.Track
				for _, el := range arrRaw {
					var elInner domain.PlexTrackContainer
					if err := json.Unmarshal(el, &elInner); err == nil && len(elInner.Metadata) > 0 {
						combined = append(combined, elInner.Metadata...)
						continue
					}
					// try alt for Metadata array under element
					var alt struct {
						Metadata []domain.Track `json:"Metadata"`
					}
					if err := json.Unmarshal(el, &alt); err == nil && len(alt.Metadata) > 0 {
						combined = append(combined, alt.Metadata...)
						continue
					}
					// try direct array of tracks
					var arr2 []domain.Track
					if err := json.Unmarshal(el, &arr2); err == nil && len(arr2) > 0 {
						combined = append(combined, arr2...)
						continue
					}
				}
				if len(combined) > 0 {
					out.Metadata = combined
					return nil
				}
			}
			// Try direct decode as array of tracks
			var directArr []domain.Track
			if err := json.Unmarshal(raw, &directArr); err == nil && len(directArr) > 0 {
				out.Metadata = directArr
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

	// Try as a direct array of tracks
	var directTracks []domain.Track
	if err := json.Unmarshal(body, &directTracks); err == nil && len(directTracks) > 0 {
		out.Metadata = directTracks
		return nil
	}

	return fmt.Errorf("no valid PlexTrackContainer found")
}

// LibraryService provides methods to fetch library data from Plex servers.
type LibraryService struct {
	baseURL       string // e.g., "https://192.168.1.100:32400"
	token         string
	httpClient    domain.HTTPClient
	clientFactory domain.HTTPClientFactory
}

// NewLibraryService creates a new library service for a Plex server.
// baseURL should be the full URL to the Plex server (scheme + host + port)
// e.g., "https://192.168.1.100:32400" or "http://localhost:32400"
func NewLibraryService(baseURL, token string, factory domain.HTTPClientFactory) *LibraryService {
	// Extract host from baseURL for intelligent TLS handling
	u, err := url.Parse(baseURL)
	var client domain.HTTPClient
	if err != nil {
		// Fallback if URL parsing fails, though factory might need a host
		client = factory.GetClient("")
	} else {
		client = factory.GetClient(u.Host)
	}

	return &LibraryService{
		baseURL:       baseURL,
		token:         token,
		httpClient:    client,
		clientFactory: factory,
	}
}

// FetchLibraries fetches the list of music libraries from the Plex server.
func (s *LibraryService) FetchLibraries(ctx context.Context) ([]domain.MusicLibrary, int, error) {
	endpoint := fmt.Sprintf("%s/library/sections?type=8", s.baseURL)
	log.Debug("FetchLibraries", "endpoint", endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Error("FetchLibraries: HTTP request failed", "endpoint", endpoint, "error", err)
		return nil, 0, fmt.Errorf("failed to fetch libraries: %w", err)
	}
	defer resp.Body.Close()
	log.Debug("LibraryService.FetchLibraries", "status", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read body bytes to support both direct media container and wrapped responses
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read libraries response: %w", err)
	}

	var container domain.PlexMediaContainer
	// Try direct decoding of the typical Plex MediaContainer structure.
	if err := decodePlexMediaContainer(body, &container); err != nil {
		log.Error(
			"FetchLibraries: failed to decode",
			"error",
			err,
			"body_preview",
			string(body[:min(len(body), 500)]),
		)
		return nil, 0, fmt.Errorf("failed to decode libraries: %w", err)
	}

	// Filter to music libraries only. Use a whitelist of known music section
	// types to avoid including TV/movie/photo sections that some servers
	// may return unexpectedly.
	allowed := map[string]bool{
		"artist":      true,
		"music":       true,
		"album":       true,
		"collection":  true,
		"music_video": true,
	}
	var libraries []domain.MusicLibrary
	for _, dir := range container.Directory {
		if allowed[strings.ToLower(dir.Type)] {
			libraries = append(libraries, dir)
		}
	}
	log.Debug("FetchLibraries: success", "count", len(libraries), "totalSize", container.TotalSize)
	return libraries, container.TotalSize, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FetchAlbums fetches all albums from a specific library.
func (s *LibraryService) FetchAlbums(
	ctx context.Context,
	libraryKey string,
) ([]domain.Album, int, error) {
	endpoint := fmt.Sprintf("%s/library/sections/%s/albums", s.baseURL, libraryKey)
	return s.fetchAlbums(ctx, endpoint)
}

// FetchRecentlyAdded fetches recently added albums for the server.
func (s *LibraryService) FetchRecentlyAdded(ctx context.Context) ([]domain.Album, int, error) {
	endpoint := fmt.Sprintf("%s/library/recentlyAdded?type=9", s.baseURL)
	log.Debug("FetchRecentlyAdded", "endpoint", endpoint)
	return s.fetchAlbums(ctx, endpoint)
}

// FetchRecentlyAddedInLibrary fetches recently added albums scoped to a specific library.
func (s *LibraryService) FetchRecentlyAddedInLibrary(
	ctx context.Context,
	libraryKey string,
) ([]domain.Album, int, error) {
	endpoint := fmt.Sprintf("%s/library/sections/%s/recentlyAdded?type=9", s.baseURL, libraryKey)
	return s.fetchAlbums(ctx, endpoint)
}

// fetchAlbums is a helper for fetching albums from any endpoint.
func (s *LibraryService) fetchAlbums(
	ctx context.Context,
	endpoint string,
) ([]domain.Album, int, error) {
	log.Debug("fetchAlbums: starting request", "endpoint", endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Error("fetchAlbums: HTTP request failed", "endpoint", endpoint, "error", err)
		return nil, 0, fmt.Errorf("failed to fetch albums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Robust decode to handle Plex responses that may be wrapped in MediaContainer
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read albums response: %w", err)
	}

	var container domain.PlexMediaContainer
	if err := decodePlexMediaContainer(body, &container); err != nil {
		return nil, 0, fmt.Errorf("failed to decode albums: %w", err)
	}

	return container.Metadata, container.TotalSize, nil
}

// FetchPlaylists fetches all playlists from the server.
func (s *LibraryService) FetchPlaylists(ctx context.Context) ([]domain.Playlist, int, error) {
	endpoint := s.baseURL + "/playlists"
	log.Debug("FetchPlaylists", "endpoint", endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	s.addPlexHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Error("FetchPlaylists: HTTP request failed", "endpoint", endpoint, "error", err)
		return nil, 0, fmt.Errorf("failed to fetch playlists: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read body bytes and decode using the robust helper
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}

	var container domain.PlexPlaylistContainer
	if err := decodePlexPlaylistContainer(body, &container); err != nil {
		return nil, 0, fmt.Errorf("failed to decode playlists: %w", err)
	}

	return container.Metadata, container.TotalSize, nil
}

// FetchTracks fetches tracks from a specific album or playlist.
// The key parameter should be the media key (e.g., /library/metadata/{id} or /playlists/{id})
func (s *LibraryService) FetchTracks(ctx context.Context, key string) ([]domain.Track, int, error) {
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
	tracks, totalSize, err := func() ([]domain.Track, int, error) {
		// Use endpoint variable instead of constructing from baseURL
		req, rerr := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if rerr != nil {
			return nil, 0, fmt.Errorf("failed to create request: %w", rerr)
		}
		s.addPlexHeaders(req)
		resp, derr := s.httpClient.Do(req)
		if derr != nil {
			return nil, 0, fmt.Errorf("failed to fetch tracks: %w", derr)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, 0, fmt.Errorf(
				"server returned status %d: %s",
				resp.StatusCode,
				string(body),
			)
		}
		// Read body and use decode helper to support wrapped responses
		body, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			return nil, 0, fmt.Errorf("failed to read tracks response: %w", rerr)
		}
		var container domain.PlexTrackContainer
		if derr := decodePlexTrackContainer(body, &container); derr != nil {
			return nil, 0, fmt.Errorf("failed to decode tracks: %w", derr)
		}
		return container.Metadata, container.TotalSize, nil
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
			altTracks, altTotalSize, altErr := func() ([]domain.Track, int, error) {
				req, rerr := http.NewRequestWithContext(ctx, "GET", altEndpoint, nil)
				if rerr != nil {
					return nil, 0, fmt.Errorf("failed to create request: %w", rerr)
				}
				s.addPlexHeaders(req)
				resp, derr := s.httpClient.Do(req)
				if derr != nil {
					return nil, 0, fmt.Errorf("failed to fetch tracks: %w", derr)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(resp.Body)
					return nil, 0, fmt.Errorf(
						"server returned status %d: %s",
						resp.StatusCode,
						string(body),
					)
				}
				body, rerr := io.ReadAll(resp.Body)
				if rerr != nil {
					return nil, 0, fmt.Errorf("failed to read tracks response: %w", rerr)
				}
				var container domain.PlexTrackContainer
				if derr := decodePlexTrackContainer(body, &container); derr != nil {
					return nil, 0, fmt.Errorf("failed to decode tracks: %w", derr)
				}
				return container.Metadata, container.TotalSize, nil
			}()
			if altErr == nil && len(altTracks) > 0 {
				return altTracks, altTotalSize, nil
			}
		}
	}
	return tracks, totalSize, err
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
func (s *LibraryService) FetchStream(
	ctx context.Context,
	track *domain.Track,
) (io.ReadCloser, string, error) {
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
// The URL is validated to ensure it's a properly formatted Plex server URL.
func (s *LibraryService) SetBaseURL(baseURL string) error {
	// Validate URL format
	validator := domain.NewValidator()
	if err := validator.ValidateVar(baseURL, "required,plexurl"); err != nil {
		return ErrValidation{Message: "Invalid Plex server URL: " + baseURL}
	}

	s.baseURL = baseURL
	// Update HTTP client for new host
	u, err := url.Parse(baseURL)
	if err == nil && s.clientFactory != nil {
		s.httpClient = s.clientFactory.GetClient(u.Host)
	}
	return nil
}

// SetToken updates the authentication token.
// The token is validated to ensure it meets Plex authentication token requirements.
func (s *LibraryService) SetToken(token string) error {
	// Validate token format
	validator := domain.NewValidator()
	if err := validator.ValidateVar(token, "required,plextoken"); err != nil {
		return ErrValidation{Message: "Invalid Plex authentication token"}
	}

	s.token = token
	return nil
}

// FetchSectionCounts fetches the total count of artists, albums, and tracks for a library section.
func (s *LibraryService) FetchSectionCounts(
	ctx context.Context,
	sectionKey string,
) (int, int, int, error) {
	// Helper to fetch count for a specific type
	fetchCount := func(typeID int) (int, error) {
		endpoint := fmt.Sprintf(
			"%s/library/sections/%s/all?type=%d&X-Plex-Container-Start=0&X-Plex-Container-Size=0",
			s.baseURL,
			sectionKey,
			typeID,
		)
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return 0, err
		}
		s.addPlexHeaders(req)
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return 0, err
		}
		var container domain.PlexMediaContainer
		if err := decodePlexMediaContainer(body, &container); err != nil {
			// Log the body for debugging if decode fails
			log.Debug("Failed to decode stats response", "type", typeID, "body", string(body))
			return 0, err
		}
		log.Debug(
			"fetchCount success",
			"type",
			typeID,
			"totalSize",
			container.TotalSize,
			"body_len",
			len(body),
		)
		return container.TotalSize, nil
	}

	// Type 8 = Artist, 9 = Album, 10 = Track
	// We attempt to fetch each count independently. If one fails, we continue with 0.
	// This ensures we display partial stats rather than nothing if a specific type query fails.
	// We run these in parallel to speed up the total fetch time.

	var wg sync.WaitGroup
	wg.Add(3)

	var artists, albums, tracks int

	go func() {
		defer wg.Done()
		var err error
		log.Debug("FetchSectionCounts: fetching artists")
		artists, err = fetchCount(8)
		if err != nil {
			log.Error("Failed to fetch artist count", "err", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		log.Debug("FetchSectionCounts: fetching albums")
		albums, err = fetchCount(9)
		if err != nil {
			log.Error("Failed to fetch album count", "err", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		log.Debug("FetchSectionCounts: fetching tracks")
		tracks, err = fetchCount(10)
		if err != nil {
			log.Error("Failed to fetch track count", "err", err)
		}
	}()

	wg.Wait()

	// If all failed, we might want to return an error, but for UI purposes,
	// returning 0s is better than crashing or showing nothing if at least one succeeded.
	// If all are 0, it might be a total failure or an empty library.

	return artists, albums, tracks, nil
}

// HasPlexPass checks if the connected server supports Plex Pass features.
func (s *LibraryService) HasPlexPass(ctx context.Context) (bool, error) {
	// Query the server's identity endpoint to check for Plex Pass
	endpoint := fmt.Sprintf("%s/", s.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return false, err
	}
	s.addPlexHeaders(req)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	// Look for plexPass or subscription fields in the response
	var container struct {
		PlexPass         bool `json:"plexPass"`
		OwnerHasPlexPass bool `json:"ownerHasPlexPass"`
		MyPlex           struct {
			SubscriptionActive bool `json:"subscriptionActive"`
		} `json:"myPlex"`
	}
	if err := json.Unmarshal(body, &container); err != nil {
		return false, err
	}
	return container.PlexPass || container.OwnerHasPlexPass || container.MyPlex.SubscriptionActive, nil
}

// HasSonicAnalysis checks if sonic analysis data is available for the library.
func (s *LibraryService) HasSonicAnalysis(ctx context.Context) (bool, error) {
	// First, check server-wide recentlyAdded endpoint
	recentURL := fmt.Sprintf("%s/library/recentlyAdded?type=10&X-Plex-Container-Size=50", s.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", recentURL, nil)
	if err == nil {
		s.addPlexHeaders(req)
		if resp, err := s.httpClient.Do(req); err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// Try to decode as tracks directly
				var trackContainer domain.PlexTrackContainer
				if err := decodePlexTrackContainer(body, &trackContainer); err == nil &&
					len(trackContainer.Metadata) > 0 {
					for _, track := range trackContainer.Metadata {
						if track.HasSonicAnalysis || track.MusicAnalysisVersion > 0 {
							return true, nil
						}
					}
				}
				// Check if the response contains album entries with children keys
				var albumContainer domain.PlexMediaContainer
				if err := decodePlexMediaContainer(body, &albumContainer); err == nil &&
					len(albumContainer.Metadata) > 0 {
					// Sample a few albums to check their children for sonic analysis
					checked := 0
					for _, album := range albumContainer.Metadata {
						if checked >= 5 {
							break // Only check first 5 albums to avoid too many requests
						}
						childKey := album.Key
						if childKey == "" {
							continue
						}
						// If key doesn't end with /children, append it
						if !strings.HasSuffix(childKey, "/children") {
							childKey = childKey + "/children"
						}
						childURL := childKey
						if !strings.HasPrefix(childURL, "http") {
							childURL = s.baseURL + childKey
						}
						childReq, err := http.NewRequestWithContext(ctx, "GET", childURL, nil)
						if err != nil {
							continue
						}
						s.addPlexHeaders(childReq)
						childResp, err := s.httpClient.Do(childReq)
						if err != nil {
							continue
						}
						childBody, _ := io.ReadAll(childResp.Body)
						childResp.Body.Close()
						if childResp.StatusCode == http.StatusOK {
							// Log a preview of the response to understand the format
							preview := string(childBody)
							if len(preview) > 1500 {
								preview = preview[:1500]
							}
							log.Debug("HasSonicAnalysis: children response", "album", album.Title, "preview", preview)

							var childContainer domain.PlexTrackContainer
							if err := decodePlexTrackContainer(childBody, &childContainer); err == nil {
								log.Debug(
									"HasSonicAnalysis: decoded tracks",
									"album",
									album.Title,
									"count",
									len(childContainer.Metadata),
								)
								for _, track := range childContainer.Metadata {
									log.Debug(
										"HasSonicAnalysis: track info",
										"title",
										track.Title,
										"hasSonic",
										track.HasSonicAnalysis,
										"analysisVersion",
										track.MusicAnalysisVersion,
									)
									if track.HasSonicAnalysis || track.MusicAnalysisVersion > 0 {
										log.Debug("HasSonicAnalysis: found sonic analysis", "track", track.Title)
										return true, nil
									}
								}
							} else {
								log.Debug("HasSonicAnalysis: failed to decode children", "album", album.Title, "err", err)
							}
						} else {
							log.Debug("HasSonicAnalysis: children request failed", "album", album.Title, "status", childResp.StatusCode)
						}
						checked++
					}
				}
			}
		}
	}

	// Query libraries and check for sonic analysis metadata
	libs, _, err := s.FetchLibraries(ctx)
	if err != nil {
		// If we can't fetch libraries, return false without error
		// since we already checked server-wide recentlyAdded
		return false, nil
	}
	for _, lib := range libs {
		// First, check library-level tracks via type=10
		endpoint := fmt.Sprintf(
			"%s/library/sections/%s/all?type=10&X-Plex-Container-Start=0&X-Plex-Container-Size=1",
			s.baseURL,
			lib.Key,
		)
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err == nil {
			s.addPlexHeaders(req)
			resp, err := s.httpClient.Do(req)
			if err == nil {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var container domain.PlexTrackContainer
					if err := decodePlexTrackContainer(body, &container); err == nil {
						for _, track := range container.Metadata {
							if track.HasSonicAnalysis || track.MusicAnalysisVersion > 0 {
								return true, nil
							}
						}
					}
				}
			}
		}

		// Check per-library recentlyAdded endpoint (always try this, even if /all failed)
		recentEndpoint := fmt.Sprintf(
			"%s/library/sections/%s/recentlyAdded?type=10&X-Plex-Container-Size=20",
			s.baseURL,
			lib.Key,
		)
		req2, err := http.NewRequestWithContext(ctx, "GET", recentEndpoint, nil)
		if err != nil {
			continue
		}
		s.addPlexHeaders(req2)
		resp2, err := s.httpClient.Do(req2)
		if err != nil {
			continue
		}
		body2, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		if resp2.StatusCode == http.StatusOK {
			var recentContainer domain.PlexTrackContainer
			if err := decodePlexTrackContainer(body2, &recentContainer); err == nil {
				for _, track := range recentContainer.Metadata {
					if track.HasSonicAnalysis || track.MusicAnalysisVersion > 0 {
						return true, nil
					}
				}
			}
			// Also check if the response contains album entries with children keys
			var albumContainer domain.PlexMediaContainer
			if err := decodePlexMediaContainer(body2, &albumContainer); err == nil {
				for _, album := range albumContainer.Metadata {
					if album.Key != "" && strings.Contains(album.Key, "/children") {
						// Fetch children to check for sonic analysis
						childURL := album.Key
						if !strings.HasPrefix(childURL, "http") {
							childURL = s.baseURL + album.Key
						}
						childReq, err := http.NewRequestWithContext(ctx, "GET", childURL, nil)
						if err != nil {
							continue
						}
						s.addPlexHeaders(childReq)
						childResp, err := s.httpClient.Do(childReq)
						if err != nil {
							continue
						}
						childBody, _ := io.ReadAll(childResp.Body)
						childResp.Body.Close()
						if childResp.StatusCode == http.StatusOK {
							var childContainer domain.PlexTrackContainer
							if err := decodePlexTrackContainer(childBody, &childContainer); err == nil {
								for _, track := range childContainer.Metadata {
									if track.HasSonicAnalysis || track.MusicAnalysisVersion > 0 {
										return true, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return false, nil
}

// FetchSonicallySimilarTracks returns tracks similar to the specified ratingKey.
func (s *LibraryService) FetchSonicallySimilarTracks(
	ctx context.Context,
	ratingKey string,
	limit int,
	maxDistance float64,
) ([]domain.Track, int, error) {
	if ratingKey == "" || limit <= 0 {
		return nil, 0, fmt.Errorf("ratingKey is required and limit must be positive")
	}
	// Try multiple plausible endpoints in order
	candidates := []string{
		fmt.Sprintf(
			"%s/library/metadata/%s/similar?type=10&size=%d&maxDistance=%f",
			s.baseURL,
			ratingKey,
			limit,
			maxDistance,
		),
		fmt.Sprintf(
			"%s/library/metadata/%s/related?type=10&size=%d&maxDistance=%f",
			s.baseURL,
			ratingKey,
			limit,
			maxDistance,
		),
	}
	var combined []domain.Track
	var total int
	for _, endpoint := range candidates {
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			continue
		}
		s.addPlexHeaders(req)
		resp, err := s.httpClient.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		var container domain.PlexTrackContainer
		if err := decodePlexTrackContainer(body, &container); err != nil {
			log.Debug(
				"FetchSonicallySimilarTracks: failed to decode response",
				"endpoint",
				endpoint,
				"error",
				err.Error(),
			)
			continue
		}
		combined = append(combined, container.Metadata...)
		total += container.TotalSize
	}
	if len(combined) > 0 {
		return combined, total, nil
	}
	return nil, 0, fmt.Errorf("no supported similar endpoint worked for ratingKey %s", ratingKey)
}

// FetchSonicAdventure returns tracks representing a 'sonic adventure' between two tracks.
func (s *LibraryService) FetchSonicAdventure(ctx context.Context, start, end string) ([]domain.Track, int, error) {
	if start == "" || end == "" {
		return nil, 0, fmt.Errorf("start and end ratingKeys are required")
	}
	// Try the sonicAdventure endpoint
	endpoint := fmt.Sprintf("%s/library/sonicAdventure?start=%s&end=%s", s.baseURL, start, end)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	s.addPlexHeaders(req)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	var container domain.PlexTrackContainer
	if err := decodePlexTrackContainer(body, &container); err != nil {
		return nil, 0, err
	}
	return container.Metadata, container.TotalSize, nil
}

// FetchLibraryHubs fetches all hubs for a music library section (stations, mixes, on this day, etc.)
func (s *LibraryService) FetchLibraryHubs(ctx context.Context, sectionKey string) ([]domain.Hub, error) {
	// Fetch hubs for the music section - this includes stations, mixes, on this day, etc.
	endpoint := fmt.Sprintf("%s/hubs/sections/%s?includeStations=1", s.baseURL, sectionKey)
	log.Debug("FetchLibraryHubs: fetching hubs", "endpoint", endpoint, "sectionKey", sectionKey)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Log the raw response to understand the structure
	preview := string(body)
	if len(preview) > 2000 {
		preview = preview[:2000]
	}
	log.Debug("FetchLibraryHubs: response preview", "preview", preview)

	// Parse the hub response
	var hubResponse struct {
		MediaContainer struct {
			Hub []struct {
				HubIdentifier string            `json:"hubIdentifier"`
				Title         string            `json:"title"`
				Type          string            `json:"type"`
				Context       string            `json:"context"`
				Style         string            `json:"style"`
				Size          int               `json:"size"`
				Metadata      []json.RawMessage `json:"Metadata"` // Could be playlists, albums, or tracks
			} `json:"Hub"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &hubResponse); err != nil {
		return nil, fmt.Errorf("failed to parse hubs: %w", err)
	}

	var hubs []domain.Hub
	for _, rawHub := range hubResponse.MediaContainer.Hub {
		log.Debug(
			"FetchLibraryHubs: found hub",
			"identifier",
			rawHub.HubIdentifier,
			"title",
			rawHub.Title,
			"context",
			rawHub.Context,
			"type",
			rawHub.Type,
			"size",
			rawHub.Size,
		)

		hub := domain.Hub{
			HubIdentifier: rawHub.HubIdentifier,
			Title:         rawHub.Title,
			Type:          rawHub.Type,
			Context:       rawHub.Context,
			Style:         rawHub.Style,
			Size:          rawHub.Size,
		}

		// Try to parse metadata as playlists first, then albums, then tracks
		for _, rawItem := range rawHub.Metadata {
			// Check if it's a playlist-like item (has playlistType or type == "playlist")
			var itemType struct {
				Type         string `json:"type"`
				PlaylistType string `json:"playlistType"`
				Title        string `json:"title"`
				Key          string `json:"key"`
			}
			if err := json.Unmarshal(rawItem, &itemType); err == nil {
				log.Debug(
					"FetchLibraryHubs: parsing item",
					"type",
					itemType.Type,
					"playlistType",
					itemType.PlaylistType,
					"title",
					itemType.Title,
					"key",
					itemType.Key,
				)
				if itemType.PlaylistType != "" || itemType.Type == "playlist" {
					var pl domain.Playlist
					if err := json.Unmarshal(rawItem, &pl); err == nil {
						hub.Playlists = append(hub.Playlists, pl)
						continue
					}
				}
				if itemType.Type == "album" {
					var al domain.Album
					if err := json.Unmarshal(rawItem, &al); err == nil {
						hub.Albums = append(hub.Albums, al)
						continue
					}
				}
				if itemType.Type == "track" {
					var tr domain.Track
					if err := json.Unmarshal(rawItem, &tr); err == nil {
						hub.Tracks = append(hub.Tracks, tr)
						continue
					}
				}
				// If it has a title and key but no recognized type, treat as a playlist/station
				if itemType.Title != "" && itemType.Key != "" && itemType.Type == "" {
					var pl domain.Playlist
					if err := json.Unmarshal(rawItem, &pl); err == nil {
						hub.Playlists = append(hub.Playlists, pl)
						continue
					}
				}
			}
			// Default: try to decode as playlist
			var pl domain.Playlist
			if err := json.Unmarshal(rawItem, &pl); err == nil && pl.Title != "" {
				hub.Playlists = append(hub.Playlists, pl)
			}
		}
		hubs = append(hubs, hub)
	}

	log.Debug("FetchLibraryHubs: parsed hubs", "count", len(hubs))
	return hubs, nil
}

// FetchMixesForYou attempts to fetch personalized mixes generated by the server.
// Uses the hubs endpoint for the current library section.
func (s *LibraryService) FetchMixesForYou(ctx context.Context) ([]domain.Playlist, int, error) {
	// Try to get the first music library for section key
	libs, _, err := s.FetchLibraries(ctx)
	if err != nil || len(libs) == 0 {
		return nil, 0, fmt.Errorf("no music library available")
	}
	sectionKey := libs[0].Key

	// Fetch all hubs for the section
	hubs, err := s.FetchLibraryHubs(ctx, sectionKey)
	if err != nil {
		return nil, 0, err
	}

	// Find playlists/mixes from the hubs that are stations or mixes
	var playlists []domain.Playlist
	for _, hub := range hubs {
		log.Debug(
			"FetchMixesForYou: evaluating hub",
			"identifier",
			hub.HubIdentifier,
			"title",
			hub.Title,
			"context",
			hub.Context,
			"type",
			hub.Type,
			"playlists",
			len(hub.Playlists),
		)
		isStation := strings.Contains(strings.ToLower(hub.Context), "station") ||
			strings.Contains(strings.ToLower(hub.HubIdentifier), "station") ||
			strings.Contains(strings.ToLower(hub.Title), "station") ||
			strings.Contains(strings.ToLower(hub.Title), "radio") ||
			strings.Contains(strings.ToLower(hub.Title), "mix")
		if isStation {
			// If the hub has playlists, add them
			if len(hub.Playlists) > 0 {
				playlists = append(playlists, hub.Playlists...)
			} else {
				// The hub itself might be the station - treat the hub as a playlist entry
				// This happens when each hub IS a station (e.g., "Library Radio" hub)
				playlists = append(playlists, domain.Playlist{
					Title:        hub.Title,
					Key:          hub.HubIdentifier, // Use hubIdentifier as key for launching
					PlaylistType: "audio",
				})
			}
		}
	}

	return playlists, len(playlists), nil
}

// FetchOnThisDay returns albums that were released on today's date in history.
func (s *LibraryService) FetchOnThisDay(ctx context.Context) ([]domain.Album, int, error) {
	// Get music library section key
	libs, _, err := s.FetchLibraries(ctx)
	if err != nil || len(libs) == 0 {
		return nil, 0, fmt.Errorf("no music library available")
	}
	sectionKey := libs[0].Key

	// Fetch all hubs for the section
	hubs, err := s.FetchLibraryHubs(ctx, sectionKey)
	if err != nil {
		return nil, 0, err
	}

	// Find "on this day" hub - look for identifier containing "onthisday", "history", or related terms
	var albums []domain.Album
	for _, hub := range hubs {
		log.Debug(
			"FetchOnThisDay: evaluating hub",
			"identifier",
			hub.HubIdentifier,
			"title",
			hub.Title,
			"context",
			hub.Context,
			"albums",
			len(hub.Albums),
		)
		lowerID := strings.ToLower(hub.HubIdentifier)
		lowerTitle := strings.ToLower(hub.Title)
		lowerContext := strings.ToLower(hub.Context)
		isHistoryHub := strings.Contains(lowerID, "onthisday") ||
			strings.Contains(lowerID, "thisday") ||
			strings.Contains(lowerID, "history") ||
			strings.Contains(lowerTitle, "on this day") ||
			strings.Contains(lowerTitle, "today in") ||
			strings.Contains(lowerTitle, "years ago") ||
			strings.Contains(lowerTitle, "history") ||
			strings.Contains(lowerContext, "history")
		if isHistoryHub {
			albums = append(albums, hub.Albums...)
		}
	}

	return albums, len(albums), nil
}

// FetchMoodStation returns tracks for a named mood station.
// If station is empty, returns tracks from all mood/genre-related hubs.
func (s *LibraryService) FetchMoodStation(ctx context.Context, station string, limit int) ([]domain.Track, int, error) {
	// Get music library section key
	libs, _, err := s.FetchLibraries(ctx)
	if err != nil || len(libs) == 0 {
		return nil, 0, fmt.Errorf("no music library available")
	}
	sectionKey := libs[0].Key

	// Fetch all hubs for the section
	hubs, err := s.FetchLibraryHubs(ctx, sectionKey)
	if err != nil {
		return nil, 0, err
	}

	// Find mood/genre station hubs
	var tracks []domain.Track
	for _, hub := range hubs {
		log.Debug(
			"FetchMoodStation: evaluating hub",
			"station",
			station,
			"identifier",
			hub.HubIdentifier,
			"title",
			hub.Title,
			"context",
			hub.Context,
			"tracks",
			len(hub.Tracks),
			"playlists",
			len(hub.Playlists),
		)

		// If station is empty, collect from all mood/genre-like hubs
		if station == "" {
			// Look for mood, genre, or similar content hubs
			lowerID := strings.ToLower(hub.HubIdentifier)
			lowerTitle := strings.ToLower(hub.Title)
			lowerContext := strings.ToLower(hub.Context)
			isMoodHub := strings.Contains(lowerID, "mood") ||
				strings.Contains(lowerID, "genre") ||
				strings.Contains(lowerTitle, "mood") ||
				strings.Contains(lowerTitle, "genre") ||
				strings.Contains(lowerContext, "mood") ||
				strings.Contains(lowerContext, "genre")
			if isMoodHub {
				tracks = append(tracks, hub.Tracks...)
			}
		} else {
			// Match specific station name
			if strings.EqualFold(hub.Title, station) ||
				strings.Contains(strings.ToLower(hub.Title), strings.ToLower(station)) ||
				strings.Contains(strings.ToLower(hub.HubIdentifier), strings.ToLower(station)) {
				tracks = append(tracks, hub.Tracks...)
			}
		}
	}

	// Limit results if requested
	if limit > 0 && len(tracks) > limit {
		tracks = tracks[:limit]
	}

	return tracks, len(tracks), nil
}
