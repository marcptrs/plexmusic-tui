package components

import (
	"strings"
	"testing"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/service"
)

func TestNowPlaying_VolumeDisplayReflectsPbService(t *testing.T) {
	coord := app.NewCoordinator()
	// Ensure coordinator has a playback service
	pbSvc := coord.GetAppContext().Services.PlaybackService()
	if pbSvc == nil {
		coord.GetAppContext().Services.SetPlaybackService(service.NewPlaybackService())
		pbSvc = coord.GetAppContext().Services.PlaybackService()
	}

	// Prepare current track to ensure volume display renders
	track := app.Track{Title: "Test Track", Artist: "Artist", Album: "Album", Duration: 200000}
	coord.GetAppContext().Playback.SetCurrentTrack(&track)

	// Set volume to 0 (100%) and verify text contains 'Volume: 100%'
	pbSvc.SetVolume(0)
	np := NewNowPlayingComponent(coord.GetAppContext(), pbSvc)
	out := np.Render(80, 20)
	if !strings.Contains(out, "Volume: 100%") {
		t.Fatalf("expected Volume: 100%% in rendered output, got: %s", out)
	}

	// Now set volume to 1 (200%) and verify display
	pbSvc.SetVolume(1)
	out2 := np.Render(80, 20)
	if !strings.Contains(out2, "Volume: 200%") {
		t.Fatalf("expected Volume: 200%% in rendered output, got: %s", out2)
	}
}
