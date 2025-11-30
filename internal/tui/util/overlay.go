package util

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// findByteIndexForVisibleRune returns the byte index in s that corresponds to
// the start of the Nth visible rune, skipping ANSI escape sequences. If n is
// greater than visible runes, it returns len(s).
func findByteIndexForVisibleRune(s string, n int) int {
	if n <= 0 {
		return 0
	}
	i := 0
	visible := 0
	for i < len(s) {
		// ANSI sequence?
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// consume until final letter
			j := i + 2
			for j < len(s) {
				b := s[j]
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		// decode rune
		_, size := utf8.DecodeRuneInString(s[i:])
		visible++
		if visible == n {
			return i
		}
		i += size
	}
	return len(s)
}

// truncateANSIToVisible truncates s so that it contains at most 'limit'
// visible runes, preserving ANSI escape sequences intact.
func truncateANSIToVisible(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	var buf bytes.Buffer
	i := 0
	visible := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// copy entire ANSI seq
			j := i + 2
			for j < len(s) {
				b := s[j]
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
					j++
					break
				}
				j++
			}
			buf.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if visible < limit {
			buf.WriteString(string(r))
			visible++
			i += size
			continue
		}
		break
	}
	return buf.String()
}

// OverlayTopRight overlays the provided overlay string (which may be multiple
// lines) onto the base multi-line string `base`. The overlay is placed in the
// top-right corner with `rightPadding` characters between the overlay and the
// right edge. The top parameter selects the row index to start (0-indexed).
// If any overlay lines exceed the available width they'll be truncated.
func OverlayTopRight(base string, overlay string, width int, top int, rightPadding int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Ensure baseLines is large enough to overlay onto; pad with empty lines
	// if required so we can safely index into any row.
	for len(baseLines) < top+len(overlayLines) {
		baseLines = append(baseLines, strings.Repeat(" ", width))
	}

	// Apply overlay line by line
	for i, ol := range overlayLines {
		ri := top + i
		if ri < 0 || ri >= len(baseLines) {
			continue
		}
		// Ensure the base line is the full *visible* width (accounting for ANSI sequences)
		bl := baseLines[ri]
		visibleBL := utf8.RuneCountInString(ansiPattern.ReplaceAllString(bl, ""))
		if visibleBL < width {
			bl = bl + strings.Repeat(" ", width-visibleBL)
		} else if visibleBL > width {
			// Crop to 'width' visible runes while preserving ANSI sequences
			bl = truncateANSIToVisible(bl, width)
		}

		// Compute visible runes for overlay line; we also preserve ANSI
		// sequences and will truncate the overlay while keeping those intact
		// if it exceeds available width.
		visibleStrip := ansiPattern.ReplaceAllString(ol, "")
		olVisibleLen := utf8.RuneCountInString(visibleStrip)
		if olVisibleLen > width-rightPadding {
			ol = truncateANSIToVisible(ol, width-rightPadding)
			visibleStrip = ansiPattern.ReplaceAllString(ol, "")
			olVisibleLen = utf8.RuneCountInString(visibleStrip)
		}
		// ol is now truncated and contains ANSI sequences but fits within
		// width-rightPadding visible runes.
		// Compute overlay position
		// Compute position but constrain it to avoid writing over the outer
		// border characters; we clamp to at least 1 (leave a 1-char left
		// border area) and at most width-olVisibleLen-2 (preserve right border).
		pos := width - olVisibleLen - rightPadding
		if pos < 1 {
			pos = 1
		}
		maxPos := width - olVisibleLen - 2
		if pos > maxPos {
			pos = maxPos
		}
		if pos < 0 {
			pos = 0
		}
		// Replace the segment in base line with overlay content
		// Find byte indices in the base line corresponding to the visible rune
		// offsets so we don't split ANSI escape sequences.
		startIdx := findByteIndexForVisibleRune(bl, pos)
		endIdx := findByteIndexForVisibleRune(bl, pos+olVisibleLen)
		// Insert reset codes before and after overlay so styles don't bleed.
		// We only add reset at the end to ensure overlay's style remains intact.
		reset := "\x1b[0m"
		newLine := bl[:startIdx] + ol + reset + bl[endIdx:]
		baseLines[ri] = newLine
	}
	return strings.Join(baseLines, "\n")
}
