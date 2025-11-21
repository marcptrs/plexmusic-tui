package http

import (
	"net/http"
	"testing"
)

func TestIsLocalAddressCases(t *testing.T) {
	cases := []struct {
		host     string
		expected bool
	}{
		{"127.0.0.1:32400", true},
		{"localhost:32400", true},
		{"[::1]:32400", true},
		{"192.168.0.5:32400", true},
		{"10.0.0.5:32400", true},
		{"172.16.0.5:32400", true},
		{"8.8.8.8:32400", false},
		{"example.com:32400", false},
		{"[fe80::1]:32400", true},
	}

	for _, c := range cases {
		ok := isLocalAddress(c.host)
		if ok != c.expected {
			t.Fatalf("isLocalAddress(%q) = %v; want %v", c.host, ok, c.expected)
		}
	}
}

func TestGetForHostTLSBehavior(t *testing.T) {
	cLocal := GetForHost("127.0.0.1:32400")
	if cLocal == nil {
		t.Fatalf("GetForHost returned nil for local host")
	}
	if cLocal.HTTPClient() == nil {
		t.Fatalf("HTTPClient() returned nil for local client")
	}
	if tr, ok := cLocal.HTTPClient().Transport.(*http.Transport); ok {
		if tr.TLSClientConfig == nil || tr.TLSClientConfig.InsecureSkipVerify != true {
			t.Fatalf("expected InsecureSkipVerify=true for local host, got %v", tr.TLSClientConfig)
		}
	} else {
		t.Fatalf("expected underlying transport to be *http.Transport for local host")
	}

	cRemote := GetForHost("8.8.8.8:32400")
	if cRemote == nil {
		t.Fatalf("GetForHost returned nil for remote host")
	}
	// For remote hosts we use the default New() client which contains no custom
	// Transport (Transport is nil). If Transport exists, it should not have
	// InsecureSkipVerify enabled.
	if tr, ok := cRemote.HTTPClient().Transport.(*http.Transport); ok {
		if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify == true {
			t.Fatalf("expected remote host transport to NOT have InsecureSkipVerify")
		}
	}
}
