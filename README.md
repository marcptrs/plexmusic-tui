# plexmusic-tui

> ⚠️ **Work in Progress**: This project is under active development. Features and functionality may change.

A terminal user interface (TUI) for browsing and playing music from your Plex Media Server.

## Features

### Current Features

- **Authentication**: Login with your Plex account credentials
- **Server Selection**: Automatically discover and connect to Plex Media Servers
- **Music Browsing**:
  - Recently Added albums
  - Playlists (browse and play)
  - Album track listings
- **Playback Controls**:
  - Play, pause, and stop playback
  - Next/previous track navigation
  - Volume control
  - Real-time playback position tracking
- **Album Art Display**: Supports multiple terminal protocols:
  - Kitty graphics protocol
  - iTerm2 inline images
  - Sixel graphics
  - Unicode half-blocks (fallback)
- **Multi-pane Interface**: Navigate between menu, content, and details
- **Format Support**: MP3, FLAC, WAV, Vorbis/OGG audio formats
- **Smart TLS Handling**: Automatically handles self-signed certificates for local servers

### Planned Features

- Search functionality
- Settings customization
- Queue management
- More playback controls

## Development Environment Setup

### Prerequisites

- **Go**: Version 1.24.0 or higher
- **Operating System**: Linux (primary support), macOS, Windows
- **Audio**: Working audio output device

### Dependencies

The project uses the following main dependencies:

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Style definitions
- [Beep](https://github.com/faiface/beep) - Audio playback library
- [Imaging](https://github.com/disintegration/imaging) - Image processing

All dependencies are managed via Go modules and will be automatically downloaded.

UI / Styles: The canonical TUI styles are provided in `internal/tui/styles/styles.go`. This package is the single source of truth for TUI styling (color palette, named lipgloss styles such as `TitleStyle`, `FocusedStyle`, `BlurredStyle`, `PaneStyle`, and helpers like `PrimaryTextStyle`). The `internal/ui` package focuses on view/layout helpers (e.g., `GetContentPaneWidth`, `FormatTrackDuration`) and does not provide canonical style tokens. When adding or refactoring UI code, import `internal/tui/styles` for styles and `internal/ui` for view helpers.

### Installation

1. **Clone the repository**:
   ```bash
   git clone https://github.com/marcptrs/plexmusic-tui.git
   cd plexmusic-tui
   ```

2. **Download dependencies**:
   ```bash
   go mod download
   ```

3. **Build the project**:
   ```bash
   go build -o plexmusic-tui
   ```

4. **Run the application**:
   ```bash
   ./plexmusic-tui
   ```

## Build Commands

### Development Build

Build for local testing:
```bash
go build -o plexmusic-tui
```

### Release Build

Build with optimizations:
```bash
go build -ldflags="-s -w" -o plexmusic-tui
```

### Cross-compilation

Build for different platforms:

**Linux (AMD64)**:
```bash
GOOS=linux GOARCH=amd64 go build -o plexmusic-tui-linux-amd64
```

**macOS (ARM64 - M1/M2)**:
```bash
GOOS=darwin GOARCH=arm64 go build -o plexmusic-tui-darwin-arm64
```

**macOS (AMD64 - Intel)**:
```bash
GOOS=darwin GOARCH=amd64 go build -o plexmusic-tui-darwin-amd64
```

**Windows (AMD64)**:
```bash
GOOS=windows GOARCH=amd64 go build -o plexmusic-tui.exe
```

### Testing

Run tests (when available):
```bash
go test ./...
```

### Cleaning Build Artifacts

```bash
go clean
rm -f plexmusic-tui
```

## Usage

1. **Launch the application**:
   ```bash
   ./plexmusic-tui
   ```

2. **Login**: Enter your Plex account credentials

3. **Select Server**: Choose your Plex Media Server from the list

4. **Navigate**:
   - `Tab`/`Shift+Tab`: Switch between panes
   - `Up`/`Down`: Navigate within panes
   - `Enter`: Select/activate item
   - `Esc`: Go back

5. **Playback Controls**:
   - `Space` or `p`: Play/pause
   - `s`: Stop
   - `n`: Next track
   - `b`: Previous track

6. **Exit**: Press `Ctrl+C` to quit

## Configuration

Configuration is stored in `~/.config/plexmusic-tui/config.json`:

```json
{
  "authToken": "your-plex-token",
  "lastSelectedServer": "hostname/server-name"
}
```

The application automatically saves your authentication token and remembers your last selected server using a canonical host/name key (e.g., hostname/server-name).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgments

- Built with [Charm](https://charm.sh/) TUI libraries
- Audio playback powered by [Beep](https://github.com/faiface/beep)
- Inspired by the need for a lightweight Plex music client
