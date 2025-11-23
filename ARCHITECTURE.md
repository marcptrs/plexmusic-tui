# Architecture Refactoring Plan

Following patterns from `tmp/crush/`, this refactoring improves separation of concerns, adds event-driven architecture, and creates proper layering.

## Key Changes

### 1. Directory Structure (COMPLETED)

```
internal/
├── app/              # Application orchestration (existing, enhanced)
│   ├── coordinator.go  # State container
│   └── messages.go     # Message types (existing)
├── domain/           # Domain models (existing)
├── service/          # Service layer (existing, add interfaces)
│   ├── interfaces.go   # Service interfaces (NEW)
│   └── library.go      # Implementation
├── pubsub/           # Event broker (NEW)
│   └── broker.go
├── tui/              # UI layer (NEW)
│   ├── components/   # Reusable UI components
│   │   └── statusbar.go
│   ├── page/         # Page components (TODO)
│   ├── styles/       # Theming and styles
│   │   └── styles.go
│   └── util/         # TUI utilities
│       └── util.go
├── auth/             # Authentication (existing)
├── config/           # Configuration (existing)
├── http/             # HTTP client (existing)
├── image/            # Image rendering (existing)
└── playback/         # Audio playback (existing)
```

### 2. Pub/Sub Event System (COMPLETED)

**Pattern**: Type-safe generic event broker for decoupled communication.

**Created**: `internal/pubsub/broker.go`
- Generic `Broker[T]` for type-safe events
- `Subscribe(ctx)` returns event channel
- `Publish(event)` broadcasts to subscribers
- Context-based cleanup

**Usage Example**:
```internal/service/interfaces.go#L1-40
package service

// (Example excerpt — not a literal file copy)
type LibraryServicer interface {
    pubsub.Subscriber[domain.Album]
    FetchLibraries() ([]domain.MusicLibrary, error)
    FetchAlbums(libraryKey string) ([]domain.Album, error)
}
```

### 3. Service Interfaces (COMPLETED)

**Pattern**: Define contracts for services, enable testability and composition.

**Created**: `internal/service/interfaces.go`

```internal/service/interfaces.go#L1-60
type LibraryServicer interface {
    pubsub.Subscriber[domain.Album]
    FetchLibraries() ([]domain.MusicLibrary, error)
    FetchAlbums(libraryKey string) ([]domain.Album, error)
    // ...
}
```

**Benefits**:
- Testable via mocks
- Swappable implementations
- Clear contracts
- Embeds `pubsub.Subscriber` for event support

### 4. TUI Layer (COMPLETED - Base Components)

**Pattern**: Separate UI logic into reusable components with standard interfaces.

**Created**:
- `internal/tui/util/util.go`: Core interfaces and helpers
  - `Model`, `Sizeable`, `Focusable` interfaces
  - `InfoMsg` for status messages
  - `ReportError`, `ReportSuccess` helpers
  - `CmdHandler` for message wrapping

- `internal/tui/components/statusbar.go`: Status bar component
  - Displays success/error/warning/info messages
  - Color-coded with icons
  - Implements `Sizeable` interface

- `internal/tui/styles/styles.go`: Centralized styling
  - Color palette constants
  - Common style definitions (titles, borders, tabs, buttons)
  - Helper functions for applying dimensions

### 5. Message-Based Communication (ENHANCED)

**Pattern**: Custom message types for decoupled component communication.

**Existing**: `internal/app/messages.go` already has good message structure
- `CoordinatorMsg` with typed operations
- Command helpers for async operations

**Recommended Next Steps**:
1. Add more specific message types for UI events (page changes, focus changes)
2. Move from `interface{}` Data field to type-safe unions
3. Add message routing in App layer

---

## AppModel / pageFactory (Crush pattern) — New documentation

One key pattern borrowed from the `tmp/crush/` code is an application-level model that:
- Owns global key bindings (the KeyMap).
- Owns or wires the `Router`.
- Creates/loads pages only through a `pageFactory`.
- Handles app-wide messages (quit, page change) in a single place.

This resolves subtle input-handling issues by ensuring a single source of truth for global key behavior and a predictable lifecycle for pages.

### Responsibilities

- AppModel
  - Holds the canonical `KeyMap` for global bindings (quit, help, etc).
  - Handles top-level keys before delegation to the router/pages.
  - Responds to `PageChangeMsg` by asking a `pageFactory` to create the requested page and instructing the router to navigate to it.
  - Coordinates shutdown / cleanup (calls `router.Close()` and any app-level teardown).
- Router
  - Handles page lifecycle (Init, Update delegation, View, Close) and a lightweight `GlobalKeyMap` check for router-local needs.
  - Still checks global keys before delegating to the page, but AppModel is the authoritative owner of the KeyMap.
- PageFactory
  - A callback provided to `AppModel` (to avoid package cycles) that returns concrete `Page` implementations for a given `PageID`.
  - Keeps `tui` package decoupled from `tui/pages` concrete types.

### Why this fixes keymap problems

- Previously global keys could be scattered between main, router, and pages, or be missed if a focused text input consumed the key.
- With AppModel owning the `KeyMap` and checking keys first in `Update`, quitting and other global actions are guaranteed to be detected even when a page/widget is focused.
- The router still offers a second-level check (defensive), but AppModel is the primary guard — matching the Crush approach.

### Example flow

1. A user presses `q`.
2. Bubble Tea delivers a `tea.KeyMsg` to the top-level model (`AppModel`).
3. `AppModel.Update` checks key against `KeyMap.Quit`.
   - Match: AppModel calls `router.Close()` and returns `tea.Quit`.
   - No match: `AppModel` delegates to `router.Update(msg)`.
4. `router.Update(msg)` checks its own `GlobalKeyMap.CheckGlobalKeys` (redundant but safe).
5. If the `router` or a `page` decides navigation is required, it returns a `PageChangeMsg` (or a command that resolves to that msg).
6. `AppModel` handles `PageChangeMsg` by invoking `pageFactory(PageID)` and calling `router.NavigateTo(newPage, id)`.

### Example code references

- AppModel (handles the keymap and page factory)
```internal/tui/app_model.go#L1-240
// AppModel: top-level tea.Model that owns keyMap, router, pageFactory
// (See internal/tui/app_model.go for the full implementation.)
```

- Router (delegates to current page and offers a secondary global key check)
```internal/tui/router.go#L1-200
// Router.Update checks router-local global keys before delegating to page.Update
```

- main.go (wires services, router, AppModel, and provides a pageFactory)
```main.go#L1-200
// buildAppModel() constructs the Coordinator, services, router and returns
// an AppModel configured with a pageFactory that knows how to create pages
// on demand (login, server selection, top-level page).
```

### Implementation notes / guidelines

- Keep KeyMap definition in one file (`internal/tui/keys.go`) and create it once during startup via `DefaultKeyMap()`. Inject this into `AppModel` and the router during initialization (router.globalKeys should be set from AppModel to avoid drift).
- The `pageFactory` should capture the minimal dependencies required to construct pages (coordinator, services, config). It should return `nil` for unknown `PageID`s.
- `AppModel` should inspect immediate results from commands produced by the router (the existing pattern where the router returns a command that immediately yields a `QuitRequestedMsg`). If the command yields a `QuitRequestedMsg`, `AppModel` should perform coordinated shutdown.
- Pages should be self-contained `tea.Model` implementations and should not perform global quit handling — they can document "Press q to quit" in help text but rely on the AppModel for actual behavior.
- Keep `ctrl+c` as a secondary quit binding and advise users that some terminal environments may not deliver `ctrl+c` as a normal key event; `q` should be the primary.

### Image Renderer decisions

- The image renderer builds an exact-size pixel canvas for Kitty/iTerm2 to avoid fractional-scaling seams when placing images in terminal cell grids. The implementation uses constants for pixels-per-cell (Kitty: 10, iTerm2: 20) which are tunable in code; see `internal/image/renderer.go` for details.
- For debugging, the renderer exposes CLI flags (`-force-renderer`, `-render-debug`, `-dump-view`) so developers can reproduce terminal-specific rendering differences without environment variables.
- The renderer caches encoded PNGs and uses a content hash (sha256 of PNG bytes) as part of cache keys to avoid re-rendering the same content. For Kitty, a numeric image ID is derived from the content hash and used to transmit/place images via the Kitty protocol.

### Migration checklist (to convert existing code to AppModel + pageFactory)

1. Extract top-level state and key handling from `main.go` into `internal/tui/app_model.go` (the AppModel).
2. Create a `pageFactory` closure in `main` (or in a small builder) that knows how to construct pages with the required dependencies.
3. Construct the router with an initial page and set `router.globalKeys` from the `KeyMap` owned by AppModel.
4. Replace `tea.NewProgram(model)` with `tea.NewProgram(appModel)`.
5. Remove ad-hoc quit checks scattered in pages; centralize them in AppModel.Update.
6. Add unit tests for AppModel.Update ensuring:
   - `q` and `ctrl+c` produce `tea.Quit`.
   - `PageChangeMsg` causes `router.NavigateTo` with a page built by pageFactory.
7. Add integration tests to validate that a focused text input does not prevent global quit from being processed.

---

## 6. Remaining Work (TODO)

#### A. Convert Coordinator to Service-Based

Currently the Coordinator is a giant state container with direct field access. Refactor to:

```internal/app/coordinator.go#L1-200
type Coordinator struct {
    // Services (dependencies)
    auth     service.AuthServicer
    library  service.LibraryServicer
    playback service.PlaybackServicer
    
    // State (private)
    state    *domain.AppState
    
    // Event routing
    eventBus *pubsub.Broker[tea.Msg]
}
```

#### B. Create Page Components

Move view logic from `main.go` into page components:

```
internal/tui/pages/
├── login_page.go               # Login page
├── server_selection_page.go    # Server selection
├── library_page.go             # Library page (three-pane layout)
├── queue.go                    # Queue modal
└── playback.go                 # Playback controls
```

Each page implements:
```internal/tui/page.go#L1-40
type Page interface {
    tea.Model
    Close() // Cleanup when navigating away
}
```

#### C. Create App Orchestrator

Create `internal/app/app.go` to wire services and manage lifecycle:

```internal/app/app.go#L1-120
type App struct {
    coordinator *Coordinator
    services    Services
    program     *tea.Program
}
```

#### D. Migrate main.go

Move logic from monolithic `main.go` into:
1. Service initialization
2. App creation
3. Simple `main()` that wires and runs

Target:
```main.go#L1-120
func main() {
    cfg := config.Load()
    
    services := app.Services{
        Auth:     auth.NewService(),
        Library:  service.NewLibraryService(cfg.BaseURL, cfg.Token),
        Playback: playback.NewService(),
    }
    
    app := app.New(services)
    if err := app.Run(); err != nil {
        log.Fatal(err)
    }
}
```

#### E. Add Tests

With interfaces in place, add unit tests:
- Mock services for testing coordinator logic
- Test page components in isolation
- Test pubsub event flow

Example:
```internal/tui/app_model_test.go#L1-200
// Test AppModel receives key messages and exits; Test pageFactory-driven navigation
```

## Migration Strategy

**Phase 1: Foundation (DONE)**
- ✅ Create directory structure
- ✅ Add pubsub broker
- ✅ Define service interfaces
- ✅ Create base TUI components and styles

**Phase 2: Service Wiring (NEXT)**
- Refactor Coordinator to use service interfaces
- Move direct Plex client calls to services
- Add event publishing to services

**Phase 3: UI Components**
- Extract pages from main.go
- Implement page interface
- Use shared styles and components

**Phase 4: App Layer**
- Create App orchestrator
- Wire services and coordinator
- Route events to UI

**Phase 5: Main Simplification**
- Slim down main.go to initialization
- Move all business logic out
- Add graceful shutdown

> Note: It's acceptable for `main.go` to directly set initial service state (for example, calling `pbSvc.SetVolume(savedVolume)`) during startup. This is considered a one-time initialization; run-time playback actions should still go through the `Orchestrator` or other service interfaces for consistent behavior and event publishing.

**Phase 6: Testing**
- Add service mocks
- Test coordinator in isolation
- Test UI components
- Integration tests

## Benefits

1. **Separation of Concerns**: Clear boundaries between app, service, and UI layers
2. **Testability**: Interfaces enable mocking and isolated testing
3. **Reusability**: Components can be reused across pages
4. **Maintainability**: Small, focused files instead of monolithic main.go
5. **Event-Driven**: Decoupled communication via pubsub
6. **Type Safety**: Generic broker provides compile-time safety
7. **Consistency**: Follows established patterns from tmp/crush, including an `AppModel` that owns global input behavior and a `pageFactory` that safely instantiates pages

## Files Created

- `internal/pubsub/broker.go` - Generic event broker
- `internal/service/interfaces.go` - Service contracts
- `internal/tui/util/util.go` - TUI utilities and interfaces
- `internal/tui/components/statusbar.go` - Status bar component
- `internal/tui/styles/styles.go` - Centralized styles
- `internal/tui/app_model.go` - AppModel and pageFactory wiring (NEW)
- `internal/tui/pages/list_items.go` - List item adapters for bubbles/list (NEW)

## Next Steps

1. Refactor existing `LibraryService` to implement `LibraryServicer`
2. Create `AuthService` implementing `AuthServicer`
3. Create `PlaybackService` implementing `PlaybackServicer`
4. Refactor Coordinator to accept services
5. Extract first page component (login page)
6. Continue with remaining phases
7. Add unit tests for `AppModel` and `pageFactory` behavior (global key handling + navigation)

---

Generated: 2025-11-15
Status: Phase 1 Complete, Phase 2 Ready