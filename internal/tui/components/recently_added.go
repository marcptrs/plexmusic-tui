package components

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
)

type RecentlyAddedComponent struct {
	*BaseListComponent
}

func NewRecentlyAddedComponent(ctx *app.AppContext) *RecentlyAddedComponent {
	base := NewBaseListComponent(ctx, "")
	base.List().SetShowTitle(false)

	return &RecentlyAddedComponent{
		BaseListComponent: base,
	}
}

func (c *RecentlyAddedComponent) SetItems(items []list.Item) {
	c.BaseListComponent.SetItems(items)
}

func (c *RecentlyAddedComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := c.BaseListComponent.Update(msg)
	c.BaseListComponent = model.(*BaseListComponent)
	return c, cmd
}
