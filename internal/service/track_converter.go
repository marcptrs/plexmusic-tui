package service

// TrackConverter is a placeholder service for future track conversion logic.
// It's intentionally minimal to avoid circular imports with the app package.
// For now, track conversions (between app.Track and domain.Track) happen directly in library_page.go
// where both types are available. This service can be expanded later if needed.
type TrackConverter struct{}

// NewTrackConverter creates a new TrackConverter instance.
func NewTrackConverter() *TrackConverter {
	return &TrackConverter{}
}

// Note: AppToDomain and DomainToApp methods are not included here because they require
// importing both app and domain packages, which would create a circular dependency
// (app.Coordinator imports service package, so service cannot import app).
// Instead, these conversions are handled in library_page.go where the full type information is available.
