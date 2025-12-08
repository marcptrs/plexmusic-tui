package app

import (
	"sync"

	"plexmusic-tui/internal/domain"
)

// SessionContext manages authentication and server selection state.
// This separates session/auth concerns from content and playback.
type SessionContext struct {
	// Authentication
	token string
	err   error

	// Server and library selection
	servers         []domain.PlexServer
	selectedServer  int
	libraries       []domain.MusicLibrary
	selectedLibrary int

	mu sync.RWMutex
}

// NewSessionContext creates a new session context
func NewSessionContext() *SessionContext {
	return &SessionContext{
		selectedServer:  -1,
		selectedLibrary: -1,
	}
}

// Authentication

func (s *SessionContext) Token() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

func (s *SessionContext) SetToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
}

func (s *SessionContext) IsLoggedIn() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token != ""
}

func (s *SessionContext) Error() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *SessionContext) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// Servers

func (s *SessionContext) Servers() []domain.PlexServer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.servers
}

func (s *SessionContext) SetServers(servers []domain.PlexServer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.servers = servers
}

func (s *SessionContext) SelectedServer() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selectedServer
}

func (s *SessionContext) SetSelectedServer(idx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selectedServer = idx
}

func (s *SessionContext) GetCurrentServer() *domain.PlexServer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.selectedServer >= 0 && s.selectedServer < len(s.servers) {
		return &s.servers[s.selectedServer]
	}
	return nil
}

// Libraries

func (s *SessionContext) Libraries() []domain.MusicLibrary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.libraries
}

func (s *SessionContext) SetLibraries(libs []domain.MusicLibrary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.libraries = libs
}

func (s *SessionContext) SelectedLibrary() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selectedLibrary
}

func (s *SessionContext) SetSelectedLibrary(idx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selectedLibrary = idx
}
