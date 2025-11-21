package util

// GetNavPaneWidth calculates the navigation pane width
func GetNavPaneWidth(totalWidth int) int {
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	navWidth := usableWidth * 20 / 100
	if navWidth < 20 {
		navWidth = 20
	}
	return navWidth
}

// GetContentPaneWidth calculates the content pane width
func GetContentPaneWidth(totalWidth int) int {
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	contentWidth := usableWidth * 30 / 100
	if contentWidth < 30 {
		contentWidth = 30
	}
	return contentWidth
}

// GetDetailPaneWidth calculates the detail pane width
func GetDetailPaneWidth(totalWidth int) int {
	if totalWidth == 0 {
		totalWidth = 120
	}
	usableWidth := totalWidth - 6
	detailWidth := usableWidth * 40 / 100
	if detailWidth < 40 {
		detailWidth = 40
	}
	return detailWidth
}
