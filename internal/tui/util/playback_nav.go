package util

import "plexmusic-tui/internal/app"

// NextIndices returns whether queue is used, the updated queue index, and the updated selected track index.
// If a queue is present, it's preferred over the default track list. This function now uses
// non-wrapping semantics for queue navigation: advancing beyond the last queue item signals
// the queue is complete by returning a queue index of -1 (no next queued item).
//
// given the current queue, queue index, tracks, and selected track index.
func NextIndices(
	queue []app.Track,
	queueIndex int,
	tracks []app.Track,
	selected int,
) (isQueue bool, newQueueIndex int, newSelected int) {
	if len(queue) > 0 {
		idx := queueIndex
		// If no current queue index (e.g. -1), start at the first item.
		if idx < 0 {
			idx = 0
			return true, idx, selected
		}
		// Advance to the next queued item; do NOT wrap around to the beginning.
		idx = idx + 1
		if idx >= len(queue) {
			// Queue completed; indicate there is no next queue index.
			return true, -1, selected
		}
		return true, idx, selected
	}

	// No queue present — fall back to legacy tracklist navigation (wrap)
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
func PrevIndices(
	queue []app.Track,
	queueIndex int,
	tracks []app.Track,
	selected int,
) (isQueue bool, newQueueIndex int, newSelected int) {
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
