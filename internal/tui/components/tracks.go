package components

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
)

type TracksComponent struct {
	coordinator app.Coordinatorer
	list        list.Model
	focused     bool
}

func NewTracksComponent(coord app.Coordinatorer) *TracksComponent {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 20, 10)
	l.Title = "Tracks"
	l.SetShowHelp(false)

	return &TracksComponent{
		coordinator: coord,
		list:        l,
	}
}

func (c *TracksComponent) Init() tea.Cmd {
	return nil
}

func (c *TracksComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

func (c *TracksComponent) View() string {
	return c.list.View()
}

func (c *TracksComponent) SetSize(width, height int) {
	c.list.SetSize(width, height)
}

func (c *TracksComponent) SetWidth(width int) {
	c.list.SetWidth(width)
}

func (c *TracksComponent) SetItems(items []list.Item) {
	c.list.SetItems(items)
}

func (c *TracksComponent) Select(index int) {
	c.list.Select(index)
}

func (c *TracksComponent) Index() int {
	return c.list.Index()
}

func (c *TracksComponent) Items() []list.Item {
	return c.list.Items()
}

func (c *TracksComponent) SelectedItem() list.Item {
	return c.list.SelectedItem()
}
