// Package logging provides logging infrastructure for the application
package logging

import (
	"context"
)

// Logger defines the logging interface used throughout the application
type Logger interface {
	// Debug logs a message at debug level
	Debug(msg string, keysAndValues ...any)

	// Info logs a message at info level
	Info(msg string, keysAndValues ...any)

	// Warn logs a message at warning level
	Warn(msg string, keysAndValues ...any)

	// Error logs a message at error level
	Error(msg string, keysAndValues ...any)

	// With returns a new logger with additional context
	With(keysAndValues ...any) Logger

	// WithContext returns a new logger with context
	WithContext(ctx context.Context) Logger
}
