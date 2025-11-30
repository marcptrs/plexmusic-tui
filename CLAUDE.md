# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A terminal user interface (TUI) for browsing and playing music from Plex Media Servers, built with Go and the Bubble Tea framework.

## Common Commands

### Building
```bash
# Development build
go build -o plexmusic-tui

# Release build with optimizations
go build -ldflags="-s -w" -o plexmusic-tui
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run specific package tests
go test ./internal/service/...
```

### Running
```bash
# Run the application
./plexmusic-tui

# View recent logs
./plexmusic-tui -logs

# Enable debug logging
./plexmusic-tui -debug

# Dump raw view output for debugging
./plexmusic-tui -dump-view
```

### Dependency Management
```bash
# Download dependencies
go mod download

# Update dependencies
go mod tidy

# Generate Wire dependency injection code
go generate ./internal/bootstrap
```

## Architecture Overview

This project follows a layered architecture with strict separation of concerns:

### Dependency Injection (Wire)
- Uses Google Wire for compile-time dependency injection
- Wire providers are defined in `internal/bootstrap/providers.go`
- Wire sets and initialization in `internal/bootstrap/wire.go`
- Generated code in `internal/bootstrap/wire_gen.go` (do not edit manually)
- Run `go generate ./internal/bootstrap` after modifying Wire configuration

### Core Layers

**1. Domain Layer** (`internal/domain/`)
- Pure domain models: `Album`, `Track`, `Playlist`, `MusicLibrary`
- No external dependencies
- Validation logic included

**2. Service Layer** (`internal/service/`)
- Business logic and external API interactions
- Interfaces defined in `interfaces.go`
- Key services: `LibraryService`, `AuthService`, `PlaybackService`
- Services implement `pubsub.Subscriber` for event publishing

**3. Application Layer** (`internal/app/`)
- `Coordinator`: Central state container coordinating services
- Message types in `messages.go` for Bubble Tea communication
- Thread-safe with mutex protection for concurrent access

**4. TUI Layer** (`internal/tui/`)
- Bubble Tea-based user interface
- **AppModel pattern**: Top-level model owns global keybindings and router
- **PageFactory pattern**: Creates pages on-demand to avoid package cycles
- Router handles page lifecycle (Init, Update, View, Close)

### TUI Architecture Details

The TUI follows the "AppModel + PageFactory" pattern (inspired from `tmp/crush/`):

- **AppModel** (`internal/tui/app_model.go`):
  - Owns canonical `KeyMap` for global bindings (quit, help)
  - Handles top-level keys before delegation
  - Responds to `PageChangeMsg` by invoking `pageFactory`
  - Coordinates shutdown and cleanup

- **Router** (`internal/tui/router.go`):
  - Manages page lifecycle
  - Delegates Update/View to current page
  - Secondary global key check (defensive)

- **PageFactory**:
  - Callback that creates concrete page implementations
  - Avoids package cycles between `tui` and `tui/pages`
  - Captures minimal dependencies (coordinator, services, config)

- **Pages** (`internal/tui/pages/`):
  - Self-contained `tea.Model` implementations
  - Split into multiple files per page:
    - `*_page.go`: Struct, Init, Close, constructor
    - `*_page_view.go`: View() rendering
    - `*_page_events.go`: Update() and event handlers
    - `*_page_keys.go`: Key handling logic
  - Examples: `LoginPage`, `ServerSelectionPage`, `LibraryPage`

### Supporting Packages

**Pub/Sub** (`internal/pubsub/`)
- Generic type-safe event broker: `Broker[T]`
- Context-based subscription and cleanup
- Used for decoupled service-to-UI communication

**Styles** (`internal/tui/styles/`)
- Single source of truth for TUI styling
- Color palette, named lipgloss styles (`TitleStyle`, `FocusedStyle`, etc.)
- Import this for all UI styling needs

**Components** (`internal/tui/components/`)
- Reusable UI components (StatusBar, NowPlaying, etc.)
- Implement standard interfaces (`Sizeable`, `Focusable`)

**Utilities** (`internal/tui/util/`)
- View/layout helpers (not styles)
- `InfoMsg` for status messages
- `ReportError`, `ReportSuccess` helpers

**Config** (`internal/config/`)
- Configuration loading/saving
- Stored in `~/.config/plexmusic-tui/config.json`
- Manages auth token and last selected server

**Image Rendering** (`internal/image/`)
- Multi-protocol support: Kitty, iTerm2, Sixel, Unicode half-blocks
- Exact-size pixel canvas to avoid fractional scaling
- Content-hash based caching (SHA-256 of PNG bytes)
- Kitty uses numeric image IDs derived from content hash
- CLI flags: `-force-renderer`, `-render-debug` for debugging

**Playback** (`internal/playback/`)
- Audio streaming and control using Beep library
- Supports MP3, FLAC, WAV, Vorbis/OGG formats

**Logging** (`internal/logging/`)
- Logs stored in system cache directory:
  - Linux: `~/.cache/plexmusic-tui/plexmusic.log`
  - macOS: `~/Library/Caches/plexmusic-tui/plexmusic.log`
  - Windows: `%LocalAppData%\plexmusic-tui\plexmusic.log`

## Key Patterns and Guidelines

### Global Key Handling
- Global keys (quit, help) are owned by `AppModel`
- AppModel checks keys BEFORE delegating to router/pages
- This ensures quit works even when text inputs are focused
- Pages should NOT implement quit handling directly

### Service Interfaces
- All services have interfaces in `internal/service/interfaces.go`
- Enable testability via mocks
- Services embed `pubsub.Subscriber` for event support

### State Management
- Coordinator protects state with mutex for thread safety
- Avoid goroutine races in page Init/Close methods
- Clear renderer caches when content changes (e.g., album art updates)

### Message-Based Communication
- Use custom message types defined in `internal/app/messages.go`
- `CoordinatorMsg` with typed operations for app-level communication
- Command helpers for async operations

### Testing
- Mock services using interfaces
- Test pages in isolation
- Integration tests for event flow
- See existing `*_test.go` files for patterns

### Styling Conventions
- **Always** import `internal/tui/styles` for canonical styles
- Use `internal/tui/util` for view helpers only
- Don't create style definitions outside `styles` package
- Color palette and style tokens are centralized

## Cross-Compilation

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o plexmusic-tui-linux-amd64

# macOS ARM64 (M1/M2)
GOOS=darwin GOARCH=arm64 go build -o plexmusic-tui-darwin-arm64

# macOS AMD64 (Intel)
GOOS=darwin GOARCH=amd64 go build -o plexmusic-tui-darwin-amd64

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o plexmusic-tui.exe
```

## Configuration

Server selection uses canonical `hostname/server-name` keys to persist last selected server. Auth tokens are stored securely in the config file.

## Important Notes

- TLS: Automatically handles self-signed certificates for local Plex servers
- Thread Safety: Coordinator state is mutex-protected; be careful with goroutines
- Album Art: Renderer clears hash cache on art changes to avoid stale content
- Wire: Always regenerate after changing providers (`go generate ./internal/bootstrap`)
