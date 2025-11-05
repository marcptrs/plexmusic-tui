# PlexMusic TUI - Project Overview

## Project Purpose
A terminal user interface (TUI) for browsing and playing music from Plex Media Servers. Built with Go using Bubble Tea framework.

## Current Architecture Issue
- **Single monolithic file**: `main.go` contains ~3000+ lines
- **Mixed concerns**: Authentication, API calls, UI rendering, playback, config management all in one file
- **No clear separation**: No distinction between layers (domain, application, infrastructure, presentation)

## Tech Stack
- **Language**: Go 1.24.0+
- **UI Framework**: Bubble Tea (bubbletea), Bubbles, Lipgloss
- **Audio**: Beep (supports MP3, FLAC, WAV, Vorbis/OGG)
- **Image Processing**: Imaging (disintegration)

## Key Functionalities to Separate
1. **Authentication**: Plex login and token management
2. **API Integration**: Server discovery, library/album/playlist/track fetching
3. **Playback**: Audio streaming and control (play, pause, next, previous, stop)
4. **UI Rendering**: Multiple views (login, server selection, home, recently added, playlists, main app)
5. **Configuration**: Config file loading/saving
6. **Image Handling**: Album art rendering with multiple protocols (Kitty, iTerm2, Sixel, Unicode)

## Code Conventions (Go)
- Standard Go conventions
- Method receivers on model struct
- Exported types (capitalized)
- Tea message types as custom types (authResult, serverListResult, etc.)
