package util

import (
	"fmt"
	"time"

	"plexmusic-tui/internal/domain"
)

// FormatTrackDuration formats a track duration in milliseconds to MM:SS format
func FormatTrackDuration(durationMs int) string {
	durationMin := durationMs / 60000 // Convert ms to minutes
	durationSec := (durationMs % 60000) / 1000
	return fmt.Sprintf("%d:%02d", durationMin, durationSec)
}

// FormatTotalDuration formats the total duration of multiple tracks in MM:SS or HH:MM:SS format
func FormatTotalDuration(totalMs int) string {
	totalSec := totalMs / 1000
	hours := totalSec / 3600
	minutes := (totalSec % 3600) / 60
	seconds := totalSec % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// CalculateTotalDuration sums the durations of all tracks
func CalculateTotalDuration(tracks []domain.Track) int {
	var total int
	for _, t := range tracks {
		total += t.Duration
	}
	return total
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
