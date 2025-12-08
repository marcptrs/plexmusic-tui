package components

import tea "github.com/charmbracelet/bubbletea"

// Component is the base interface that all TUI components must implement.
// This follows the Bubble Tea model interface pattern.
type Component interface {
	// Init initializes the component and returns an initial command
	Init() tea.Cmd

	// Update processes a message and returns the updated component and command
	Update(msg tea.Msg) (tea.Model, tea.Cmd)

	// View renders the component to a string
	View() string
}

// Sizeable represents a component that can be resized
type Sizeable interface {
	SetSize(width, height int)
}

// Focusable represents a component that can receive and lose focus
type Focusable interface {
	// SetFocused sets the focus state of the component
	SetFocused(focused bool)

	// IsFocused returns whether the component is currently focused
	IsFocused() bool
}

// ListComponent extends Component with list-specific functionality
type ListComponent interface {
	Component
	Sizeable
	Focusable

	// Index returns the currently selected index
	Index() int

	// Select selects an item by index
	Select(index int)
}
