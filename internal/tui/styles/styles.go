package styles

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	ColorPrimary   = lipgloss.Color("39")  // Blue
	ColorSecondary = lipgloss.Color("170") // Purple
	ColorSuccess   = lipgloss.Color("10")  // Green
	ColorError     = lipgloss.Color("9")   // Red
	ColorWarning   = lipgloss.Color("11")  // Yellow
	ColorInfo      = lipgloss.Color("12")  // Light blue
	ColorMuted     = lipgloss.Color("240") // Gray
	ColorBorder    = lipgloss.Color("238") // Dark gray
	ColorSelected  = lipgloss.Color("170") // Purple
)

// Common styles
var (
	// TitleStyle for view titles
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(0, 1)

	// SubtitleStyle for subtitles
	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// SelectedItemStyle for selected list items
	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(ColorSelected).
				Bold(true).
				Padding(0, 1)

	// NormalItemStyle for normal list items
	NormalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	// FocusedBorderStyle for focused panes
	FocusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary).
				Padding(1, 2)

	// NormalBorderStyle for unfocused panes
	NormalBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Padding(1, 2)

	// ErrorStyle for error messages
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	// SuccessStyle for success messages
	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Bold(true)

	// InfoStyle for informational messages
	InfoStyle = lipgloss.NewStyle().
			Foreground(ColorInfo)

	// MutedStyle for muted text
	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// ButtonStyle for buttons
	ButtonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(ColorPrimary).
			Padding(0, 2).
			Bold(true)

	// DisabledButtonStyle for disabled buttons
	DisabledButtonStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Background(lipgloss.Color("236")).
				Padding(0, 2)

	// TabStyle for tabs
	TabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(ColorMuted)

	// ActiveTabStyle for active tabs
	ActiveTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(ColorPrimary).
			Background(lipgloss.Color("236")).
			Bold(true)

	// InputStyle for text inputs
	InputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	// FocusedInputStyle for focused text inputs
	FocusedInputStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary).
				Padding(0, 1)
)

// ApplyWidth applies a width to a style
func ApplyWidth(style lipgloss.Style, width int) lipgloss.Style {
	return style.Width(width)
}

// ApplyHeight applies a height to a style
func ApplyHeight(style lipgloss.Style, height int) lipgloss.Style {
	return style.Height(height)
}

// ApplySize applies width and height to a style
func ApplySize(style lipgloss.Style, width, height int) lipgloss.Style {
	return style.Width(width).Height(height)
}
