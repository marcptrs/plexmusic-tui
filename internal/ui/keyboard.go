package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// KeyboardHandler encapsulates keyboard event handling logic
type KeyboardHandler struct {
	onEscape      func() (interface{}, tea.Cmd)
	onTab         func(reverse bool) (interface{}, tea.Cmd)
	onUpDown      func(down bool) (interface{}, tea.Cmd)
	onPlayPause   func() (interface{}, tea.Cmd)
	onStop        func() (interface{}, tea.Cmd)
	onNext        func() (interface{}, tea.Cmd)
	onPrevious    func() (interface{}, tea.Cmd)
	onEnter       func() (interface{}, tea.Cmd)
	usernameInput *textinput.Model
	passwordInput *textinput.Model
}

// NewKeyboardHandler creates a new keyboard handler
func NewKeyboardHandler(
	usernameInput *textinput.Model,
	passwordInput *textinput.Model,
) *KeyboardHandler {
	return &KeyboardHandler{
		usernameInput: usernameInput,
		passwordInput: passwordInput,
	}
}

// SetEscapeHandler sets the callback for escape key
func (k *KeyboardHandler) SetEscapeHandler(f func() (interface{}, tea.Cmd)) {
	k.onEscape = f
}

// SetTabHandler sets the callback for tab/shift+tab keys
func (k *KeyboardHandler) SetTabHandler(f func(reverse bool) (interface{}, tea.Cmd)) {
	k.onTab = f
}

// SetUpDownHandler sets the callback for up/down arrow keys
func (k *KeyboardHandler) SetUpDownHandler(f func(down bool) (interface{}, tea.Cmd)) {
	k.onUpDown = f
}

// SetPlayPauseHandler sets the callback for space/p keys
func (k *KeyboardHandler) SetPlayPauseHandler(f func() (interface{}, tea.Cmd)) {
	k.onPlayPause = f
}

// SetStopHandler sets the callback for 's' key
func (k *KeyboardHandler) SetStopHandler(f func() (interface{}, tea.Cmd)) {
	k.onStop = f
}

// SetNextHandler sets the callback for 'n' key
func (k *KeyboardHandler) SetNextHandler(f func() (interface{}, tea.Cmd)) {
	k.onNext = f
}

// SetPreviousHandler sets the callback for 'b' key
func (k *KeyboardHandler) SetPreviousHandler(f func() (interface{}, tea.Cmd)) {
	k.onPrevious = f
}

// SetEnterHandler sets the callback for enter key
func (k *KeyboardHandler) SetEnterHandler(f func() (interface{}, tea.Cmd)) {
	k.onEnter = f
}

// HandleKey processes a keyboard key event
// Returns (shouldConsume, model, cmd) where shouldConsume indicates if the key was handled
func (k *KeyboardHandler) HandleKey(keyStr string) (bool, interface{}, tea.Cmd) {
	switch keyStr {
	case "ctrl+c":
		// Let global key handling decide whether to quit.
		return false, nil, nil

	case "esc":
		if k.onEscape != nil {
			model, cmd := k.onEscape()
			return true, model, cmd
		}
		return false, nil, nil

	case "tab":
		if k.onTab != nil {
			model, cmd := k.onTab(false) // forward
			return true, model, cmd
		}
		return false, nil, nil

	case "shift+tab":
		if k.onTab != nil {
			model, cmd := k.onTab(true) // reverse
			return true, model, cmd
		}
		return false, nil, nil

	case "up":
		if k.onUpDown != nil {
			model, cmd := k.onUpDown(false) // up
			return true, model, cmd
		}
		return false, nil, nil

	case "down":
		if k.onUpDown != nil {
			model, cmd := k.onUpDown(true) // down
			return true, model, cmd
		}
		return false, nil, nil

	case " ", "p":
		// Space or 'p' for play/pause
		if k.onPlayPause != nil {
			model, cmd := k.onPlayPause()
			return true, model, cmd
		}
		return false, nil, nil

	case "s":
		// Stop playback
		if k.onStop != nil {
			model, cmd := k.onStop()
			return true, model, cmd
		}
		return false, nil, nil

	case "n":
		// Next track
		if k.onNext != nil {
			model, cmd := k.onNext()
			return true, model, cmd
		}
		return false, nil, nil

	case "b":
		// Previous track
		if k.onPrevious != nil {
			model, cmd := k.onPrevious()
			return true, model, cmd
		}
		return false, nil, nil

	case "enter":
		if k.onEnter != nil {
			model, cmd := k.onEnter()
			return true, model, cmd
		}
		return false, nil, nil
	}

	return false, nil, nil
}

// UpdateInputs delegates keyboard input to text input fields
// Returns command from text input update
func (k *KeyboardHandler) UpdateInputs(msg tea.Msg, focusIndex int) tea.Cmd {
	cmds := make([]tea.Cmd, 2)

	if focusIndex == 0 && k.usernameInput != nil {
		*k.usernameInput, cmds[0] = k.usernameInput.Update(msg)
	} else if focusIndex == 1 && k.passwordInput != nil {
		*k.passwordInput, cmds[1] = k.passwordInput.Update(msg)
	}

	return tea.Batch(cmds...)
}
