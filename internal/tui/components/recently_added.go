package components

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/tui/styles"
)

type RecentlyAddedComponent struct {
	coordinator app.Coordinatorer
	list        list.Model
	focused     bool
}

func NewRecentlyAddedComponent(coord app.Coordinatorer) *RecentlyAddedComponent {
	delegate := styles.NewCustomDelegate()
	l := list.New(nil, delegate, 20, 10)
	l.Title = "" // Title is rendered by the page, not the component
	l.SetShowHelp(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings() // Disable q/Q quit keys - use ctrl+c for quit

	return &RecentlyAddedComponent{
		coordinator: coord,
		list:        l,
	}
}

func (c *RecentlyAddedComponent) Init() tea.Cmd {
	return nil
}

func (c *RecentlyAddedComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

func (c *RecentlyAddedComponent) View() string {
	return c.list.View()
}

func (c *RecentlyAddedComponent) SetSize(width, height int) {
	c.list.SetSize(width, height)
}

func (c *RecentlyAddedComponent) SetItems(items []list.Item) {
	c.list.SetItems(items)
}

func (c *RecentlyAddedComponent) Select(index int) {
	c.list.Select(index)
}

func (c *RecentlyAddedComponent) Index() int {
	return c.list.Index()
}

func (c *RecentlyAddedComponent) Items() []list.Item {
	return c.list.Items()
}

func (c *RecentlyAddedComponent) SelectedItem() list.Item {
	return c.list.SelectedItem()
}

func (c *RecentlyAddedComponent) SetFocused(focused bool) {
	c.focused = focused
}

func (c *RecentlyAddedComponent) IsFocused() bool {
	return c.focused
}
