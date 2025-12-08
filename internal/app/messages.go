package app

import (
	"time"

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

// Tea Command Helpers

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
