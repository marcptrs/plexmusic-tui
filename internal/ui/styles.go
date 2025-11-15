package ui

import "github.com/charmbracelet/lipgloss"

// Styling constants for the UI
var (
	// Title style - Orange bold text with bottom margin
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF8C00")).
			MarginBottom(1)

	// Focused style - Orange bold text for focused elements
	FocusedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF8C00")).
			Bold(true)

	// Blurred style - Gray text for unfocused elements
	BlurredStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	// Button style - White text on orange background with padding
	ButtonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#FF8C00")).
			Padding(0, 3).
			MarginTop(1)

	// Blurred button style - White text on gray background
	ButtonBlurredStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#666666")).
				Padding(0, 3).
				MarginTop(1)

	// Error style - Red bold text
	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true).
			MarginTop(1)

	// Success style - Green bold text
	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true).
			MarginTop(1)

	// Help style - Gray italic text
	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true)

	// Primary text color - Orange
	primaryColor = lipgloss.Color("#FF8C00")
	// Secondary text color - White
	secondaryColor = lipgloss.Color("#FFFFFF")
	// Tertiary text color - Gray
	tertiaryColor = lipgloss.Color("#888888")
	// Muted text color - Dark gray
	mutedColor = lipgloss.Color("#666666")
	// Error color - Red
	errorColor = lipgloss.Color("#FF0000")
	// Success color - Green
	successColor = lipgloss.Color("#00FF00")
	// Border color - Dark gray
	borderColor = lipgloss.Color("#444444")
)

// PaneStyle creates a style for a UI pane with the given dimensions
func PaneStyle(width, height int) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)
}

// PrimaryTextStyle creates a style for primary colored text
func PrimaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true)
}

// SecondaryTextStyle creates a style for secondary colored text
func SecondaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(secondaryColor)
}

// TertiaryTextStyle creates a style for tertiary colored text
func TertiaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(tertiaryColor)
}

// NothingPlayingStyle creates the style for "Nothing Playing" message
func NothingPlayingStyle() string {
	return lipgloss.NewStyle().
		Foreground(tertiaryColor).
		Render("♫ Nothing Playing")
}

// NothingPlayingHintStyle creates the style for "Nothing Playing" hint text
func NothingPlayingHintStyle() string {
	return lipgloss.NewStyle().
		Foreground(mutedColor).
		Render("Select a track and press Enter to start playback")
}
