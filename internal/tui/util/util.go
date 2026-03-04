package util

import (
	tea "charm.land/bubbletea/v2"
)

// Model is the standard Bubble Tea model interface
type Model interface {
	Init() tea.Cmd
	Update(tea.Msg) (tea.Model, tea.Cmd)
	View() tea.View
}

// Sizeable interface for components that can be resized
type Sizeable interface {
	SetSize(width, height int) tea.Cmd
	GetSize() (int, int)
}

// Focusable interface for components that can receive focus
type Focusable interface {
	Focus() tea.Cmd
	Blur() tea.Cmd
	IsFocused() bool
}

// InfoType represents the type of info message
type InfoType int

const (
	InfoTypeSuccess InfoType = iota
	InfoTypeError
	InfoTypeWarning
	InfoTypeInfo
)

// InfoMsg represents an informational message to display
type InfoMsg struct {
	Type InfoType
	Msg  string
}

// ReportError creates a command that reports an error to the TUI and logs it using charm log
func ReportError(err error) tea.Cmd {
	// TODO: Add logging
	return CmdHandler(InfoMsg{Type: InfoTypeError, Msg: err.Error()})
}

// ReportSuccess creates a command that reports a success message
func ReportSuccess(msg string) tea.Cmd {
	return CmdHandler(InfoMsg{Type: InfoTypeSuccess, Msg: msg})
}

// ReportInfo creates a command that reports an info message
func ReportInfo(msg string) tea.Cmd {
	return CmdHandler(InfoMsg{Type: InfoTypeInfo, Msg: msg})
}

// CmdHandler wraps a message as a tea.Cmd
func CmdHandler(msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return msg
	}
}
