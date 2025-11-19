package pages

import (
	"context"
	"testing"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"

	tea "github.com/charmbracelet/bubbletea"
)

// MockAuthService for testing
type MockAuthService struct{}

func (m *MockAuthService) Subscribe(ctx context.Context) <-chan pubsub.Event[service.AuthEvent] {
	return make(chan pubsub.Event[service.AuthEvent])
}

func (m *MockAuthService) AuthenticateUser(ctx context.Context, username, password string) (string, error) {
	return "mock-token", nil
}

func (m *MockAuthService) FetchServers(ctx context.Context, token string) ([]domain.PlexServer, error) {
	return []domain.PlexServer{}, nil
}

func TestLoginPage_View_RendersHelp(t *testing.T) {
	coord := app.NewCoordinator()
	mockAuth := &MockAuthService{}
	page := NewLoginPage(coord, mockAuth)

	// Initialize the model (this sets up the help keys)
	page.Init()

	// Send WindowSizeMsg to set dimensions
	page.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := page.View()

	// Check for help text
	// Note: The help view might render with ANSI codes, so we might need to strip them or check for substrings carefully.
	// The help keys are "↑/↓" (Up/Down) and "enter" (Enter) and "q" (Quit)

	// We check for the key descriptions as they are likely to be present in the output
	expectedHelpParts := []string{"up", "down", "sign in", "quit"}

	for _, part := range expectedHelpParts {
		if !contains(view, part) {
			t.Errorf("Expected view to contain help text part %q, but it didn't.\nView:\n%s", part, view)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && search(s, substr)
}

func search(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
