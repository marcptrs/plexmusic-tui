package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// fakeRoundTripper returns a canned response body for any request.
type fakeRoundTripper struct {
	status int
	body   []byte
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{
		StatusCode: f.status,
		Status:     fmt.Sprintf("%d %s", f.status, http.StatusText(f.status)),
		Body:       io.NopCloser(bytes.NewReader(f.body)),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Request:    req,
	}
	return resp, nil
}

// startListener binds a TCP listener on 127.0.0.1:0 and starts a goroutine
// that accepts and immediately closes incoming connections. It returns the
// listener and the assigned port.
func startListener(t *testing.T) (net.Listener, int) {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Listener closed; exit goroutine.
				return
			}
			_ = conn.Close()
		}
	}()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		ln.Close()
		t.Fatalf("listener address not TCP")
	}
	return ln, tcpAddr.Port
}

func newTestAuthenticatorWithJSON(t *testing.T, body []byte) *Authenticator {
	client := &http.Client{
		Transport: &fakeRoundTripper{
			status: http.StatusOK,
			body:   body,
		},
	}
	return NewAuthenticator(client)
}

// buildResourcesJSON is a helper to build JSON payload returned by Plex /api/v2/resources
func buildResourcesJSON(t *testing.T, conns []map[string]interface{}) []byte {
	resources := []map[string]interface{}{
		{
			"name":        "Test Server",
			"provides":    "server",
			"accessToken": "tok",
			"connections": conns,
		},
	}
	b, err := json.Marshal(resources)
	if err != nil {
		t.Fatalf("failed to marshal resources JSON: %v", err)
	}
	return b
}

func TestFetchServers_ReachabilityPreference(t *testing.T) {
	// Use a slightly longer test timeout to avoid CI flakiness.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("reachable remote preferred over unreachable local", func(t *testing.T) {
		// Create a reachable TCP listener to simulate the remote connection being reachable.
		ln, port := startListener(t)
		defer ln.Close()

		// remote: Local=false and reachable; local: Local=true but not reachable (different port).
		conns := []map[string]interface{}{
			{
				"protocol": "https",
				"address":  "127.0.0.1",
				"port":     port,
				"local":    false,
			},
			{
				"protocol": "https",
				"address":  "127.0.0.1",
				"port":     port + 1,
				"local":    true,
			},
		}

		b := buildResourcesJSON(t, conns)
		a := newTestAuthenticatorWithJSON(t, b)

		servers, err := a.FetchServers(ctx, "token")
		if err != nil {
			t.Fatalf("FetchServers returned error: %v", err)
		}
		if len(servers) != 1 {
			t.Fatalf("expected 1 server returned, got %d", len(servers))
		}
		if servers[0].Host != "127.0.0.1" {
			t.Fatalf("expected host 127.0.0.1, got %s", servers[0].Host)
		}
		if servers[0].Port != fmt.Sprintf("%d", port) {
			t.Fatalf("expected port %d, got %s", port, servers[0].Port)
		}
	})

	t.Run("reachable local preferred if no reachable remote", func(t *testing.T) {
		ln, localPort := startListener(t)
		defer ln.Close()

		// remote: unreachable (unbound port), local: reachable
		conns := []map[string]interface{}{
			{
				"protocol": "https",
				"address":  "127.0.0.2", // loopback variant on the loopback range; no listener here => unreachable
				"port":     localPort + 10,
				"local":    false,
			},
			{
				"protocol": "https",
				"address":  "127.0.0.1",
				"port":     localPort,
				"local":    true,
			},
		}
		b := buildResourcesJSON(t, conns)
		a := newTestAuthenticatorWithJSON(t, b)

		servers, err := a.FetchServers(ctx, "token")
		if err != nil {
			t.Fatalf("FetchServers returned error: %v", err)
		}
		if len(servers) != 1 {
			t.Fatalf("expected 1 server returned, got %d", len(servers))
		}
		if servers[0].Host != "127.0.0.1" {
			t.Fatalf("expected host 127.0.0.1, got %s", servers[0].Host)
		}
		if servers[0].Port != fmt.Sprintf("%d", localPort) {
			t.Fatalf("expected port %d, got %s", localPort, servers[0].Port)
		}
	})

	t.Run("fallback to first remote when none are reachable", func(t *testing.T) {
		// Both connections are unreachable; the fallback should pick the first remote.
		conns := []map[string]interface{}{
			{
				"protocol": "https",
				"address":  "127.0.0.2",
				"port":     49152,
				"local":    false,
			},
			{
				"protocol": "https",
				"address":  "127.0.0.3",
				"port":     49153,
				"local":    true,
			},
		}
		b := buildResourcesJSON(t, conns)
		a := newTestAuthenticatorWithJSON(t, b)

		servers, err := a.FetchServers(ctx, "token")
		if err != nil {
			t.Fatalf("FetchServers returned error: %v", err)
		}
		if len(servers) != 1 {
			t.Fatalf("expected 1 server returned, got %d", len(servers))
		}
		if servers[0].Host != "127.0.0.2" {
			t.Fatalf("expected fallback host 127.0.0.2, got %s", servers[0].Host)
		}
		if servers[0].Port != fmt.Sprintf("%d", 49152) {
			t.Fatalf("expected port 49152, got %s", servers[0].Port)
		}
	})
}
