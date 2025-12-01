package util

import (
	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/pubsub"
)

// SubscribeToChannel creates a command that waits for a single event from a pubsub channel.
// This follows the Bubble Tea best practice of returning ONE message per command execution.
// The command should be re-issued in Update() to continue receiving events.
//
// Example usage:
//
//	func (p *Page) Init() tea.Cmd {
//	    return util.SubscribeToChannel(p.eventCh)
//	}
//
//	func (p *Page) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
//	    switch msg := msg.(type) {
//	    case domain.SomeEvent:
//	        // Handle event...
//	        return p, util.SubscribeToChannel(p.eventCh)  // Re-subscribe
//	    }
//	}
func SubscribeToChannel[T any](ch <-chan pubsub.Event[T]) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil // Channel closed
		}
		return ev.Payload
	}
}
