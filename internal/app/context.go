package app

import (
	"context"
)

// AppContext represents the global application state, composed of focused contexts.
// It replaces the monolithic Coordinator facade by exposing contexts directly.
// This follows the pattern used in gh-dash where a lightweight context struct
// is passed around instead of a heavy coordinator.
type AppContext struct {
	// Core standard context for cancellation
	Ctx context.Context

	// State Contexts (Public for direct access)
	View     *ViewContext
	Content  *ContentState
	Session  *SessionContext
	Playback *PlaybackContext
	Services *Services
}

// NewAppContext creates a new application context with initialized sub-contexts.
func NewAppContext(services *Services) *AppContext {
	// Use the services context as the base context
	ctx := context.Background()
	if services != nil {
		ctx = services.Context()
	}

	return &AppContext{
		Ctx:      ctx,
		View:     NewViewContext(),
		Content:  NewContentState(),
		Session:  NewSessionContext(),
		Playback: NewPlaybackContext(),
		Services: services,
	}
}
