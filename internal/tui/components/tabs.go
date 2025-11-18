package components

import (
	"fmt"
	"os"

	styles "plexmusic-tui/internal/tui/styles"

	"github.com/charmbracelet/lipgloss"
	ansi "github.com/charmbracelet/x/ansi"
)

// Tabs renders a row of tab panes centered into the provided width.
// It returns rendered string and total width.
type Tabs struct {
	Names []string
}

// tabPaneChromeWidth is the left+right border and left+right padding in our PaneStyle
const tabPaneChromeWidth = 4

func NewTabs(names []string) *Tabs {
	return &Tabs{Names: names}
}

// Render the tabs into a width, using currently active index to highlight.
// navActive is the index of the tab to highlight (focused). If navActive is out-of-range
// it will be ignored.
func (t *Tabs) Render(nowWidth int, navActive int) (string, int) {
	usable := nowWidth
	if usable < 0 {
		usable = 0
	}
	tabNames := t.Names
	count := len(tabNames)
	if count == 0 {
		return "", 0
	}

	labelWidths := make([]int, count)
	maxLabel := 0
	for i, n := range tabNames {
		w := lipgloss.Width(n)
		labelWidths[i] = w
		if w > maxLabel {
			maxLabel = w
		}
	}

	preferred := make([]int, count)
	minWidths := make([]int, count)
	sumPreferred := 0
	sumMin := 0
	for i := 0; i < count; i++ {
		preferred[i] = labelWidths[i] + 6
		minWidths[i] = labelWidths[i] + tabPaneChromeWidth
		if minWidths[i] < 6 {
			minWidths[i] = 6
		}
		sumPreferred += preferred[i]
		sumMin += minWidths[i]
	}

	if sumMin > usable && usable > 0 {
		uniformMin := usable / count
		if uniformMin < 3 {
			uniformMin = 3
		}
		for i := 0; i < count; i++ {
			minWidths[i] = uniformMin
		}
	}

	widths := make([]int, count)
	if sumPreferred <= usable {
		for i := 0; i < count; i++ {
			widths[i] = preferred[i]
		}
		remaining := usable - sumPreferred
		for remaining > 0 {
			for i := 0; i < count && remaining > 0; i++ {
				widths[i]++
				remaining--
			}
		}
	} else {
		remainingExcess := sumPreferred - usable
		reduced := make([]int, count)
		active := make([]bool, count)
		activeCount := count
		for i := 0; i < count; i++ {
			active[i] = true
		}
		for remainingExcess > 0 && activeCount > 0 {
			sumPrefActive := 0
			for i := 0; i < count; i++ {
				if active[i] {
					sumPrefActive += preferred[i]
				}
			}
			if sumPrefActive == 0 {
				for i := 0; i < count && remainingExcess > 0; i++ {
					if !active[i] {
						continue
					}
					reduced[i]++
					remainingExcess--
					if preferred[i]-reduced[i] <= minWidths[i] {
						reduced[i] = preferred[i] - minWidths[i]
						active[i] = false
						activeCount--
					}
				}
				continue
			}
			allocatedThisPass := 0
			for i := 0; i < count && remainingExcess > 0; i++ {
				if !active[i] {
					continue
				}
				share := (preferred[i] * remainingExcess) / sumPrefActive
				if share <= 0 {
					share = 1
				}
				if share > remainingExcess {
					share = remainingExcess
				}
				reduced[i] += share
				remainingExcess -= share
				allocatedThisPass += share
				if preferred[i]-reduced[i] <= minWidths[i] {
					excessOver := reduced[i] - (preferred[i] - minWidths[i])
					if excessOver > 0 {
						reduced[i] -= excessOver
						remainingExcess += excessOver
					}
					active[i] = false
					activeCount--
				}
			}
			if allocatedThisPass == 0 {
				for i := 0; i < count && remainingExcess > 0; i++ {
					if !active[i] {
						continue
					}
					reduced[i]++
					remainingExcess--
					if preferred[i]-reduced[i] <= minWidths[i] {
						reduced[i] = preferred[i] - minWidths[i]
						active[i] = false
						activeCount--
					}
				}
			}
		}
		for i := 0; i < count; i++ {
			w := preferred[i] - reduced[i]
			if w < minWidths[i] {
				w = minWidths[i]
			}
			widths[i] = w
		}
	}

	parts := make([]string, 0, count)
	totalWidth := 0
	chrome := tabPaneChromeWidth
	innerWidths := make([]int, count)
	for i := 0; i < count; i++ {
		inner := widths[i] - chrome
		if inner < 1 {
			inner = 1
		}
		innerWidths[i] = inner
	}
	debugTabs := os.Getenv("DEBUG_TABS") != ""
	var partWidths []int
	if debugTabs {
		partWidths = make([]int, count)
	}
	for i := 0; i < count; i++ {
		// resolve style based on whether tab is active
		style := styles.BlurredStyle
		if i == navActive {
			style = styles.FocusedStyle
		}
		label := ansi.Truncate(tabNames[i], innerWidths[i], "…")
		tabLabel := lipgloss.NewStyle().Width(innerWidths[i]).Height(1).MaxWidth(innerWidths[i]).Align(lipgloss.Center, lipgloss.Center).Render(style.Render(label))
		paneStyle := styles.PaneStyle(widths[i], 3).Padding(0, 1).Align(lipgloss.Center, lipgloss.Center)
		if i == navActive {
			paneStyle = paneStyle.BorderForeground(lipgloss.Color("#FF8C00"))
		}
		rendered := paneStyle.Render(tabLabel)
		parts = append(parts, rendered)
		totalWidth += widths[i]
		if debugTabs {
			partWidths[i] = lipgloss.Width(rendered)
		}
	}
	tabRow := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	rowWidth := lipgloss.Width(tabRow)
	innerMin := make([]int, count)
	for i := 0; i < count; i++ {
		im := minWidths[i] - chrome
		if im < 1 {
			im = 1
		}
		innerMin[i] = im
	}
	if rowWidth > usable {
		for rowWidth > usable {
			maxIdx := -1
			maxInner := 0
			for i := 0; i < count; i++ {
				if innerWidths[i] > innerMin[i] && innerWidths[i] > maxInner {
					maxIdx = i
					maxInner = innerWidths[i]
				}
			}
			if maxIdx == -1 {
				break
			}
			innerWidths[maxIdx]--
			widths[maxIdx]--
			// resolve style for the reduced tab index
			style := styles.BlurredStyle
			if maxIdx == navActive {
				style = styles.FocusedStyle
			}
			label := ansi.Truncate(tabNames[maxIdx], innerWidths[maxIdx], "…")
			tabLabel := lipgloss.NewStyle().Width(innerWidths[maxIdx]).Height(1).MaxWidth(innerWidths[maxIdx]).Align(lipgloss.Center, lipgloss.Center).Render(style.Render(label))
			paneStyle := styles.PaneStyle(widths[maxIdx], 3).Padding(0, 1).Align(lipgloss.Center, lipgloss.Center)
			if maxIdx == navActive {
				paneStyle = paneStyle.BorderForeground(lipgloss.Color("#FF8C00"))
			}
			parts[maxIdx] = paneStyle.Render(tabLabel)
			tabRow = lipgloss.JoinHorizontal(lipgloss.Left, parts...)
			newRowWidth := lipgloss.Width(tabRow)
			if newRowWidth >= rowWidth {
				break
			}
			rowWidth = newRowWidth
		}
	}
	tabsWidth := rowWidth
	if tabsWidth > usable {
		tabsWidth = usable
	}
	if debugTabs {
		fmt.Fprintf(os.Stderr, "[component tabs] now=%d usable=%d chrome=%d widths=%v inner=%v total=%d row=%d final=%d partWidths=%v\n",
			nowWidth, usable, chrome, widths, innerWidths, totalWidth, rowWidth, tabsWidth, partWidths)
	}
	tabsPane := lipgloss.Place(nowWidth, 3, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Width(tabsWidth).Render(tabRow))
	return tabsPane, tabsWidth
}
