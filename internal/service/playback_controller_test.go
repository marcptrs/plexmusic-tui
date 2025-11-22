package service

import (
	"context"
	"errors"
	"io"
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
	isQ, newQ, _, played, err := pc.PlayNext(context.Background(), q, 0, tracks, 0, nil)
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
	isQ, newQ, _, played, err := pc.PlayPrev(context.Background(), q, 0, tracks, 0, nil)
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
