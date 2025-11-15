package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines global key bindings for the TUI.
//
// Semantics:
//   - Quit: ctrl+c
//   - Esc: reserved for contextual back/cancel/close behavior and
//     should NOT be used as a hard quit key.
type KeyMap struct {
	// Quit binding that should exit the application from any page.
	Quit key.Binding

	pageBindings []key.Binding
}

// DefaultKeyMap returns the default global key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}
}
