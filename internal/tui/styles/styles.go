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

// Backwards-compatible aliases & new helper styles to mirror legacy UI naming.
// The canonical source for TUI styling is `internal/tui/styles`. These aliases
// maintain compatibility with code that referenced style names from older UI
// helpers; prefer importing `internal/tui/styles` directly in new code.
var (
	// FocusedStyle (alias for ActiveTabStyle)
	// Matches the semantic "focused" look used across the TUI.
	FocusedStyle = ActiveTabStyle

	// BlurredStyle (alias for MutedStyle)
	// When a UI element is not focused it should use a muted color.
	BlurredStyle = MutedStyle

	// ButtonBlurredStyle (disabled/blurred variant for buttons).
	ButtonBlurredStyle = DisabledButtonStyle

	// Help text style (muted + italic)
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// Scrim used when a modal/drawer is active - dims content
	ScrimStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Background(lipgloss.Color("0"))
)

// PaneStyle returns a framed pane with rounded borders and the theme's border color.
// This keeps behavior consistent with the previous pane styling helper and centralizes
// border color and style, so all panes share a unified look.
func PaneStyle(width, height int) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder)
}

// Primary/Secondary/Tertiary text styles to match the names used by `ui`.
func PrimaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
}

func SecondaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(ColorSecondary)
}

func TertiaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(ColorMuted)
}

// Convenience textual helpers to match the old ui package contract.
// These return rendered strings (matching existing use in the codebase).
func NothingPlayingStyle() string {
	return lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("♫ Nothing Playing")
}

func NothingPlayingHintStyle() string {
	return lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("Select a track and press Enter to start playback")
}

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
