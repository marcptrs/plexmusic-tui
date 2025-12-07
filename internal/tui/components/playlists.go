package components

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/tui/styles"
)

type PlaylistsComponent struct {
	coordinator app.Coordinatorer
	list        list.Model
	focused     bool
}

func NewPlaylistsComponent(coord app.Coordinatorer) *PlaylistsComponent {
	delegate := styles.NewCustomDelegate()
	l := list.New(nil, delegate, 20, 10)
	l.Title = "Playlists"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings() // Disable q/Q quit keys - use ctrl+c for quit

	return &PlaylistsComponent{
		coordinator: coord,
		list:        l,
	}
}

func (c *PlaylistsComponent) Init() tea.Cmd {
	return nil
}

func (c *PlaylistsComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

func (c *PlaylistsComponent) View() string {
	return c.list.View()
}

func (c *PlaylistsComponent) SetSize(width, height int) {
	c.list.SetSize(width, height)
}

func (c *PlaylistsComponent) SetItems(items []list.Item) {
	c.list.SetItems(items)
}

func (c *PlaylistsComponent) Select(index int) {
	c.list.Select(index)
}

func (c *PlaylistsComponent) Index() int {
	return c.list.Index()
}

func (c *PlaylistsComponent) Items() []list.Item {
	return c.list.Items()
}

func (c *PlaylistsComponent) SelectedItem() list.Item {
	return c.list.SelectedItem()
}
