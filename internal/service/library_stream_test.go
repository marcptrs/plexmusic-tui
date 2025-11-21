package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"plexmusic-tui/internal/domain"
)

func TestFetchStreamSetsTokenHeaderAndReturnsContent(t *testing.T) {
	// Set up server that checks header and returns content
	h := http.NewServeMux()
	h.HandleFunc("/library/parts/1", func(w http.ResponseWriter, r *http.Request) {
		// Expect header to be set
		token := r.Header.Get("X-Plex-Token")
		if token != "server-token" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "missing token header: %s", token)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		fmt.Fprint(w, "audio-data")
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	service := NewLibraryService(srv.URL, "server-token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	track := &domain.Track{
		Media: []struct {
			Part []struct {
				Key string `json:"key"`
			} `json:"Part"`
		}{
			{
				Part: []struct {
					Key string `json:"key"`
				}{
					{Key: "/library/parts/1"},
				},
			},
		},
	}

	rc, contentType, err := service.FetchStream(ctx, track)
	if err != nil {
		t.Fatalf("FetchStream returned error: %v", err)
	}
	defer rc.Close()

	if contentType != "audio/mpeg" {
		t.Fatalf("expected content type 'audio/mpeg', got '%s'", contentType)
	}

	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(b) != "audio-data" {
		t.Fatalf("unexpected body content: %s", string(b))
	}
}
