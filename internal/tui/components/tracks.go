package components

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
)

type TracksComponent struct {
	*BaseListComponent
}

func NewTracksComponent(ctx *app.AppContext) *TracksComponent {
	return &TracksComponent{
		BaseListComponent: NewBaseListComponent(ctx, "Tracks"),
	}
}

func (c *TracksComponent) SetItems(items []list.Item) {
	c.BaseListComponent.SetItems(items)
}

func (c *TracksComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := c.BaseListComponent.Update(msg)
	c.BaseListComponent = model.(*BaseListComponent)
	return c, cmd
}
