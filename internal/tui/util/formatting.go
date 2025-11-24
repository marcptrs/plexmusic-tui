package util

import (
	"fmt"
	"time"
)

// FormatTrackDuration formats a track duration in milliseconds to MM:SS format
func FormatTrackDuration(durationMs int) string {
	durationMin := durationMs / 60000 // Convert ms to minutes
	durationSec := (durationMs % 60000) / 1000
	return fmt.Sprintf("%d:%02d", durationMin, durationSec)
}

// FormatTimeDuration formats a time.Duration to MM:SS format
func FormatTimeDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// FormatNumber formats an integer with comma separators (e.g. 1,234,567)
func FormatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 1000 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
