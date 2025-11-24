package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"testing"

	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
)

func TestPlaybackController_Next_QueuePreferred(t *testing.T) {
	pc := NewPlaybackController(nil)
	queue := []domain.Track{{Title: "A"}, {Title: "B"}, {Title: "C"}}
	tracks := []domain.Track{{Title: "1"}, {Title: "2"}}
	isQ, newQ, _, next := pc.Next(queue, 0, tracks, 0)
	if !isQ {
		t.Fatalf("expected queue to be preferred")
	}
	if newQ != 1 {
		t.Fatalf("expected new queue index 1, got %d", newQ)
	}
	if next == nil || next.Title != "B" {
		t.Fatalf("expected next track Title 'B', got %v", next)
	}
}

func TestPlaybackController_Prev_QueuePreferred(t *testing.T) {
	pc := NewPlaybackController(nil)
	queue := []domain.Track{{Title: "A"}, {Title: "B"}, {Title: "C"}}
	tracks := []domain.Track{{Title: "1"}, {Title: "2"}}
	isQ, newQ, _, prev := pc.Prev(queue, 0, tracks, 0)
	if !isQ {
		t.Fatalf("expected queue to be preferred")
	}
	if newQ != len(queue)-1 {
		t.Fatalf("expected new queue index wrap to last, got %d", newQ)
	}
	if prev == nil || prev.Title != "C" {
		t.Fatalf("expected prev track Title 'C', got %v", prev)
	}
}

func TestPlaybackController_Next_TracksWhenNoQueue(t *testing.T) {
	pc := NewPlaybackController(nil)
	queue := []domain.Track{}
	tracks := []domain.Track{{Title: "1"}, {Title: "2"}, {Title: "3"}}
	isQ, _, newSelected, next := pc.Next(queue, -1, tracks, 0)
	if isQ {
		t.Fatalf("did not expect queue preference when empty")
	}
	if newSelected != 1 {
		t.Fatalf("expected selected to advance to 1, got %d", newSelected)
	}
	if next == nil || next.Title != "2" {
		t.Fatalf("expected next track Title '2', got %v", next)
	}
}

func TestPlaybackController_Prev_TracksWhenNoQueue(t *testing.T) {
	pc := NewPlaybackController(nil)
	queue := []domain.Track{}
	tracks := []domain.Track{{Title: "1"}, {Title: "2"}, {Title: "3"}}
	isQ, _, newSelected, prev := pc.Prev(queue, -1, tracks, 0)
	if isQ {
		t.Fatalf("did not expect queue preference when empty")
	}
	if newSelected != len(tracks)-1 {
		t.Fatalf("expected prev to wrap to last track, got %d", newSelected)
	}
	if prev == nil || prev.Title != "3" {
		t.Fatalf("expected prev track Title '3', got %v", prev)
	}
}

// mock pb service that returns error for PlayDomainTrack
type mockPbSvcPlayErr struct {
	err error
}

func (m *mockPbSvcPlayErr) Play(track *domain.Track) error { return m.err }
func (m *mockPbSvcPlayErr) Pause() error                   { return m.err }
func (m *mockPbSvcPlayErr) Resume() error                  { return m.err }
func (m *mockPbSvcPlayErr) Stop() error                    { return m.err }
func (m *mockPbSvcPlayErr) Seek(position int) error        { return m.err }
func (m *mockPbSvcPlayErr) SetVolume(v float64)            {}
func (m *mockPbSvcPlayErr) GetPosition() int               { return 0 }
func (m *mockPbSvcPlayErr) GetDuration() int               { return 0 }
func (m *mockPbSvcPlayErr) GetState() domain.PlaybackState { return domain.PlaybackStopped }
func (m *mockPbSvcPlayErr) GetVolume() float64             { return 0 }
func (m *mockPbSvcPlayErr) PlayDomainTrack(ctx context.Context, lib interface {
	FetchStream(ctx context.Context, t *domain.Track) (io.ReadCloser, string, error)
}, track *domain.Track,
) error {
	return m.err
}

func (m *mockPbSvcPlayErr) Subscribe(ctx context.Context) <-chan pubsub.Event[PlaybackEvent] {
	ch := make(chan pubsub.Event[PlaybackEvent], 1)
	close(ch)
	return ch
}

func TestPlaybackController_PlayNext_WithPbSvcError(t *testing.T) {
	errVal := errors.New("playback error")
	mockPB := &mockPbSvcPlayErr{err: errVal}
	pc := NewPlaybackController(mockPB)
	q := []domain.Track{{Title: "A"}}
	tracks := []domain.Track{{Title: "1"}}
	// With the updated non-wrapping queue semantics, pass -1 to indicate we
	// want the controller to use the first queued item (index 0) as the next.
	isQ, newQ, _, played, err := pc.PlayNext(context.Background(), q, -1, tracks, 0, nil)
	if err != nil {
		t.Fatalf("did not expect error from PlayNext since controller no longer calls pbSvc: %v", err)
	}
	if played == nil {
		t.Fatalf("expected played track returned despite error")
	}
	if !isQ {
		t.Fatalf("expected queue preferred path")
	}
	if newQ != 0 {
		t.Fatalf("expected queue index 0, got %d", newQ)
	}
}

func TestPlaybackController_PlayPrev_WithPbSvcError(t *testing.T) {
	errVal := errors.New("playback error")
	mockPB := &mockPbSvcPlayErr{err: errVal}
	pc := NewPlaybackController(mockPB)
	q := []domain.Track{{Title: "A"}}
	tracks := []domain.Track{{Title: "1"}}
	// With the updated non-wrapping queue semantics, pass -1 to indicate we
	// want the controller to use the queue's last item as the prev selection.
	isQ, newQ, _, played, err := pc.PlayPrev(context.Background(), q, -1, tracks, 0, nil)
	if err != nil {
		t.Fatalf("did not expect error from PlayPrev since controller no longer calls pbSvc: %v", err)
	}
	if played == nil {
		t.Fatalf("expected prev track returned despite error")
	}
	if !isQ {
		t.Fatalf("expected queue preferred path")
	}
	if newQ != 0 {
		t.Fatalf("expected queue index 0, got %d", newQ)
	}
}

// mock pb service that stores and returns a gaussian/log-scale volume value
type mockPbSvcVolume struct {
	vol float64
}

func (m *mockPbSvcVolume) Play(track *domain.Track) error { return nil }
func (m *mockPbSvcVolume) Pause() error                   { return nil }
func (m *mockPbSvcVolume) Resume() error                  { return nil }
func (m *mockPbSvcVolume) Stop() error                    { return nil }
func (m *mockPbSvcVolume) Seek(position int) error        { return nil }
func (m *mockPbSvcVolume) SetVolume(v float64)            { m.vol = v }
func (m *mockPbSvcVolume) GetPosition() int               { return 0 }
func (m *mockPbSvcVolume) GetDuration() int               { return 0 }
func (m *mockPbSvcVolume) GetState() domain.PlaybackState { return domain.PlaybackStopped }
func (m *mockPbSvcVolume) GetVolume() float64             { return m.vol }
func (m *mockPbSvcVolume) PlayDomainTrack(ctx context.Context, lib interface {
	FetchStream(ctx context.Context, t *domain.Track) (io.ReadCloser, string, error)
}, track *domain.Track,
) error {
	return nil
}

func (m *mockPbSvcVolume) Subscribe(ctx context.Context) <-chan pubsub.Event[PlaybackEvent] {
	ch := make(chan pubsub.Event[PlaybackEvent], 1)
	close(ch)
	return ch
}

func TestPlaybackController_AdjustVolume_Steps(t *testing.T) {
	// We'll verify that adjusting by +5 or -5 percent results in an exact
	// displayed change of +/-5 percentage points after rounding.
	cases := []struct{ startPct, delta int }{
		{0, 5},
		{1, 5},
		{10, 5},
		{50, 5},
		{95, 5},
		{97, 5},
		{100, 5},
		{195, 5},
		{200, 5},
		{100, -5},
		{5, -5},
		{1, -5},
		{0, -5},
	}
	for _, c := range cases {
		name := fmt.Sprintf("start=%d, delta=%d", c.startPct, c.delta)
		t.Run(name, func(t *testing.T) {
			mock := &mockPbSvcVolume{}
			// Convert starting percent to log2 volume scale
			startVol := math.Log2(float64(c.startPct) / 100.0)
			// Edge: when startPct is 0, math.Log2(0) is -Inf; set to -10 (very low)
			if c.startPct == 0 {
				startVol = -10.0
			}
			mock.vol = startVol
			pc := NewPlaybackController(mock)
			pc.AdjustVolume(float64(c.delta))
			// Compute resulting percentage as displayed (rounding)
			newPct := math.Pow(2, mock.GetVolume()) * 100
			displayed := int(math.Round(newPct))
			// Expected value clamped to 0..200
			expected := c.startPct + c.delta
			if expected < 0 {
				expected = 0
			}
			if expected > 200 {
				expected = 200
			}
			if displayed != expected {
				t.Fatalf("unexpected percent: got %d, expected %d (raw newPct=%.6f)", displayed, expected, newPct)
			}
		})
	}
}
