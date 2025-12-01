package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/tui/styles"
)

// HomeItem represents a selectable item in the home view
type HomeItem struct {
	Title    string
	Subtitle string
	Key      string
	Type     string // "station", "album", "playlist", "artist"
}

// HomeComponent displays a scrollable home view with hubs and recently added
type HomeComponent struct {
	coordinator app.Coordinatorer
	viewport    viewport.Model
	items       []HomeItem
	selectedIdx int
	width       int
	height      int
	focused     bool
}

// NewHomeComponent creates a new scrollable home component
func NewHomeComponent(coord app.Coordinatorer) *HomeComponent {
	vp := viewport.New(80, 20)

	return &HomeComponent{
		coordinator: coord,
		viewport:    vp,
		items:       []HomeItem{},
		selectedIdx: 0,
	}
}

func (c *HomeComponent) Init() tea.Cmd {
	return nil
}

func (c *HomeComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !c.focused {
		return c, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if c.selectedIdx > 0 {
				c.selectedIdx--
				c.updateContent()
				c.ensureVisible()
			}
			return c, nil
		case "down", "j":
			if c.selectedIdx < len(c.items)-1 {
				c.selectedIdx++
				c.updateContent()
				c.ensureVisible()
			}
			return c, nil
		case "pgup", "pgdown", "home", "end":
			// Only pass scroll keys to viewport, not other keys like 'q'
			var cmd tea.Cmd
			c.viewport, cmd = c.viewport.Update(msg)
			return c, cmd
		}
		// Let other keys (like 'q' for quit) bubble up
		return c, nil
	}

	// Handle non-key messages (like window resize)
	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)
	return c, cmd
}

func (c *HomeComponent) View() string {
	return c.viewport.View()
}

func (c *HomeComponent) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.viewport.Width = width
	c.viewport.Height = height
	c.updateContent()
}

func (c *HomeComponent) SetFocused(focused bool) {
	c.focused = focused
}

func (c *HomeComponent) IsFocused() bool {
	return c.focused
}

func (c *HomeComponent) SelectedItem() *HomeItem {
	if c.selectedIdx >= 0 && c.selectedIdx < len(c.items) {
		return &c.items[c.selectedIdx]
	}
	return nil
}

func (c *HomeComponent) SelectedIndex() int {
	return c.selectedIdx
}

func (c *HomeComponent) Items() []HomeItem {
	return c.items
}

// RefreshFromCoordinator rebuilds the item list from coordinator data
func (c *HomeComponent) RefreshFromCoordinator() {
	c.items = []HomeItem{}

	// Add recently played artists FIRST (matching Plex dashboard order)
	recentArtists := c.coordinator.RecentlyPlayedArtists()
	for _, artist := range recentArtists {
		c.items = append(c.items, HomeItem{
			Title:    artist.Name,
			Subtitle: "Recently Played Artist",
			Key:      artist.Key,
			Type:     "artist",
		})
	}

	// Get hubs once for all sections below
	hubs := c.coordinator.LibraryHubs()

	// Add recently added albums SECOND - from hubs
	for _, hub := range hubs {
		if strings.Contains(strings.ToLower(hub.Context), "recent.added") && len(hub.Albums) > 0 {
			for _, a := range hub.Albums {
				c.items = append(c.items, HomeItem{
					Title:    a.Title,
					Subtitle: fmt.Sprintf("%s (%d) • Recently Added", a.Artist, a.Year),
					Key:      a.Key,
					Type:     "album",
				})
			}
			break
		}
	}

	// Add station/radio items from hubs
	for _, hub := range hubs {
		if strings.Contains(strings.ToLower(hub.Context), "station") && len(hub.Playlists) > 0 {
			for _, pl := range hub.Playlists {
				c.items = append(c.items, HomeItem{
					Title:    pl.Title,
					Subtitle: "Radio Station",
					Key:      pl.Key,
					Type:     "station",
				})
			}
		}
	}

	// Add hub album recommendations (limit each hub to 3 items)
	for _, hub := range hubs {
		if len(hub.Albums) == 0 {
			continue
		}
		// Skip stations (already handled) and recently added (handled separately)
		if strings.Contains(strings.ToLower(hub.Context), "station") ||
			strings.Contains(strings.ToLower(hub.Context), "recent.added") {
			continue
		}

		limit := 3
		if len(hub.Albums) < limit {
			limit = len(hub.Albums)
		}
		for i := 0; i < limit; i++ {
			a := hub.Albums[i]
			c.items = append(c.items, HomeItem{
				Title:    a.Title,
				Subtitle: fmt.Sprintf("%s • %s", a.Artist, hub.Title),
				Key:      a.Key,
				Type:     "album",
			})
		}
	}

	c.updateContent()
}

// updateContent rebuilds the viewport content based on current items and selection
func (c *HomeComponent) updateContent() {
	var b strings.Builder

	// Group items by section for display
	var currentSection string
	itemIdx := 0

	for i, item := range c.items {
		// Determine section header
		var section string
		switch {
		case item.Type == "station":
			section = "Stations"
		case strings.Contains(item.Subtitle, "Recently Added"):
			section = "Recently Added"
		case item.Type == "artist":
			section = "Recently Played Artists"
		default:
			// Extract hub title from subtitle (format: "Artist • Hub Title")
			parts := strings.Split(item.Subtitle, " • ")
			if len(parts) > 1 {
				section = parts[1]
			} else {
				section = "Recommendations"
			}
		}

		// Print section header if changed
		if section != currentSection {
			if currentSection != "" {
				b.WriteString("\n")
			}
			b.WriteString(styles.TitleStyle.Render(section))
			b.WriteString("\n")
			currentSection = section
		}

		// Render item
		prefix := "  "
		style := styles.PrimaryTextStyle()
		if i == c.selectedIdx {
			prefix = "> "
			style = styles.SelectedItemStyle
		}

		if item.Type == "station" || item.Type == "artist" {
			b.WriteString(fmt.Sprintf("%s%s\n", prefix, style.Render(item.Title)))
		} else {
			// For albums, show title and artist
			parts := strings.Split(item.Subtitle, " • ")
			artistInfo := parts[0]
			b.WriteString(fmt.Sprintf("%s%s\n", prefix, style.Render(fmt.Sprintf("%s — %s", item.Title, artistInfo))))
		}
		itemIdx++
	}

	c.viewport.SetContent(b.String())
}

// ensureVisible scrolls the viewport to ensure the selected item is visible
func (c *HomeComponent) ensureVisible() {
	// Calculate approximate line position of selected item
	// Each item is roughly 1 line, plus section headers add lines
	linePos := 0
	currentSection := ""
	sectionStartLine := 0 // Track where the current section's header starts

	for i, item := range c.items {
		// Check for section change (adds a header line)
		var section string
		switch {
		case item.Type == "station":
			section = "Stations"
		case strings.Contains(item.Subtitle, "Recently Added"):
			section = "Recently Added"
		case item.Type == "artist":
			section = "Recently Played Artists"
		default:
			parts := strings.Split(item.Subtitle, " • ")
			if len(parts) > 1 {
				section = parts[1]
			}
		}
		if section != currentSection {
			if currentSection != "" {
				linePos++ // blank line between sections
			}
			sectionStartLine = linePos // Remember where this section header starts
			linePos++                  // section header
			currentSection = section
		}

		if i == c.selectedIdx {
			break
		}
		linePos++ // item line
	}

	// Scroll viewport to show selected line
	viewStart := c.viewport.YOffset
	viewEnd := viewStart + c.viewport.Height

	if linePos < viewStart {
		// When scrolling up, show the section header if we're at the first item of a section
		// or if the header would be just above the viewport
		if sectionStartLine < viewStart {
			c.viewport.SetYOffset(sectionStartLine)
		} else {
			c.viewport.SetYOffset(linePos)
		}
	} else if linePos >= viewEnd-1 {
		c.viewport.SetYOffset(linePos - c.viewport.Height + 2)
	}
}
