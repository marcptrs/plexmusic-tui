package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Config holds logging configuration
type Config struct {
	Level       slog.Level
	Source      bool
	LogToFile   bool
	LogFilePath string
	UseJSON     bool // Use JSON format (false for plain text)
}

// DefaultConfig returns default logging configuration
func DefaultConfig() *Config {
	return &Config{
		Level:       slog.LevelInfo,
		Source:      true,
		LogToFile:   true, // Default to file logging for TUI applications
		LogFilePath: getDefaultLogFilePath(),
		UseJSON:     false, // Use plain text for file logs (more human-readable)
	}
}

// newHandler creates the appropriate slog handler based on format preference
func newHandler(writer io.Writer, level slog.Level, useJSON bool) slog.Handler {
	options := &slog.HandlerOptions{
		Level: level,
	}

	if useJSON {
		return slog.NewJSONHandler(writer, options)
	} else {
		return slog.NewTextHandler(writer, options)
	}
}

// getDefaultLogFilePath returns the default log file path
func getDefaultLogFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/plexmusic-tui.log"
	}
	return filepath.Join(homeDir, ".config", "plexmusic-tui", "logs", "app.log")
}

// Setup initializes the slog logger with the given configuration
func Setup(cfg *Config) *slog.Logger {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var handler slog.Handler

	if cfg.LogToFile {
		// Ensure the log directory exists
		logDir := filepath.Dir(cfg.LogFilePath)
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			// Fall back to stdout if we can't create the directory
			handler = newHandler(os.Stdout, cfg.Level, cfg.UseJSON)
		} else {
			// Open the log file for append
			file, err := os.OpenFile(cfg.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				// Fall back to stdout if we can't open the file
				handler = newHandler(os.Stdout, cfg.Level, cfg.UseJSON)
			} else {
				handler = newHandler(file, cfg.Level, cfg.UseJSON)
			}
		}
	} else {
		// Default to stdout with JSON (for CLI tools)
		handler = newHandler(os.Stdout, cfg.Level, true)
	}

	return slog.New(handler).With("source", cfg.Source)
}

// GetLogger retrieves a logger with optional key-value pairs
func GetLogger(logger *slog.Logger, keysAndValues ...any) Logger {
	adapter := NewSlogAdapter(logger)
	if len(keysAndValues) > 0 {
		return adapter.With(keysAndValues...)
	}
	return adapter
}

// SetupWithAdapter initializes the slog logger and returns a Logger interface
func SetupWithAdapter(cfg *Config) Logger {
	slogLogger := Setup(cfg)
	return NewSlogAdapter(slogLogger)
}

// SetupWithFileLogging creates a logger that writes to both file and stdout
func SetupWithFileLogging() Logger {
	cfg := DefaultConfig()
	cfg.LogToFile = true
	return SetupWithAdapter(cfg)
}

// SetupWithFileLoggingAtLevel creates a logger with file logging at specified level
func SetupWithFileLoggingAtLevel(level slog.Level) Logger {
	cfg := DefaultConfig()
	cfg.LogToFile = true
	cfg.Level = level
	return SetupWithAdapter(cfg)
}

// SetupWithStdoutLogging creates a logger that writes to stdout (for CLI tools)
func SetupWithStdoutLogging() Logger {
	cfg := DefaultConfig()
	cfg.LogToFile = false // Explicitly disable file logging
	return SetupWithAdapter(cfg)
}

// SetupWithStdoutLoggingAtLevel creates a stdout logger at specified level
func SetupWithStdoutLoggingAtLevel(level slog.Level) Logger {
	cfg := DefaultConfig()
	cfg.LogToFile = false
	cfg.Level = level
	return SetupWithAdapter(cfg)
}

// SetupWithFileLoggingJSON creates a file logger with JSON format
func SetupWithFileLoggingJSON() Logger {
	cfg := DefaultConfig()
	cfg.UseJSON = true
	return SetupWithAdapter(cfg)
}

// SetupWithFileLoggingJSONAtLevel creates a JSON file logger at specified level
func SetupWithFileLoggingJSONAtLevel(level slog.Level) Logger {
	cfg := DefaultConfig()
	cfg.UseJSON = true
	cfg.Level = level
	return SetupWithAdapter(cfg)
}

// SetupWithFileLoggingText creates a file logger with plain text format (default)
func SetupWithFileLoggingText() Logger {
	cfg := DefaultConfig()
	cfg.UseJSON = false
	return SetupWithAdapter(cfg)
}

// SetupWithFileLoggingTextAtLevel creates a text file logger at specified level
func SetupWithFileLoggingTextAtLevel(level slog.Level) Logger {
	cfg := DefaultConfig()
	cfg.UseJSON = false
	cfg.Level = level
	return SetupWithAdapter(cfg)
}

// DefaultLogger returns the default logger adapter
func DefaultLogger() Logger {
	return NewSlogAdapter(slog.Default())
}

// GetLoggerOrDefault returns the given logger or the default if nil
func GetLoggerOrDefault(logger Logger) Logger {
	if logger == nil {
		return DefaultLogger()
	}
	return logger
}
