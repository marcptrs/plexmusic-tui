// Package colors provides color utilities for theme management
package colors

import (
	"fmt"
	"strings"
)

// ThemeColors represents the current theme colors
var (
	CurrentTextColor       string = "#FFFFFF" // Default to white
	CurrentBackgroundColor string = "#2a2a2a" // Default dark gray
	CurrentSecondaryColor  string = "#FFFFFF" // White for good contrast
	CurrentTertiaryColor   string = "#CCCCCC" // Light gray for secondary text
)

// SetThemeColors updates the global theme colors
func SetThemeColors(textColor, backgroundColor, secondaryColor, tertiaryColor string) {
	CurrentTextColor = textColor
	CurrentBackgroundColor = backgroundColor

	// Calculate secondary and tertiary colors that provide good contrast
	// For dark backgrounds, use lighter colors. For light backgrounds, use darker colors.
	if IsDarkColor(backgroundColor) {
		// Dark background - use lighter colors for secondary text
		CurrentSecondaryColor = "#FFFFFF" // White for good contrast
		CurrentTertiaryColor = "#CCCCCC"  // Light gray for album text
	} else {
		// Light background - use darker colors for secondary text
		CurrentSecondaryColor = "#000000" // Black for good contrast
		CurrentTertiaryColor = "#333333"  // Dark gray for album text
	}
}

// GetThemeColors returns the current theme colors
func GetThemeColors() (textColor, backgroundColor, secondaryColor, tertiaryColor string) {
	return CurrentTextColor, CurrentBackgroundColor, CurrentSecondaryColor, CurrentTertiaryColor
}

// IsDarkColor determines if a color is dark based on its brightness
func IsDarkColor(hexColor string) bool {
	// Remove # if present
	hexColor = strings.TrimPrefix(hexColor, "#")

	// Parse hex color
	var r, g, b uint8
	if len(hexColor) == 6 {
		fmt.Sscanf(hexColor, "%02x%02x%02x", &r, &g, &b)
	} else {
		// Default to dark if parsing fails
		return true
	}

	// Calculate perceived brightness using YIQ formula
	brightness := (299*float64(r) + 587*float64(g) + 114*float64(b)) / 1000.0

	// Return true if dark (brightness <= 128)
	return brightness <= 128
}

// CalculateContrastingTextColor selects black or white text based on background brightness
func CalculateContrastingTextColor(bgColor string) string {
	// Remove # if present
	bgColor = strings.TrimPrefix(bgColor, "#")

	// Parse hex color
	var r, g, b uint8
	if len(bgColor) == 6 {
		fmt.Sscanf(bgColor, "%02x%02x%02x", &r, &g, &b)
	} else {
		// Default to white text if parsing fails
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

// GetSpinnerColor returns a color suitable for spinner dots based on current theme background
// Uses a pink/purple color (similar to original #205) that contrasts well with both light and dark backgrounds
func GetSpinnerColor() string {
	// Use the current background color to determine contrast
	bgColor := CurrentBackgroundColor

	// For dark backgrounds, use a light pink/purple (similar to terminal color 205)
	// For light backgrounds, use a darker purple for better visibility
	if IsDarkColor(bgColor) {
		return "#FF80FF" // Light pink/purple for dark backgrounds
	} else {
		return "#800080" // Dark purple for light backgrounds
	}
}

// AdjustBrightness adjusts the brightness of a hex color by a factor
func AdjustBrightness(hexColor string, factor float64) string {
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
