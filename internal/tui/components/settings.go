package components

import (
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/tui/styles"
	"plexmusic-tui/internal/tui/util"
)

// SettingsComponent handles the settings list and configuration updates.
type SettingsComponent struct {
	coordinator app.Coordinatorer
	list        list.Model
	width       int
	height      int
}

// NewSettingsComponent creates a new SettingsComponent.
func NewSettingsComponent(coord app.Coordinatorer) *SettingsComponent {
	delegate := styles.NewCustomDelegate()
	l := list.New(nil, delegate, 20, 10)
	l.Title = "Settings"
	l.SetShowHelp(false)
	l.SetShowTitle(false) // We render our own title
	l.SetShowStatusBar(false)

	s := &SettingsComponent{
		coordinator: coord,
		list:        l,
	}
	s.RefreshItems()
	return s
}

// Init initializes the component.
func (s *SettingsComponent) Init() tea.Cmd {
	return nil
}

// Update handles messages for the settings component.
func (s *SettingsComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle shortcuts
		switch msg.String() {
		case "c":
			if s.coordinator != nil && s.coordinator.ConfigManager() != nil {
				cur := s.coordinator.ConfigManager().GetCoverArtPosition()
				newVal := "right"
				if cur == "right" {
					newVal = "left"
				}
				s.coordinator.ConfigManager().SetCoverArtPosition(newVal)
				_ = s.coordinator.ConfigManager().Save()
				s.RefreshItems()
				s.coordinator.SetNotification("Cover art position updated", "success", 3*time.Second)
			}
			return s, nil
		case "enter":
			if item, ok := s.list.SelectedItem().(util.SettingsItem); ok {
				switch item.Key {
				case "coverArtPos":
					// Toggle left/right choice
					cur := "left"
					if s.coordinator != nil && s.coordinator.ConfigManager() != nil {
						cur = s.coordinator.ConfigManager().GetCoverArtPosition()
					}
					newVal := "left"
					if cur == "left" {
						newVal = "right"
					}
					if s.coordinator != nil && s.coordinator.ConfigManager() != nil {
						s.coordinator.ConfigManager().SetCoverArtPosition(newVal)
						_ = s.coordinator.ConfigManager().Save()
					}
					s.RefreshItems()
					s.coordinator.SetNotification("Cover art position updated", "success", 2*time.Second)
				}
			}
			return s, nil
		}
	}

	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// View renders the settings component.
func (s *SettingsComponent) View() string {
	title := styles.TitleStyle.Render("Settings")
	return lipgloss.JoinVertical(lipgloss.Left, title, "", s.list.View())
}

// SetSize sets the component's size.
func (s *SettingsComponent) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.list.SetSize(w, h)
}

// RefreshItems reloads settings from the configuration.
func (s *SettingsComponent) RefreshItems() {
	if s.coordinator != nil && s.coordinator.ConfigManager() != nil {
		pos := s.coordinator.ConfigManager().GetCoverArtPosition()
		items := []list.Item{}
		items = append(items, util.SettingsItem{
			Group:           "Layout",
			Name:            "Cover art position",
			Key:             "coverArtPos",
			Kind:            "choice",
			Value:           pos,
			DescriptionText: "Position of cover art within the layout: left or right (Press 'c' to toggle)",
		})
		s.list.SetItems(items)
	}
}

// Items returns the current list items.
func (s *SettingsComponent) Items() []list.Item {
	return s.list.Items()
}

// SelectedItem returns the currently selected item.
func (s *SettingsComponent) SelectedItem() list.Item {
	return s.list.SelectedItem()
}

// Select sets the selected index of the list.
func (s *SettingsComponent) Select(index int) {
	s.list.Select(index)
}
