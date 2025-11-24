package util

import (
	"testing"

	"plexmusic-tui/internal/app"
)

func TestNextIndices_QueuePreferred(t *testing.T) {
	q := []app.Track{{Title: "A"}, {Title: "B"}, {Title: "C"}}
	tracks := []app.Track{{Title: "1"}, {Title: "2"}}
	isQueue, newQ, _ := NextIndices(q, 0, tracks, 0)
	if !isQueue {
		t.Fatalf("expected queue to be preferred")
	}
	if newQ != 1 {
		t.Fatalf("expected new queue index 1, got %d", newQ)
	}
}

func TestNextIndices_QueueEnds(t *testing.T) {
	q := []app.Track{{Title: "A"}, {Title: "B"}}
	tracks := []app.Track{{Title: "1"}}
	isQueue, newQ, _ := NextIndices(q, 1, tracks, 0)
	if !isQueue || newQ != -1 {
		t.Fatalf(
			"expected queue completion (newQueueIndex = -1), got isQueue=%v newQ=%d",
			isQueue,
			newQ,
		)
	}
}

func TestNextIndices_TracksWhenNoQueue(t *testing.T) {
	q := []app.Track{}
	tracks := []app.Track{{Title: "1"}, {Title: "2"}, {Title: "3"}}
	isQueue, _, newSelected := NextIndices(q, -1, tracks, 0)
	if isQueue {
		t.Fatalf("did not expect queue preference when empty")
	}
	if newSelected != 1 {
		t.Fatalf("expected selected to advance to 1, got %d", newSelected)
	}
}

func TestPrevIndices_QueuePreferred(t *testing.T) {
	q := []app.Track{{Title: "A"}, {Title: "B"}, {Title: "C"}}
	tracks := []app.Track{{Title: "1"}, {Title: "2"}}
	isQueue, newQ, _ := PrevIndices(q, 0, tracks, 0)
	if !isQueue {
		t.Fatalf("expected queue to be preferred")
	}
	if newQ != len(q)-1 {
		t.Fatalf("expected new queue index wrap to last, got %d", newQ)
	}
}

func TestPrevIndices_TracksWhenNoQueue(t *testing.T) {
	q := []app.Track{}
	tracks := []app.Track{{Title: "1"}, {Title: "2"}, {Title: "3"}}
	isQueue, _, newSelected := PrevIndices(q, -1, tracks, 0)
	if isQueue {
		t.Fatalf("did not expect queue preference when empty")
	}
	if newSelected != len(tracks)-1 {
		t.Fatalf("expected prev to wrap to last track, got %d", newSelected)
	}
}
