package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/tui/util"
)

// StatusBar displays status messages and errors
type StatusBar struct {
	width        int
	currentMsg   string
	msgType      util.InfoType
	successStyle lipgloss.Style
	errorStyle   lipgloss.Style
	warningStyle lipgloss.Style
	infoStyle    lipgloss.Style
}

// NewStatusBar creates a new status bar
func NewStatusBar() *StatusBar {
	return &StatusBar{
		successStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true),
		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true),
		warningStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true),
		infoStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true),
	}
}

// Update handles messages
func (s *StatusBar) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case util.InfoMsg:
		s.currentMsg = msg.Msg
		s.msgType = msg.Type
	}
	return nil
}

// View renders the status bar
func (s *StatusBar) View() string {
	if s.currentMsg == "" {
		return ""
	}

	var style lipgloss.Style
	var prefix string

	switch s.msgType {
	case util.InfoTypeSuccess:
		style = s.successStyle
		prefix = "✓ "
	case util.InfoTypeError:
		style = s.errorStyle
		prefix = "✗ "
	case util.InfoTypeWarning:
		style = s.warningStyle
		prefix = "⚠ "
	case util.InfoTypeInfo:
		style = s.infoStyle
		prefix = "ℹ "
	}

	msg := prefix + s.currentMsg
	if len(msg) > s.width-2 {
		msg = msg[:s.width-5] + "..."
	}

	return style.Render(msg)
}

// SetSize updates the status bar width
func (s *StatusBar) SetSize(width, height int) tea.Cmd {
	s.width = width
	return nil
}

// GetSize returns the current size
func (s *StatusBar) GetSize() (int, int) {
	return s.width, 1
}

// Clear clears the current message
func (s *StatusBar) Clear() {
	s.currentMsg = ""
}

// SetMessage sets a message with the given type
func (s *StatusBar) SetMessage(msg string, msgType util.InfoType) {
	s.currentMsg = msg
	s.msgType = msgType
}

// FormatDuration formats milliseconds into MM:SS
func FormatDuration(ms int) string {
	seconds := ms / 1000
	minutes := seconds / 60
	seconds = seconds % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// TruncateString truncates a string to fit within maxWidth
func TruncateString(s string, maxWidth int) string {
	if len(s) <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
	}
	return s[:maxWidth-3] + "..."
}
