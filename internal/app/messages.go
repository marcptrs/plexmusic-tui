package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faiface/beep"
)

// Message types for the coordinator
type MessageType int

const (
	MessageSetServers MessageType = iota
	MessageSetLibraries
	MessageSetAlbums
	MessageSetTracks
	MessageSetPlaylists
	MessagePlayTrack
	MessagePlayPause
	MessageStop
	MessageVolumeUp
	MessageVolumeDown
	MessageQueueTrack
	MessageNextTab
	MessagePreviousTab
	MessageSelectAlbum
	MessageSelectTrack
)

// CoordinatorMsg wraps messages for the coordinator
type CoordinatorMsg struct {
	Type      MessageType
	Timestamp time.Time
	Data      interface{}
}

// Dispatch processes a coordinator message and updates state
// Returns true if the message was handled
func (c *Coordinator) Dispatch(msg CoordinatorMsg) bool {
	switch msg.Type {
	case MessageSetServers:
		if servers, ok := msg.Data.([]PlexServer); ok {
			c.SetServers(servers)
			return true
		}

	case MessageSetLibraries:
		if libs, ok := msg.Data.([]MusicLibrary); ok {
			c.SetLibraries(libs)
			return true
		}

	case MessageSetAlbums:
		if albums, ok := msg.Data.([]Album); ok {
			c.SetAlbums(albums)
			return true
		}

	case MessageSetTracks:
		if tracks, ok := msg.Data.([]Track); ok {
			c.SetTracks(tracks)
			return true
		}

	case MessageSetPlaylists:
		if playlists, ok := msg.Data.([]Playlist); ok {
			c.SetPlaylists(playlists)
			return true
		}

	case MessageQueueTrack:
		if track, ok := msg.Data.(Track); ok {
			// convert app.Track -> domain.Track then set via coordinator API
			q := c.Queue()
			q = append(q, track)
			c.SetQueue(q)
			return true
		}

	case MessageNextTab:
		c.NextTab()
		return true

	case MessagePreviousTab:
		c.PreviousTab()
		return true

	case MessageSelectAlbum:
		if idx, ok := msg.Data.(int); ok {
			c.SetSelectedAlbum(idx)
			return true
		}

	case MessageSelectTrack:
		if idx, ok := msg.Data.(int); ok {
			c.SetSelectedTrack(idx)
			return true
		}
	}

	return false
}

// Tea Command Helpers

// FetchServersCmd returns a Bubble Tea command for fetching servers
func (c *Coordinator) FetchServersCmd(fn func() ([]PlexServer, error)) tea.Cmd {
	return func() tea.Msg {
		servers, err := fn()
		if err != nil {
			c.SetError(err)
			return nil
		}
		return CoordinatorMsg{
			Type:      MessageSetServers,
			Timestamp: time.Now(),
			Data:      servers,
		}
	}
}

// FetchLibrariesCmd returns a Bubble Tea command for fetching libraries
func (c *Coordinator) FetchLibrariesCmd(fn func() ([]MusicLibrary, error)) tea.Cmd {
	return func() tea.Msg {
		libs, err := fn()
		if err != nil {
			c.SetError(err)
			return nil
		}
		return CoordinatorMsg{
			Type:      MessageSetLibraries,
			Timestamp: time.Now(),
			Data:      libs,
		}
	}
}

// FetchAlbumsCmd returns a Bubble Tea command for fetching albums
func (c *Coordinator) FetchAlbumsCmd(fn func() ([]Album, error)) tea.Cmd {
	return func() tea.Msg {
		albums, err := fn()
		if err != nil {
			c.SetError(err)
			return nil
		}
		return CoordinatorMsg{
			Type:      MessageSetAlbums,
			Timestamp: time.Now(),
			Data:      albums,
		}
	}
}

// FetchTracksCmd returns a Bubble Tea command for fetching tracks
func (c *Coordinator) FetchTracksCmd(fn func() ([]Track, error)) tea.Cmd {
	return func() tea.Msg {
		tracks, err := fn()
		if err != nil {
			c.SetError(err)
			return nil
		}
		return CoordinatorMsg{
			Type:      MessageSetTracks,
			Timestamp: time.Now(),
			Data:      tracks,
		}
	}
}

// FetchPlaylistsCmd returns a Bubble Tea command for fetching playlists
func (c *Coordinator) FetchPlaylistsCmd(fn func() ([]Playlist, error)) tea.Cmd {
	return func() tea.Msg {
		playlists, err := fn()
		if err != nil {
			c.SetError(err)
			return nil
		}
		return CoordinatorMsg{
			Type:      MessageSetPlaylists,
			Timestamp: time.Now(),
			Data:      playlists,
		}
	}
}

// RequestStreamCmd returns a Bubble Tea command that fetches a stream
type StreamResult struct {
	Streamer beep.StreamSeekCloser
	Format   beep.Format
	Err      error
}

// StreamResultMsg wraps stream result for tea update
type StreamResultMsg struct {
	Result StreamResult
}

// FetchStreamCmd returns a command that fetches an audio stream
func (c *Coordinator) FetchStreamCmd(
	fn func() (beep.StreamSeekCloser, beep.Format, error),
) tea.Cmd {
	return func() tea.Msg {
		streamer, format, err := fn()
		return StreamResultMsg{
			Result: StreamResult{
				Streamer: streamer,
				Format:   format,
				Err:      err,
			},
		}
	}
}
