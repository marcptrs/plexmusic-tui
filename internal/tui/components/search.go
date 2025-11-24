package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/tui/styles"
)

// SearchComponent handles the search input and result rendering.
type SearchComponent struct {
	coordinator app.Coordinatorer
	textInput   textinput.Model
	results     []string
	width       int
	height      int
}

// NewSearchComponent creates a new SearchComponent.
func NewSearchComponent(coord app.Coordinatorer) *SearchComponent {
	ti := textinput.New()
	ti.Placeholder = "Search albums, artists, playlists"
	ti.CharLimit = 120
	ti.Width = 48
	ti.Blur() // Start blurred, focus when tab is active

	return &SearchComponent{
		coordinator: coord,
		textInput:   ti,
		results:     []string{},
	}
}

// Init initializes the component.
func (s *SearchComponent) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the search component.
func (s *SearchComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			s.PerformSearch()
		case "esc":
			s.textInput.Reset()
			s.results = []string{}
		}
	}

	s.textInput, cmd = s.textInput.Update(msg)
	return s, cmd
}

// View renders the search component.
func (s *SearchComponent) View() string {
	title := styles.TitleStyle.Render("Search")

	// Render results
	var resultView string
	if len(s.results) == 0 && s.textInput.Value() != "" {
		resultView = styles.BlurredStyle.Render("No matches")
	} else {
		resultView = lipgloss.JoinVertical(lipgloss.Left, s.results...)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		s.textInput.View(),
		"",
		resultView,
	)
}

// SetSize sets the component's size.
func (s *SearchComponent) SetSize(w, h int) {
	s.width = w
	s.height = h
}

// Focus focuses the text input.
func (s *SearchComponent) Focus() tea.Cmd {
	return s.textInput.Focus()
}

// Blur blurs the text input.
func (s *SearchComponent) Blur() {
	s.textInput.Blur()
}

// PerformSearch executes the search against the coordinator's data.
func (s *SearchComponent) PerformSearch() {
	term := strings.TrimSpace(s.textInput.Value())
	s.results = []string{}

	if term != "" {
		q := strings.ToLower(term)
		seen := make(map[string]bool)

		// Search albums
		for _, a := range s.coordinator.Albums() {
			if strings.Contains(strings.ToLower(a.Title), q) ||
				strings.Contains(strings.ToLower(a.Artist), q) {
				str := fmt.Sprintf("%s — %s", a.Title, a.Artist)
				if !seen[str] {
					s.results = append(s.results, str)
					seen[str] = true
				}
			}
		}

		// Search playlists
		for _, pl := range s.coordinator.Playlists() {
			if strings.Contains(strings.ToLower(pl.Title), q) {
				str := fmt.Sprintf("%s (playlist)", pl.Title)
				if !seen[str] {
					s.results = append(s.results, str)
					seen[str] = true
				}
			}
		}

		// Search tracks
		for _, t := range s.coordinator.Tracks() {
			if strings.Contains(strings.ToLower(t.Title), q) ||
				strings.Contains(strings.ToLower(t.Artist), q) ||
				strings.Contains(strings.ToLower(t.Album), q) {
				str := fmt.Sprintf("%s — %s (track)", t.Title, t.Artist)
				if !seen[str] {
					s.results = append(s.results, str)
					seen[str] = true
				}
			}
		}
	}
}

// SetValue sets the value of the text input.
func (s *SearchComponent) SetValue(val string) {
	s.textInput.SetValue(val)
	// Trigger search update when value is set programmatically
	s.PerformSearch()
}

// Value returns the current value of the text input.
func (s *SearchComponent) Value() string {
	return s.textInput.Value()
}
