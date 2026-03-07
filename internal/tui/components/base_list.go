package components

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/tui/styles"
)

// BaseListComponent provides common functionality for list-based components.
// It wraps the bubbles/list component and provides a standard interface
// for managing list state, focus, and sizing.
//
// Components can embed this struct to inherit common list behavior,
// reducing code duplication across similar components.
type BaseListComponent struct {
	ctx     *app.AppContext
	list    list.Model
	focused bool
	title   string
}

// NewBaseListComponent creates a new base list component with standard configuration
func NewBaseListComponent(ctx *app.AppContext, title string) *BaseListComponent {
	delegate := styles.NewDynamicDelegate()

	l := list.New(nil, delegate, 20, 10)
	l.Title = title
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings() // Disable q/Q quit keys - use ctrl+c for quit

	return &BaseListComponent{
		ctx:  ctx,
		list: l,
	}
}

// Init initializes the component
func (b *BaseListComponent) Init() tea.Cmd {
	return nil
}

// Update processes messages and updates the list
func (b *BaseListComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Only process key messages when focused
	if _, ok := msg.(tea.KeyPressMsg); ok && !b.focused {
		return b, nil
	}

	var cmd tea.Cmd
	b.list, cmd = b.list.Update(msg)
	return b, cmd
}

// View renders the list
func (b *BaseListComponent) View() tea.View {
	// Get the list view but hide the built-in title
	b.list.SetShowTitle(false)
	listContent := b.list.View()

	// Create our own theme-aware title
	theme := styles.CurrentTheme()
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.TextColor)).
		Bold(true).
		Padding(0, 1).
		Background(lipgloss.Color(theme.BackgroundColor))

	// Use the list's title directly
	styledTitle := titleStyle.Render(b.list.Title)

	// Combine our custom title with the list content
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, styledTitle, listContent))
}

// SetSize sets the dimensions of the list
func (b *BaseListComponent) SetSize(width, height int) {
	b.list.SetSize(width, height)
}

// SetWidth sets the width of the list
func (b *BaseListComponent) SetWidth(width int) {
	b.list.SetWidth(width)
}

// SetHeight sets the height of the list
func (b *BaseListComponent) SetHeight(height int) {
	b.list.SetHeight(height)
}

// SetFocused sets the focus state
func (b *BaseListComponent) SetFocused(focused bool) {
	b.focused = focused
}

// IsFocused returns whether the component is focused
func (b *BaseListComponent) IsFocused() bool {
	return b.focused
}

// Index returns the currently selected index
func (b *BaseListComponent) Index() int {
	return b.list.Index()
}

// Select selects an item by index
func (b *BaseListComponent) Select(index int) {
	b.list.Select(index)
}

// SetItems sets the list items
func (b *BaseListComponent) SetItems(items []list.Item) {
	b.list.SetItems(items)
}

// Items returns the current list items
func (b *BaseListComponent) Items() []list.Item {
	return b.list.Items()
}

// SelectedItem returns the currently selected item
func (b *BaseListComponent) SelectedItem() list.Item {
	return b.list.SelectedItem()
}

// SetTitle sets the list title
func (b *BaseListComponent) SetTitle(title string) {
	b.title = title
	b.list.Title = title
}

// Title returns the list title
func (b *BaseListComponent) Title() string {
	return b.title
}

// Context returns the app context instance
func (b *BaseListComponent) Context() *app.AppContext {
	return b.ctx
}

// List returns the underlying bubbles list model for advanced usage
func (b *BaseListComponent) List() *list.Model {
	return &b.list
}
