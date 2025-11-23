package util

import "plexmusic-tui/internal/app"

// NextIndices returns whether queue is used, the updated queue index, and the updated selected track index
// (no duplicates remain)

// given the current queue, queue index, tracks, and selected track index.
func NextIndices(queue []app.Track, queueIndex int, tracks []app.Track, selected int) (isQueue bool, newQueueIndex int, newSelected int) {
	if len(queue) > 0 {
		idx := queueIndex
		if idx < 0 {
			idx = 0
		} else {
			idx++
			if idx >= len(queue) {
				idx = 0
			}
		}
		return true, idx, selected
	}

	if len(tracks) == 0 {
		return false, queueIndex, selected
	}

	idx := selected
	if idx < 0 || idx >= len(tracks)-1 {
		idx = 0
	} else {
		idx++
	}
	return false, queueIndex, idx
}

// PrevIndices returns whether queue is used, the updated queue index, and the updated selected track index
// given the current queue, queue index, tracks, and selected track index.
func PrevIndices(queue []app.Track, queueIndex int, tracks []app.Track, selected int) (isQueue bool, newQueueIndex int, newSelected int) {
	if len(queue) > 0 {
		idx := queueIndex
		if idx <= 0 {
			idx = len(queue) - 1
		} else {
			idx--
		}
		return true, idx, selected
	}

	if len(tracks) == 0 {
		return false, queueIndex, selected
	}

	idx := selected
	if idx <= 0 {
		idx = len(tracks) - 1
	} else {
		idx--
	}
	return false, queueIndex, idx
}
