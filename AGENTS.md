# AGENTS.md

Repository: plexmusic-tui

## Purpose
Terminal User Interface (TUI) application for browsing and playing music from a Plex Media Server. Provides authentication, server/library discovery, album/playlist browsing, audio playback, and inline album art rendering across multiple terminal graphics protocols (Kitty, iTerm2, Sixel, Unicode blocks fallback).

## Tech Stack
- Language: Go (go 1.24.x) — see `go.mod` for toolchain
- UI: Charmbracelet Bubble Tea, Bubbles, Lipgloss
- Audio: faiface/beep (mp3, flac, ogg/vorbis, wav decoding + speaker)
- Imaging: disintegration/imaging for resizing; custom renderer for multiple terminal protocols
- HTTP/TLS: custom wrapper in `internal/http` auto-skips TLS verify for local/private hosts

## Build / Run / Test / Lint
Observed commands (only include those seen in config/CI):
- Build (CI): `go build -v -o plexmusic-tui .` (build.yml)
- Local build (README): `go build -o plexmusic-tui` (standard)
- Release build (README): `go build -ldflags="-s -w" -o plexmusic-tui`
- Cross-compilation examples in README using `GOOS` / `GOARCH`
- Run: `./plexmusic-tui`
- Tests (CI): `go test -v -race -coverprofile=coverage.out ./...` (test.yml)
- Basic tests (README): `go test ./...`
- Component tests: `go test ./internal/pubsub/` (pubsub broker has comprehensive tests)
- Lint (CI): golangci-lint action with `.golangci.yml` configuration. Local equivalent would be: `golangci-lint run` (inferred from presence of config; not explicitly documented in repo commands)

Do not invent other commands.

## Directory Structure (observed)
```
internal/
  app/            Coordinator (central state container) + messages
  auth/           Authentication logic
  config/         Load/Save JSON config with Manager wrapper for convenience
  domain/         Shared domain models / enums
  http/           Plex-aware HTTP client with local TLS relax logic
  image/          Terminal image protocol detection + rendering
  playback/       Audio player implementation (state, play/pause/seek)
  plex/           Deprecated legacy Plex client (use service layer instead)
  pubsub/         Event broker for pub/sub architecture (generic, type-safe)
  service/        AuthService, LibraryService + service interfaces
  tui/            TUI infrastructure (pages, router, page types)
    pages/        Individual page components (login, server_selection)
    components/   Reusable UI components (StatusBar, etc.)
    util/         TUI utilities (Model/Sizeable/Focusable interfaces, message helpers)
  ui/             Styles, view helpers, layout calculations
main.go           Bubble Tea model + transitional integration with Coordinator
.golangci.yml     Linter configuration (enabled/disabled linters, formatters)
.github/workflows build.yml, test.yml, lint.yml
ARCHITECTURE.md   Refactoring plan following tmp/crush patterns
README.md         User-oriented setup and usage docs
CRUSH.md          (this file)
```
A large `tmp/crush/` tree exists containing unrelated development tooling code (appears to be an embedded or copied agent framework). Patterns from `tmp/crush/` are being applied to main codebase architecture (see ARCHITECTURE.md).

## State Management Patterns
**Current (Transitional)**:
1. Legacy Bubble Tea model in `main.go` holds duplicated state fields (servers, libraries, albums, playlists, tracks, playback primitives, album art cache, tabs, queue). 
2. Centralized `Coordinator` (`internal/app/coordinator.go`) provides getters/setters for nearly identical state + navigation helpers. Migration in progress (comment in main.go line ~135). Prefer adding new stateful logic in Coordinator to reduce duplication.

**Target Architecture (Following tmp/crush patterns - IN PROGRESS)**:
- **Page Components**: Self-contained Tea.Model implementations in `internal/tui/pages/` (LoginPage, ServerSelectionPage). Each page:
  - Owns UI state (inputs, selections, error messages)
  - Subscribes to service events via context-based channels
  - Publishes PageChangeMsg for navigation
  - Implements Close() for cleanup
- **Router**: `internal/tui/router.go` manages page lifecycle (Init/Close), transitions, and delegates Update/View
- **Pub/Sub Events**: Generic `pubsub.Broker[T]` for type-safe event distribution. Services publish events (auth.success, servers.loaded), pages subscribe.
- **Service Interfaces**: Define contracts (`LibraryServicer`, `AuthServicer` in `internal/service/interfaces.go`) for testability and dependency injection.
- **Coordinator as State Store**: Gradually migrate to Coordinator storing domain state while pages handle UI concerns. Services remain stateless event publishers.
- **Message-Based Navigation**: PageChangeMsg triggers router transitions between pages.
- **Config Manager**: `config.Manager` wraps Config with getters/setters for token and last-selected-server persistence.

See `ARCHITECTURE.md` for detailed refactoring plan and migration strategy.

## Networking / Plex Access
- Preferred abstraction: `service.LibraryService` for fetching libraries, albums, recently added (server-scoped and library-scoped), playlists, tracks. Recent changes added support for recentlyAdded scoped to a specific library via `FetchRecentlyAddedInLibrary`.
- Deprecated: `plex.Client` (marked with comments at top). Retain for backward compatibility but do not add new features there.
- Image and track URLs built using scheme/host/port plus media key with `X-Plex-Token` query/header. `BuildStreamURL` prefers media part keys when available and avoids duplicating `X-Plex-Token` query params.
- HTTP client selection via `http.GetForHost(host)` that enables `InsecureSkipVerify` for local/private networks only.

## Audio Playback
- `playback.Player` manages decode, control, volume, position updating.
- Supported formats chosen by content-type sniffing (mp3, flac, ogg/vorbis, wav) then fallback attempt to mp3.
- Must call `Initialize()` after stream load and before `Play()`.
- Volume handled by `effects.Volume` (range effectively arbitrary; repository uses direct assignment). Keep changes consistent.

## Image Rendering
- `image.Renderer` auto-detects protocol (Kitty, iTerm2, Sixel, Unicode fallback) based on environment vars: `TERM`, `KITTY_WINDOW_ID`, `TERM_PROGRAM`.
- Unicode fallback uses lipgloss styling of upper half-block characters with foreground/background colors for two vertical pixels.
- Kitty rendering transmits PNG in chunks (4096 base64 chars) then places sub-cells via virtual placement (a=p). Cache keyed by protocol + dimensions + image bounds.
- `NewRendererWithProtocol(ProtocolUnicodeBlocks)` used for playback dedicated renderer (always fallback) while general renderer auto-detects.

## Configuration
- Stored at `~/.config/plexmusic-tui/config.json` via `internal/config` package.
- Fields: `authToken`, `lastSelectedServer` (canonical host/name key, e.g., `hostname/server-name`).
- `config.Load()` returns empty struct if file missing; saving ensures directory creation (0600 file permissions for security).

## UI / Layout
- Pane width calculations in `internal/ui/views.go` allocate percentages (navigation 20%, content 30%, detail 40%) with minimal widths and total width adjustment subtracting 6 for borders/padding. Recent UI changes favor a Now Playing-first layout where Now Playing is the main content and the tabs draw a bottom drawer/modal overlay for Recently Added, Playlists, Search, and Settings.
- Album art display uses dedicated renderer; playback pane keeps separate cached art.
- Style definitions reside in `internal/ui/styles.go` (not inspected—future agents should read to follow naming and color patterns before altering styles). A new ScrimStyle was added to dim the Now Playing content when a modal/drawer is active.
- Tab navigation logic lives in model and Coordinator (`NextTab`, `PreviousTab`) cycling enumerated tab values. Enter now opens the active tab as a drawer; keys Enter/Space/Esc supported inside the drawer to respectively open, play, and close.

## Enumerations / Conventions
- Enum groups implemented as typed int constants in each package (`SessionState`, `ContentViewType`, `PlaybackState`, `TabType`, etc.). Avoid scattering new enums in multiple packages; centralize to `internal/domain` unless strictly UI-specific.
- Domain models use Plex JSON field tags matching API responses (camelCase from Plex). Maintain existing tags when extending.

## Deprecated Code Policy
- `internal/plex/client.go` annotated Deprecated; do not enhance or copy patterns from it. When refactoring older call sites, migrate them to `service.LibraryService` methods.

## Error Handling Patterns
- Service and client methods wrap errors with contextual `fmt.Errorf("...: %w", err)` messages and include HTTP status with body excerpt when status != 200.
- Continue this pattern; include endpoint or operation detail but avoid logging secrets (tokens not logged).

## TLS / Security Notes
- Local/private host detection influences TLS verification skipping. Do not bypass `isLocalAddress` logic; extending host detection must remain conservative. Never disable verification for arbitrary public domains.
- Config file stored with permission 0600; maintain same permissions for any new sensitive files.
- Avoid logging Plex auth token.

## Testing
- CI runs `go test -v -race -coverprofile=coverage.out ./...`.
- Component tests exist for `internal/service` and `internal/tui/pages` demonstrating server response handling and UI behavior (library fetch, drawer/modal interactions, stream URL building). When adding tests for core app, follow standard Go test layout (`*_test.go` alongside code). Include race-safety for concurrent playback/network manipulations.
- Tests now include coverage for:
  - `LibraryService.FetchRecentlyAddedInLibrary` and default `FetchRecentlyAdded` decoding for wrapped Plex responses (MediaContainer, Response, Playlist arrays).
  - `BuildStreamURL` ensuring media part keys are preferred and token duplication is avoided.
  - UI page interactions ensuring drawer modal behavior and that the Now Playing-first layout renders expected content and help hints.

## Linting / Formatting
`.golangci.yml` enables:
- bodyclose, noctx, rowserrcheck (HTTP/DB safety)
- goprintffuncname, misspell, nolintlint, staticcheck, whitespace, tparallel
- sqlclosecheck (database safety though no DB used yet)
Disabled (too strict for TUI): errcheck, ineffassign, unused.
Formatters: gofumpt, goimports. Keep code formatted; run `golangci-lint run` locally before large changes.

## Queue / Playback Interaction
- Queue maintained as `[]track` plus `queueIndex` with modal visibility flag `showQueueModal`.
- Coordinator mirrors queue API; extend Coordinator for new queue features (shuffle, repeat) and adapt UI logic accordingly.

## Migration Guidance
- Unify duplicated state: gradually replace direct model fields usage with Coordinator accessors. New features should extend Coordinator first.
- Before adding new operations (e.g., search, settings persistence), create appropriate service abstractions rather than embedding logic in `main.go`.

## Gotchas / Non-Obvious Details
- Image renderer chunking for Kitty requires exact escape sequence format (`\x1b_G...\x1b\`). Altering chunk size or protocol fields incorrectly breaks display; reuse helper functions—do not inline sequences.
- Content-type detection for audio may mislabel some Plex streams; fallback MP3 decode attempt is intentional. When adding formats, preserve fallback order.
- TLS skip only for private/local; adding broader pattern risks security issues.
- Cache keys for images include bounds min coordinates; altering key composition invalidates existing cache expectations.
- Two album art caches (current vs playback). Avoid merging unless sure of usage semantics (one may display album being browsed, other track actively playing).

## Adding Features Safely
1. Extend domain types in `internal/domain` with new fields (retain JSON tags).
2. Add service methods in `internal/service` for new Plex endpoints (consistent error wrapping, header usage via helper).
3. Update Coordinator with accessor methods for new state rather than exposing raw fields.
4. Adjust UI panes using existing width helper functions.
5. Provide tests focusing on concurrency and race conditions around playback if altering audio logic.

## When Editing Code
- Respect import grouping style (standard libs first, then third-party, then internal packages).
- Follow existing error wrapping and nil checks before indexing slices.
- Use typed enums rather than bare ints for view or playback state.

## Future Work Hooks (observed placeholders)
- Search, settings, queue enhancements (README planned features) — implement using same patterns: domain + service + coordinator + UI.
- Potential consolidation of duplicated session/playback state between model and Coordinator.

## Unverified / Not Documented
Not found (so excluded): makefile, database, CLI argument parsing for runtime configuration, logging system (only incidental usage), deployment scripts.

If new systems are added (e.g., logging, metrics), update this file with observed patterns only.

---
Generated from direct repository inspection. Update this file whenever architectural patterns change. Do not add speculative instructions.
