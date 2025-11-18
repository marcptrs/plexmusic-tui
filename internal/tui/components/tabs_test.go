package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTabs_Render_Basic(t *testing.T) {
	tabNames := []string{"Home", "Recently Added", "Playlists", "Search", "Queue", "Settings"}
	tabs := NewTabs(tabNames)
	pane, _ := tabs.Render(80, 5)
	// Pane width must not exceed requested width
	if lipgloss.Width(pane) > 80 {
		t.Fatalf("expected rendered tabs not exceed width 80, got %d", lipgloss.Width(pane))
	}
	// Settings label should not contain newline
	if strings.Contains(pane, "Settings\n") || strings.Contains(pane, "\nSettings") {
		t.Fatalf("expected Settings not wrap into newline in rendered pane; pane: %q", pane)
	}
}

func TestTabs_Render_TinyWidth_AllTabsPresent(t *testing.T) {
	tabNames := []string{"Home", "Recently Added", "Playlists", "Search", "Queue", "Settings"}
	tabs := NewTabs(tabNames)
	pane, _ := tabs.Render(40, 2)
	// Even at small width, many labels will be truncated to a single letter + ellipsis
	// (e.g., "H…" "R…"). Check that a single-letter+ellipsis form exists for each tab.
	singleEllip := []string{"H…", "R…", "P…", "S…", "Q…", "S…"}
	for _, p := range singleEllip {
		if !strings.Contains(pane, p) {
			t.Fatalf("expected single-letter+ellipsis %q present in pane at tiny width; pane: %q", p, pane)
		}
	}
}
