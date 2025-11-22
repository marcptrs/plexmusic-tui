package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"runtime"
	"testing"
	"time"

	"plexmusic-tui/internal/domain"
)

// createSilenceWav returns a minimal WAV file with the given duration seconds.
func createSilenceWav(seconds int) []byte {
	sampleRate := 44100
	bitsPerSample := 16
	numChannels := 1
	numSamples := sampleRate * seconds
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := numSamples * blockAlign

	buff := &bytes.Buffer{}
	buff.WriteString("RIFF")
	_ = binary.Write(buff, binary.LittleEndian, uint32(36+dataSize))
	buff.WriteString("WAVE")
	buff.WriteString("fmt ")
	_ = binary.Write(buff, binary.LittleEndian, uint32(16))
	_ = binary.Write(buff, binary.LittleEndian, uint16(1))
	_ = binary.Write(buff, binary.LittleEndian, uint16(numChannels))
	_ = binary.Write(buff, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buff, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buff, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(buff, binary.LittleEndian, uint16(bitsPerSample))
	buff.WriteString("data")
	_ = binary.Write(buff, binary.LittleEndian, uint32(dataSize))
	zero := make([]byte, dataSize)
	buff.Write(zero)
	return buff.Bytes()
}

func TestPlaybackService_LoadInitializePlay(t *testing.T) {
	if runtime.GOOS == "linux" {
		// On headless Linux CI runners (e.g., GitHub Actions) there may be no
		// ALSA card available even with libasound2-dev installed. Skip tests
		// that initialize the audio subsystem if there is no ALSA presence.
		if _, err := os.Stat("/proc/asound"); os.IsNotExist(err) {
			t.Skip("Skipping audio test since no ALSA devices are present on this runner")
		}
	}
	s := NewPlaybackService()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch := s.Subscribe(ctx)

	// Load wav stream
	wav := createSilenceWav(1)
	rc := io.NopCloser(bytes.NewReader(wav))
	if err := s.LoadStream(rc, "audio/wav"); err != nil {
		t.Fatalf("LoadStream error: %v", err)
	}

	if err := s.Initialize(); err != nil {
		// On CI (headless runners) ALSA driver/device may not be available.
		// Don't fail the entire suite; skip the audio integration test if initialization fails.
		t.Skipf("Skipping audio initialization test: %v", err)
	}

	// Play a dummy track
	track := &domain.Track{Title: "Test"}
	if err := s.Play(track); err != nil {
		t.Fatalf("Play error: %v", err)
	}

	// Read for a playback.started event (there may be playback.loaded/initialized earlier).
	gotStarted := false
	deadline := time.After(2 * time.Second)
	for !gotStarted {
		select {
		case ev := <-ch:
			if ev.Payload.Type == "playback.started" {
				gotStarted = true
				continue
			}
		case <-deadline:
			t.Fatalf("timed out waiting for playback.started event")
		}
	}
	// Ensure position/length updated
	if s.Length() == 0 {
		t.Fatalf("expected non-zero length after load")
	}
	if s.Position() < 0 {
		t.Fatalf("expected non-negative position")
	}

	// Stop playback so test finishes cleanly
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

// Mock library service providing a FetchStream method for tests
type mockLibSvc struct {
	data []byte
}

func (m *mockLibSvc) FetchStream(ctx context.Context, track *domain.Track) (io.ReadCloser, string, error) {
	return io.NopCloser(bytes.NewReader(m.data)), "audio/wav", nil
}

func TestPlaybackService_PlayDomainTrack_Orchestration(t *testing.T) {
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/proc/asound"); os.IsNotExist(err) {
			t.Skip("Skipping audio test since no ALSA devices are present on this runner")
		}
	}
	s := NewPlaybackService()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := s.Subscribe(ctx)

	// Create mock lib service returning a silence wav
	wav := createSilenceWav(1)
	lib := &mockLibSvc{data: wav}

	track := &domain.Track{Title: "TestOrchestration"}

	if err := s.PlayDomainTrack(ctx, lib, track); err != nil {
		t.Fatalf("PlayDomainTrack failed: %v", err)
	}

	// Wait for playback.started
	gotStarted := false
	deadline := time.After(2 * time.Second)
	for !gotStarted {
		select {
		case ev := <-ch:
			if ev.Payload.Type == "playback.started" {
				gotStarted = true
				continue
			}
		case <-deadline:
			t.Fatalf("timed out waiting for playback.started event")
		}
	}
	if s.Length() == 0 {
		t.Fatalf("expected non-zero length after PlayDomainTrack")
	}

	// Stop playback so test finishes cleanly
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}
