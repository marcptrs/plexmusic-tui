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

// MainAppKeyMap defines key bindings for the main application page.
type MainAppKeyMap struct {
	Up              key.Binding
	Down            key.Binding
	Enter           key.Binding
	Play            key.Binding
	Next            key.Binding
	Prev            key.Binding
	VolumeUp        key.Binding
	VolumeDown      key.Binding
	Queue           key.Binding
	Refresh         key.Binding
	Search          key.Binding
	Settings        key.Binding
	SwitchView      key.Binding
	FocusNowPlaying key.Binding
	Back            key.Binding
	Quit            key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k MainAppKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Play, k.Back, k.Quit}
}

// FullHelp returns keybindings for the expanded help view.
func (k MainAppKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Back},
		{k.Play, k.Next, k.Prev, k.VolumeUp, k.VolumeDown, k.FocusNowPlaying},
		{k.Queue, k.Refresh, k.Search, k.Settings, k.SwitchView, k.Quit},
	}
}

// DefaultMainAppKeyMap returns the default key bindings for the main app.
func DefaultMainAppKeyMap() MainAppKeyMap {
	return MainAppKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Play: key.NewBinding(
			key.WithKeys(" ", "p"),
			key.WithHelp("space/p", "play"),
		),
		Next: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next"),
		),
		Prev: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "prev"),
		),
		VolumeUp: key.NewBinding(
			key.WithKeys("+", "="),
			key.WithHelp("+", "vol up"),
		),
		VolumeDown: key.NewBinding(
			key.WithKeys("-"),
			key.WithHelp("-", "vol down"),
		),
		Queue: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "queue"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Search: key.NewBinding(
			key.WithKeys("s", "/"),
			key.WithHelp("s", "search"),
		),
		Settings: key.NewBinding(
			key.WithKeys(","),
			key.WithHelp(",", "settings"),
		),
		SwitchView: key.NewBinding(
			key.WithKeys("ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5", "ctrl+6"),
			key.WithHelp("ctrl+1-6", "switch view"),
		),
		FocusNowPlaying: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "focus now playing"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}
}

// LoginKeyMap defines key bindings for the login page.
type LoginKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Quit  key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k LoginKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Quit}
}

// FullHelp returns keybindings for the expanded help view.
func (k LoginKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Quit},
	}
}

// DefaultLoginKeyMap returns the default key bindings for the login page.
func DefaultLoginKeyMap() LoginKeyMap {
	return LoginKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "sign in"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ServerSelectionKeyMap defines key bindings for the server selection page.
type ServerSelectionKeyMap struct {
	Select key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k ServerSelectionKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select}
}

// FullHelp returns keybindings for the expanded help view.
func (k ServerSelectionKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Select},
	}
}
