package components

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"plexmusic-tui/internal/app"
)

type PlaylistsComponent struct {
	*BaseListComponent
}

func NewPlaylistsComponent(ctx *app.AppContext) *PlaylistsComponent {
	return &PlaylistsComponent{
		BaseListComponent: NewBaseListComponent(ctx, "Playlists"),
	}
}

func (c *PlaylistsComponent) SetItems(items []list.Item) {
	c.BaseListComponent.SetItems(items)
}

func (c *PlaylistsComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := c.BaseListComponent.Update(msg)
	c.BaseListComponent = model.(*BaseListComponent)
	return c, cmd
}
