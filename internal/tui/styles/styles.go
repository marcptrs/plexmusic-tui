package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	ColorPrimary      = lipgloss.Color("39")  // Blue
	ColorSecondary    = lipgloss.Color("170") // Purple
	ColorSecondaryDim = lipgloss.Color("139") // Dimmer purple variant for subtle album tint
	// White/bright text used for selected rows and emphasis
	ColorSelectedText = lipgloss.Color("15")
	ColorSuccess      = lipgloss.Color("10")  // Green
	ColorError        = lipgloss.Color("9")   // Red
	ColorWarning      = lipgloss.Color("11")  // Yellow
	ColorInfo         = lipgloss.Color("12")  // Light blue
	ColorMuted        = lipgloss.Color("246") // Slightly lighter gray for better contrast
	ColorBorder       = lipgloss.Color("238") // Dark gray (deprecated, kept for compatibility)
	ColorSelected     = lipgloss.Color("170") // Purple

	// Background colors for panes
	ColorPaneBackground        = lipgloss.Color("235") // Very dark gray for normal panes
	ColorPaneFocusedBackground = lipgloss.Color("237") // Slightly lighter dark gray for focused panes
	ColorInputBackground       = lipgloss.Color("236") // Dark gray for inputs
)

// Common styles
var (
	// TitleStyle for view titles
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(0, 1).
			Background(ColorPaneBackground)

	// SectionTitleStyle is used for in-page section headers (e.g., "Recently Added").
	// Use a slightly different hue to make sections easier to find at a glance.
	SectionTitleStyle = lipgloss.NewStyle().
				Foreground(ColorInfo).
				Bold(true).
				Padding(0, 1).
				Background(ColorPaneBackground)
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

	// FocusedBorderStyle for focused panes (now using background instead of border)
	FocusedBorderStyle = lipgloss.NewStyle().
				Background(ColorPaneFocusedBackground).
				Padding(0, 1)

	// NormalBorderStyle for unfocused panes (now using background instead of border)
	NormalBorderStyle = lipgloss.NewStyle().
				Background(ColorPaneBackground).
				Padding(0, 1)

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
			Foreground(ColorMuted).
			Background(ColorPaneBackground)

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

	// TabStyle for tabs (made compact for split layout)
	TabStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ColorMuted)

	// ActiveTabStyle for active tabs (made compact for split layout)
	ActiveTabStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ColorPrimary).
			Background(lipgloss.Color("236")).
			Bold(true)
		// TabIconStyle is used in icons-only navs to highlight the icon glyphs
		// Make it bold, primary-colored, and slightly larger via extra padding
	TabIconStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(0, 0)

	// InputStyle for text inputs (now using background instead of border)
	InputStyle = lipgloss.NewStyle().
			Background(ColorInputBackground).
			Padding(1, 2)

	// FocusedInputStyle for focused text inputs (now using background instead of border)
	FocusedInputStyle = lipgloss.NewStyle().
				Background(ColorPaneFocusedBackground).
				Padding(1, 2)
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

	// ToastBoxStyle is a compact, foreground-only style for transient toast notifications.
	// Use foreground-only colors (no background) to avoid painting full-width bands
	// in terminal emulators that handle background painting specially.
	ToastBoxStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Padding(0, 0).
			Bold(true)

	// Severity-specific toast styles — conservative, foreground-only "pills".
	ToastSuccessStyle = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Padding(0, 0).
				Bold(true)

	ToastErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Padding(0, 0).
			Bold(true)

	// Info toasts are subtle and use foreground-only coloring.
	ToastInfoStyle = lipgloss.NewStyle().
			Foreground(ColorInfo).
			Padding(0, 0).
			Bold(true)
)

// PaneStyle returns a pane with background color instead of borders.
// This creates visual separation between panes using color instead of border lines.
// Using minimal horizontal padding to create subtle visual spacing.
// The style fills the full width and height with the background color.
func PaneStyle(width, height int) lipgloss.Style {
	// To ensure the background fills the full height, we need to set both
	// Width and Height, and ensure content alignment fills the space.
	// Using MaxWidth/MaxHeight forces the style to expand to fill the space.
	return lipgloss.NewStyle().
		Width(width).
		MaxWidth(width).
		Height(height).
		MaxHeight(height).
		Background(ColorPaneBackground).
		Padding(0, 1).
		AlignVertical(lipgloss.Top).
		AlignHorizontal(lipgloss.Left)
}

// Primary/Secondary/Tertiary text styles to match the names used by `ui`.
func PrimaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Background(ColorPaneBackground).
		Bold(true)
}

func SecondaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Background(ColorPaneBackground)
}

func TertiaryTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(ColorMuted).
		Background(ColorPaneBackground)
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

// RenderTitleArtist composes and styles a title and artist pair centrally.
// If selected is true, the selected styles are used; if playing is true and
// not selected, the title uses SuccessStyle.
func RenderTitleArtist(title, artist string, selected bool, playing bool) string {
	// Build title
	var titleStr string
	if selected {
		titleStr = SelectedItemStyle.Render(title)
	} else if playing {
		titleStr = SuccessStyle.Render(title)
	} else {
		titleStr = PrimaryTextStyle().Render(title)
	}

	if artist == "" {
		return titleStr
	}
	// Build artist + album (artist string contains artist as-is in Home list)
	// We'll split on " - " to separate parts; if none exist, treat artist as whole.
	parts := strings.SplitN(artist, " - ", 2)
	artistPart := parts[0]
	albumPart := ""
	if len(parts) > 1 {
		albumPart = parts[1]
	}
	if selected {
		// For selected rows use high-contrast text over the selected background
		artistStr := lipgloss.NewStyle().Foreground(ColorSelectedText).Background(ColorSelected).Render(artistPart)
		albumStr := ""
		if albumPart != "" {
			// Make album name visible on selected rows: use white text for clarity.
			albumStr = lipgloss.NewStyle().Foreground(ColorSelectedText).Background(ColorSelected).Render(albumPart)
		}
		sep := lipgloss.NewStyle().Foreground(ColorSelectedText).Background(ColorSelected).Render(" - ")
		sp := lipgloss.NewStyle().Background(ColorSelected).Render(" ")
		if albumPart == "" {
			return lipgloss.JoinHorizontal(lipgloss.Left, titleStr, sp, artistStr)
		}
		return lipgloss.JoinHorizontal(lipgloss.Left, titleStr, sp, artistStr, sep, albumStr)
	}

	// Non-selected row
	artistStr := lipgloss.NewStyle().Foreground(ColorSecondary).Background(ColorPaneBackground).Render(artistPart)
	albumStr := ""
	if albumPart != "" {
		albumStr = lipgloss.NewStyle().Foreground(ColorSecondaryDim).Background(ColorPaneBackground).Render(albumPart)
	}
	sep := lipgloss.NewStyle().Foreground(ColorSecondary).Background(ColorPaneBackground).Render(" - ")
	sp := lipgloss.NewStyle().Background(ColorPaneBackground).Render(" ")
	if albumPart == "" {
		return lipgloss.JoinHorizontal(lipgloss.Left, titleStr, sp, artistStr)
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, titleStr, sp, artistStr, sep, albumStr)
}
