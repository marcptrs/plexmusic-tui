package service

import (
	"context"
	"errors"

	log "github.com/charmbracelet/log/v2"

	"plexmusic-tui/internal/auth"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
)

// AuthEvent represents authentication-related events
type AuthEvent struct {
	Type    string
	Token   string
	Servers []domain.PlexServer
	Error   error
}

// AuthService provides authentication operations with event publishing
type AuthService struct {
	auth   *auth.Authenticator
	broker *pubsub.Broker[AuthEvent]
}

// NewAuthService creates a new authentication service
func NewAuthService() *AuthService {
	return &AuthService{
		auth:   auth.NewAuthenticator(),
		broker: pubsub.NewBroker[AuthEvent](),
	}
}

// Subscribe returns a channel for receiving authentication events
func (s *AuthService) Subscribe(ctx context.Context) <-chan pubsub.Event[AuthEvent] {
	return s.broker.Subscribe(ctx)
}

// AuthenticateUser authenticates a user with Plex credentials
func (s *AuthService) AuthenticateUser(
	ctx context.Context,
	username, password string,
) (string, error) {
	token, err := s.auth.AuthenticateUser(ctx, username, password)
	if err != nil {
		// Do not publish auth failure events for context cancellations.
		// These are expected when the request is cancelled (e.g., page closed)
		// and shouldn't be shown as an actionable error to the user.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		s.broker.Publish(pubsub.Event[AuthEvent]{
			Type: "auth.failed",
			Payload: AuthEvent{
				Type:  "auth.failed",
				Error: err,
			},
		})
		return "", err
	}

	s.broker.Publish(pubsub.Event[AuthEvent]{
		Type: "auth.success",
		Payload: AuthEvent{
			Type:  "auth.success",
			Token: token,
		},
	})

	return token, nil
}

// FetchServers retrieves available Plex servers for a user
func (s *AuthService) FetchServers(ctx context.Context, token string) ([]domain.PlexServer, error) {
	servers, err := s.auth.FetchServers(ctx, token)
	if err != nil {
		// If the request was canceled, don't publish an event; the
		// caller has intentionally abandoned the work (e.g., the page was closed).
		if errors.Is(err, context.Canceled) {
			// Debug: request cancelled intentionally (page closed / user navigated away).
			log.Debug("AuthService.FetchServers: context canceled", "error", err)
			return nil, err
		}

		// If the request timed out, record and publish an event; the UI may
		// want to treat deadline exceeded as retryable.
		if errors.Is(err, context.DeadlineExceeded) {
			// Deadline exceeded is worth recording as a warning so we can
			// distinguish timeouts vs. other failures during diagnostics.
			log.Warn("AuthService.FetchServers: deadline exceeded", "error", err)
			s.broker.Publish(pubsub.Event[AuthEvent]{
				Type: "servers.fetch_failed",
				Payload: AuthEvent{
					Type:  "servers.fetch_failed",
					Error: err,
				},
			})
			return nil, err
		}

		// All other errors are actionable; log and publish as a failed fetch.
		log.Error("AuthService.FetchServers: error", "error", err)
		s.broker.Publish(pubsub.Event[AuthEvent]{
			Type: "servers.fetch_failed",
			Payload: AuthEvent{
				Type:  "servers.fetch_failed",
				Error: err,
			},
		})
		return nil, err
	}

	s.broker.Publish(pubsub.Event[AuthEvent]{
		Type: "servers.loaded",
		Payload: AuthEvent{
			Type:    "servers.loaded",
			Servers: servers,
		},
	})

	return servers, nil
}

// Close closes the service and releases resources
func (s *AuthService) Close() error {
	s.broker.Close()
	return nil
}
