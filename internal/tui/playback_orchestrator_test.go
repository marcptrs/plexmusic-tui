package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/pubsub"
	"plexmusic-tui/internal/service"
	"plexmusic-tui/internal/tui/components"
)

// Mock playback service that returns an error on PlayDomainTrack / Play
type mockPbSvcErr struct {
	err error
}

func (m *mockPbSvcErr) PlayDomainTrack(ctx context.Context, lib interface {
	FetchStream(context.Context, *domain.Track) (io.ReadCloser, string, error)
}, track *domain.Track,
) error {
	return m.err
}

func (m *mockPbSvcErr) Play(t *domain.Track) error {
	return m.err
}
func (m *mockPbSvcErr) Pause() error                   { return nil }
func (m *mockPbSvcErr) Resume() error                  { return nil }
func (m *mockPbSvcErr) Stop() error                    { return nil }
func (m *mockPbSvcErr) Seek(position int) error        { return nil }
func (m *mockPbSvcErr) SetVolume(v float64)            {}
func (m *mockPbSvcErr) GetPosition() int               { return 0 }
func (m *mockPbSvcErr) GetDuration() int               { return 0 }
func (m *mockPbSvcErr) GetState() domain.PlaybackState { return domain.PlaybackStopped }
func (m *mockPbSvcErr) GetVolume() float64             { return 0 }
func (m *mockPbSvcErr) Subscribe(ctx context.Context) <-chan pubsub.Event[service.PlaybackEvent] {
	ch := make(chan pubsub.Event[service.PlaybackEvent], 1)
	close(ch)
	return ch
}

// Mock library service that returns an error when fetching a stream
type mockLibFetchErr struct{}

func (m *mockLibFetchErr) FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error) {
	return nil, "", errors.New("fetch error")
}

func TestOrchestrator_PlayAppTrack_PbErrorSetsNotification(t *testing.T) {
	coord := app.NewCoordinator()
	// Mock PlaybackService that returns an error
	pb := &mockPbSvcErr{err: errors.New("play fail")}
	orchestrator := NewOrchestrator(coord, nil, pb)
	at := &app.Track{Title: "Track 1"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := orchestrator.PlayAppTrack(ctx, at)
	if err == nil {
		t.Fatalf("expected error from PlayAppTrack when pbSvc fails")
	}
	if !coord.NotificationActive() {
		t.Fatalf("expected coordinator to have active notification after playback fail")
	}
}

func TestOrchestrator_PlayNext_PbErrorSetsNotification(t *testing.T) {
	coord := app.NewCoordinator()
	// Real PlaybackService to avoid interfacing issues
	pbSvc := service.NewPlaybackService()
	pc := service.NewPlaybackController(pbSvc)
	// Create orchestrator with libSvc that returns fetch error
	lib := &mockLibFetchErr{}
	orchestrator := NewOrchestrator(coord, lib, pbSvc)
	// Queue and tracks: single track in queue
	q := []app.Track{{Title: "Q1"}}
	tracks := []app.Track{{Title: "T1"}}
	// PlayNext should call PlayDomainTrack which will use lib.FetchStream -> error
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := orchestrator.PlayNext(ctx, pc, q, -1, tracks, 0); err == nil {
		t.Fatalf("expected error from PlayNext due to lib fetch error")
	}
	if !coord.NotificationActive() {
		t.Fatalf("expected coordinator to have active notification after PlayNext fail")
	}
}

func TestOrchestrator_PlayNext_PbSvcErrorSetsNotification(t *testing.T) {
	coord := app.NewCoordinator()
	// Mock PlaybackService that returns error on PlayDomainTrack
	pb := &mockPbSvcErr{err: errors.New("playback fail")}
	// Create a real controller using this mocked pb service
	pc := service.NewPlaybackController(pb)
	lib := &mockLibFetchErr{}
	orchestrator := NewOrchestrator(coord, lib, pb)
	q := []app.Track{{Title: "Q1"}}
	tracks := []app.Track{{Title: "T1"}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := orchestrator.PlayNext(ctx, pc, q, -1, tracks, 0); err == nil {
		t.Fatalf("expected orchestrator PlayNext error due to pbSvc PlayDomainTrack error")
	}
	if !coord.NotificationActive() {
		t.Fatalf("expected coordinator to have notification after pbSvc PlayNext error")
	}
}

func TestOrchestrator_PlayPrev_PbSvcErrorSetsNotification(t *testing.T) {
	coord := app.NewCoordinator()
	pb := &mockPbSvcErr{err: errors.New("playback fail")}
	pc := service.NewPlaybackController(pb)
	lib := &mockLibFetchErr{}
	orchestrator := NewOrchestrator(coord, lib, pb)
	q := []app.Track{{Title: "Q1"}}
	tracks := []app.Track{{Title: "T1"}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := orchestrator.PlayPrev(ctx, pc, q, 0, tracks, 0); err == nil {
		t.Fatalf("expected orchestrator PlayPrev error due to pbSvc PlayDomainTrack error")
	}
	if !coord.NotificationActive() {
		t.Fatalf("expected coordinator to have notification after pbSvc PlayPrev error")
	}
}

func TestOrchestrator_PauseResume_SetsStateAndNotifications(t *testing.T) {
	coord := app.NewCoordinator()
	// Reuse existing mock that returns nil errors for operations when err == nil.
	pb := &mockPbSvcErr{err: nil}
	orchestrator := NewOrchestrator(coord, nil, pb)
	// No context required for Pause/Resume tests
	if err := orchestrator.Pause(); err != nil {
		t.Fatalf("Pause returned unexpected error: %v", err)
	}
	if coord.PlaybackState() != app.PlaybackPaused {
		t.Fatalf("expected paused state, got %v", coord.PlaybackState())
	}
	if err := orchestrator.Resume(); err != nil {
		t.Fatalf("Resume returned unexpected error: %v", err)
	}
	if coord.PlaybackState() != app.PlaybackPlaying {
		t.Fatalf("expected playing state, got %v", coord.PlaybackState())
	}
}

func TestOrchestrator_PlayAppTrack_Success(t *testing.T) {
	coord := app.NewCoordinator()
	pb := &mockPbSvcErr{err: nil}
	orchestrator := NewOrchestrator(coord, nil, pb)
	at := &app.Track{Title: "Track OK", Artist: "Artist"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := orchestrator.PlayAppTrack(ctx, at); err != nil {
		t.Fatalf("expected PlayAppTrack success, got error: %v", err)
	}
	if !coord.HasCurrentTrack() {
		t.Fatalf("expected coordinator to have current track set after PlayAppTrack success")
	}
	if coord.CurrentTrack().Title != "Track OK" {
		t.Fatalf("expected coordinator current track title 'Track OK', got '%s'", coord.CurrentTrack().Title)
	}
	if coord.PlaybackState() != app.PlaybackPlaying {
		t.Fatalf("expected playback state to be playing")
	}
}

func TestOrchestrator_PlayAppTrack_UpdatesNowPlaying(t *testing.T) {
	coord := app.NewCoordinator()
	pb := &mockPbSvcErr{err: nil}
	orch := NewOrchestrator(coord, nil, pb)
	at := &app.Track{Title: "Integration Track", Artist: "Integration Artist"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := orch.PlayAppTrack(ctx, at); err != nil {
		t.Fatalf("expected PlayAppTrack success, got: %v", err)
	}
	// Now create NowPlaying component to render using coordinator/orchestrator
	np := components.NewNowPlayingComponent(coord, orch)
	out := np.Render(80, 20)
	if !strings.Contains(out, "Integration Track") {
		t.Fatalf("expected NowPlaying render to contain 'Integration Track', got: %s", out)
	}
	if !strings.Contains(out, "Integration Artist") {
		t.Fatalf("expected NowPlaying render to contain 'Integration Artist', got: %s", out)
	}
}
