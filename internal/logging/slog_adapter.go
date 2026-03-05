package logging

import (
	"context"
	"log/slog"
)

// SlogAdapter adapts a slog.Logger to the Logger interface
type SlogAdapter struct {
	logger *slog.Logger
}

// NewSlogAdapter creates a new SlogAdapter
func NewSlogAdapter(logger *slog.Logger) *SlogAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &SlogAdapter{logger: logger}
}

// Debug logs a message at debug level
func (s *SlogAdapter) Debug(msg string, keysAndValues ...any) {
	s.logger.Debug(msg, keysAndValues...)
}

// Info logs a message at info level
func (s *SlogAdapter) Info(msg string, keysAndValues ...any) {
	s.logger.Info(msg, keysAndValues...)
}

// Warn logs a message at warning level
func (s *SlogAdapter) Warn(msg string, keysAndValues ...any) {
	s.logger.Warn(msg, keysAndValues...)
}

// Error logs a message at error level
func (s *SlogAdapter) Error(msg string, keysAndValues ...any) {
	s.logger.Error(msg, keysAndValues...)
}

// With returns a new logger with additional context
func (s *SlogAdapter) With(keysAndValues ...any) Logger {
	return &SlogAdapter{logger: s.logger.With(keysAndValues...)}
}

// WithContext returns a new logger with context
func (s *SlogAdapter) WithContext(ctx context.Context) Logger {
	// slog.Logger doesn't have a WithContext method, so we just return the same logger
	// This could be enhanced in the future if needed
	return s
}
