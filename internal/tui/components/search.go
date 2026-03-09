package components

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/tui/styles"
)

// SearchComponent handles the search input and result rendering.
type SearchComponent struct {
	ctx       *app.AppContext
	textInput textinput.Model
	results   []string
	width     int
	height    int
}

// NewSearchComponent creates a new SearchComponent.
func NewSearchComponent(ctx *app.AppContext) *SearchComponent {
	ti := textinput.New()
	ti.Placeholder = "Search albums, artists, playlists"
	ti.CharLimit = 120
	ti.SetWidth(48)
	ti.Blur() // Start blurred, focus when tab is active

	return &SearchComponent{
		ctx:       ctx,
		textInput: ti,
		results:   []string{},
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
	case tea.KeyPressMsg:
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
func (s *SearchComponent) View() tea.View {
	theme := styles.CurrentTheme()

	// Create theme-aware title with proper background
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.TextColor)).
		Bold(true).
		Background(lipgloss.Color(theme.BackgroundColor))
	title := titleStyle.Render("Search")

	// Render results with theme background
	var resultView string
	if len(s.results) == 0 && s.textInput.Value() != "" {
		resultView = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.TertiaryColor)).
			Background(lipgloss.Color(theme.BackgroundColor)).
			Render("No matches")
	} else {
		// Apply background to each result
		styledResults := make([]string, len(s.results))
		for i, result := range s.results {
			styledResults[i] = lipgloss.NewStyle().
				Background(lipgloss.Color(theme.BackgroundColor)).
				Render(result)
		}
		resultView = lipgloss.JoinVertical(lipgloss.Left, styledResults...)
	}

	// Apply background to text input
	styledInput := lipgloss.NewStyle().
		Background(lipgloss.Color(theme.BackgroundColor)).
		Render(s.textInput.View())

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		styledInput,
		"",
		resultView,
	))
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
		for _, a := range s.ctx.Content.Albums() {
			if strings.Contains(strings.ToLower(a.Title), q) ||
				strings.Contains(strings.ToLower(a.Artist), q) {
				// Use centralized renderer for title+artist.
				str := styles.RenderTitleArtist(a.Title, a.Artist, false, false)
				if !seen[str] {
					s.results = append(s.results, str)
					seen[str] = true
				}
			}
		}

		// Search playlists
		for _, pl := range s.ctx.Content.Playlists() {
			if strings.Contains(strings.ToLower(pl.Title), q) {
				str := styles.PrimaryTextStyle().Render(fmt.Sprintf("%s (playlist)", pl.Title))
				if !seen[str] {
					s.results = append(s.results, str)
					seen[str] = true
				}
			}
		}

		// Search tracks
		for _, t := range s.ctx.Content.Tracks() {
			if strings.Contains(strings.ToLower(t.Title), q) ||
				strings.Contains(strings.ToLower(t.Artist), q) ||
				strings.Contains(strings.ToLower(t.Album), q) {
				// Use centralized helper for title/artist, mark as track in the suffix.
				str := lipgloss.JoinHorizontal(lipgloss.Left,
					styles.RenderTitleArtist(t.Title, t.Artist, false, false),
					styles.BlurredStyle.Render(" (track)"),
				)
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
