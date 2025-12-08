package app

import (
	"plexmusic-tui/internal/config"
)

// Coordinatorer defines the public methods pages and components use to
// access and update application state.
//
// Deprecated: Use AppContext directly. This interface is being phased out.
type Coordinatorer interface {
	// Access to the underlying context (for migration)
	GetAppContext() *AppContext

	// ConfigManager is kept for backward compatibility in tests if needed,
	// but should be accessed via AppContext.Services.ConfigManager()
	SetConfigManager(cfg *config.Manager)
}
