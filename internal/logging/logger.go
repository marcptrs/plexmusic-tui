package logging

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"log/slog"

	"github.com/charmbracelet/log/v2"
)

// DefaultLogger is the package-level logger used by the application.
// It is safe to use (read-only) without explicit initialization: it will
// fall back to charm's default logger.
var DefaultLogger *log.Logger = log.Default()

// InitFileLogger initializes and returns a charm logger that writes to the
// provided file path. If the file can't be opened, it returns an error
// and the default logger remains unchanged.
//
// The function also sets sensible options for file logging: timestamps are
// enabled and the log level is preserved from the current default logger.
func InitFileLogger(path string, level log.Level) (*log.Logger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open file logger: %w", err)
	}

	return NewLoggerWithWriter(f, level), nil
}

// NewLoggerWithWriter constructs a charm logger backed by the provided writer.
// This is useful for testing or for wiring loggers to non-files (e.g. io.Pipe).
func NewLoggerWithWriter(w io.Writer, level log.Level) *log.Logger {
	l := log.NewWithOptions(w, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
		Level:           level,
	})
	return l
}

// SetDefaultLogger replaces the package default logger with the provided
// charm logger. Callers who need a centralized logger (for example, wiring a
// default across packages) should call this once during application
// initialization.
func SetDefaultLogger(l *log.Logger) {
	if l != nil {
		DefaultLogger = l
	}
}

// SetLevel sets the logging level on the default logger.
func SetLevel(level log.Level) {
	if DefaultLogger != nil {
		DefaultLogger.SetLevel(level)
	}
}

// ParseLevel parses a string into a charm log.Level. Supported values are
// the same as charm's log package (debug, info, warn, error, fatal).
func ParseLevel(s string) (log.Level, error) {
	return log.ParseLevel(s)
}

// charmSlogAdapter implements slog.Handler using a charm logger instance.
type charmSlogAdapter struct {
	logger *log.Logger
}

// Enabled reports whether logging at the given level is enabled.
// We return true to allow slog to call Handle; charm log can still decide
// what to emit based on its own configured level inside Handle.
func (a *charmSlogAdapter) Enabled(ctx context.Context, level slog.Level) bool {
	// Best-effort: we could map levels and query the charm logger; however,
	// the charm log API in v2 does not expose a per-call Enabled check.
	// Returning true is safe — charm logger will filter messages based on its level.
	return true
}

// Handle formats the slog.Record attributes and forwards them to charm log.
func (a *charmSlogAdapter) Handle(ctx context.Context, r slog.Record) error {
	// Convert Attributes to key/value pairs for charm log
	kv := make([]any, 0)

	// r.Attrs passes each attribute to the function; use Any() to obtain the value.
	r.Attrs(func(attr slog.Attr) bool {
		kv = append(kv, attr.Key, attr.Value.Any())
		return true
	})

	// Add the record time and source (if available) as additional fields. This
	// is optional since charm log can add timestamps, but including Time may
	// provide parity with slog formatting expectations.
	kv = append(kv, "time", r.Time)

	// Forward message and fields to the matching charm log level.
	switch r.Level {
	case slog.LevelDebug:
		a.logger.Debug(r.Message, kv...)
	case slog.LevelInfo:
		a.logger.Info(r.Message, kv...)
	case slog.LevelWarn:
		a.logger.Warn(r.Message, kv...)
	case slog.LevelError:
		a.logger.Error(r.Message, kv...)
	default:
		// Fallback to info for unspecified levels
		a.logger.Info(r.Message, kv...)
	}

	return nil
}

// WithAttrs returns a new handler with the provided attributes attached.
func (a *charmSlogAdapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	if a == nil || a.logger == nil {
		return a
	}
	kvs := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		kvs = append(kvs, attr.Key, attr.Value.Any())
	}
	return &charmSlogAdapter{logger: a.logger.With(kvs...)}
}

// WithGroup returns a new handler that represents a new logical group.
// We map slog's grouping into charm log by storing the group name as a "group" key.
func (a *charmSlogAdapter) WithGroup(name string) slog.Handler {
	if a == nil || a.logger == nil {
		return a
	}
	if name == "" {
		return a
	}
	return &charmSlogAdapter{logger: a.logger.With("group", name)}
}

// AsSlogHandler returns a slog.Handler implementation backed by charm log.
// This allows packages using slog to be routed into charm log's output/format.
func AsSlogHandler() slog.Handler {
	// Use the package's DefaultLogger as the target; fall back to the package
	// default charm logger if not initialized.
	var target *log.Logger
	if DefaultLogger == nil {
		target = log.Default()
	} else {
		target = DefaultLogger
	}
	return &charmSlogAdapter{logger: target}
}
