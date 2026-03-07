// Package styles provides canonical TUI styling with theme support
package styles

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme represents a complete UI theme that adapts to content (e.g., album art)
type Theme struct {
	BackgroundColor string      // Primary background color
	TextColor       string      // Primary text color (contrasting)
	SecondaryColor  string      // Secondary text/accent color
	TertiaryColor   string      // Tertiary text/muted color
	BorderColor     string      // Border/accent color
	Styles          ThemeStyles // Pre-computed styles
}

// ThemeStyles contains pre-computed Lipgloss styles for efficient rendering
type ThemeStyles struct {
	PrimaryText   lipgloss.Style
	SecondaryText lipgloss.Style
	TertiaryText  lipgloss.Style
	Background    lipgloss.Style
	Border        lipgloss.Style
	Selected      lipgloss.Style
}

// Global theme variables that can be updated at runtime
var (
	currentTheme Theme
	defaultTheme Theme
)

// init initializes the default theme
func init() {
	// Create a default dark theme
	defaultTheme = CreateThemeFromColor("#2a2a2a") // Dark gray
	currentTheme = defaultTheme
}

// CurrentTheme returns the currently active global theme
func CurrentTheme() Theme {
	return currentTheme
}

// SetGlobalTheme updates the global theme
func SetGlobalTheme(theme Theme) {
	currentTheme = theme
}

// ResetToDefaultTheme resets to the default theme
func ResetToDefaultTheme() {
	currentTheme = defaultTheme
}

// CreateThemeFromColor creates a complete theme based on a background color
func CreateThemeFromColor(bgColor string) Theme {
	// Use Lip Gloss's adaptive color function to choose contrasting text
	textColor := getContrastingTextColor(bgColor)

	// Use simple color brightness adjustment
	secondaryColor := adjustBrightness(bgColor, 0.2) // 20% lighter
	tertiaryColor := adjustBrightness(bgColor, 0.4)  // 40% lighter
	borderColor := adjustBrightness(bgColor, -0.1)   // 10% darker

	theme := Theme{
		BackgroundColor: bgColor,
		TextColor:       textColor,
		SecondaryColor:  secondaryColor,
		TertiaryColor:   tertiaryColor,
		BorderColor:     borderColor,
	}

	// Create pre-computed styles for performance
	theme.Styles = ThemeStyles{
		PrimaryText: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.TextColor)).
			Background(lipgloss.Color(theme.BackgroundColor)).
			Bold(true),
		SecondaryText: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.SecondaryColor)).
			Background(lipgloss.Color(theme.BackgroundColor)),
		TertiaryText: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.TertiaryColor)).
			Background(lipgloss.Color(theme.BackgroundColor)),
		Background: lipgloss.NewStyle().
			Background(lipgloss.Color(theme.BackgroundColor)).
			Foreground(lipgloss.Color(theme.TextColor)),
		Border: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.BorderColor)),
		Selected: lipgloss.NewStyle().
			Background(lipgloss.Color(theme.TextColor)).
			Foreground(lipgloss.Color(theme.BackgroundColor)).
			Bold(true),
	}

	return theme
}

// Theme-aware style functions that use the current global theme
func ThemedPrimaryTextStyle() lipgloss.Style {
	return currentTheme.Styles.PrimaryText
}

func ThemedSecondaryTextStyle() lipgloss.Style {
	return currentTheme.Styles.SecondaryText
}

func ThemedTertiaryTextStyle() lipgloss.Style {
	return currentTheme.Styles.TertiaryText
}

func ThemedBackgroundStyle() lipgloss.Style {
	return currentTheme.Styles.Background
}

func ThemedBorderStyle() lipgloss.Style {
	return currentTheme.Styles.Border
}

func ThemedSelectedStyle() lipgloss.Style {
	return currentTheme.Styles.Selected
}

// getContrastingTextColor selects black or white text based on background brightness
// using Lip Gloss's adaptive color functionality
func getContrastingTextColor(bgColor string) string {
	// Remove # if present
	bgColor = strings.TrimPrefix(bgColor, "#")

	// Parse hex color components
	var r, g, b uint8
	if len(bgColor) == 6 {
		_, err := fmt.Sscanf(bgColor, "%02x%02x%02x", &r, &g, &b)
		if err != nil {
			// Default to white text if parsing fails
			return "#FFFFFF"
		}
	} else {
		// Default to white text if invalid format
		return "#FFFFFF"
	}

	// Calculate perceived brightness using YIQ formula
	brightness := (299*float64(r) + 587*float64(g) + 114*float64(b)) / 1000.0

	// Choose contrasting color
	if brightness > 128 {
		return "#000000" // Black text for light backgrounds
	} else {
		return "#FFFFFF" // White text for dark backgrounds
	}
}

// adjustBrightness adjusts the brightness of a hex color by a factor
// factor > 0 makes it brighter, factor < 0 makes it darker
func adjustBrightness(hexColor string, factor float64) string {
	// Remove # if present
	hexColor = strings.TrimPrefix(hexColor, "#")

	// Parse hex color
	var r, g, b uint8
	if len(hexColor) == 6 {
		fmt.Sscanf(hexColor, "%02x%02x%02x", &r, &g, &b)
	} else {
		// Default to dark gray if parsing fails
		return "#2a2a2a"
	}

	// Adjust brightness
	adjust := func(c uint8) uint8 {
		// Convert to float, adjust, and clamp to 0-255
		newVal := float64(c) * (1.0 + factor)
		if newVal < 0 {
			return 0
		}
		if newVal > 255 {
			return 255
		}
		return uint8(newVal)
	}

	newR := adjust(r)
	newG := adjust(g)
	newB := adjust(b)

	return fmt.Sprintf("#%02x%02x%02x", newR, newG, newB)
}
