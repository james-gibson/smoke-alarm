package health

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStatusReportsBoundListenerAddresses is the reporting half of
// smoke-alarm#53: when a listener claims a port other than the configured one,
// /status must say which address it holds, not leave the operator to infer it
// from an unreachable member.
func TestStatusReportsBoundListenerAddresses(t *testing.T) {
	srv := NewServer(Options{ServiceName: "listeners-test"})
	srv.SetListener("health", "localhost:8088")
	srv.SetListener("hosted", "localhost:18092")
	srv.SetListener("oauth_redirect_mock", "localhost:8878")

	snap := srv.Snapshot()
	if got, want := snap.Listeners["health"], "localhost:8088"; got != want {
		t.Fatalf("health listener = %q, want %q", got, want)
	}
	if got, want := snap.Listeners["hosted"], "localhost:18092"; got != want {
		t.Fatalf("hosted listener = %q, want %q", got, want)
	}
	if got, want := snap.Listeners["oauth_redirect_mock"], "localhost:8878"; got != want {
		t.Fatalf("oauth listener = %q, want %q", got, want)
	}

	// The address reported over HTTP is the same map.
	rec := httptest.NewRecorder()
	srv.handleStatus(rec, httptest.NewRequest("GET", "/status", http.NoBody))
	if rec.Code != 200 {
		t.Fatalf("/status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !contains(body, `"oauth_redirect_mock":"localhost:8878"`) {
		t.Fatalf("/status body does not report the bound OAuth address: %s", body)
	}
}

func TestListenerAddrUnknownIsEmpty(t *testing.T) {
	srv := NewServer(Options{})
	if got := srv.ListenerAddr("hosted"); got != "" {
		t.Fatalf("ListenerAddr(hosted) = %q, want empty for an unbound listener", got)
	}
	if got := srv.Listeners(); len(got) != 0 {
		t.Fatalf("Listeners() = %v, want empty", got)
	}
}

func TestBindWithRetryRecordsHealthListener(t *testing.T) {
	srv := NewServer(Options{ListenAddr: "127.0.0.1:0"})
	bound, err := srv.BindWithRetry(1)
	if err != nil {
		t.Fatalf("BindWithRetry: %v", err)
	}
	defer func() { _ = srv.listener.Close() }()

	if got := srv.ListenerAddr("health"); got != bound {
		t.Fatalf("ListenerAddr(health) = %q, want the bound address %q", got, bound)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestBindListenerRecordsHealthListener covers the seam the integration tests use
// (smoke-alarm#70): a caller that must know the address before the server starts
// binds a listener itself and hands it over, so the port is never released in
// between. /status must report the address that was handed over.
func TestBindListenerRecordsHealthListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind listener: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	bound := ln.Addr().String()
	srv := NewServer(Options{ListenAddr: bound})

	got, err := srv.BindListener(ln)
	if err != nil {
		t.Fatalf("BindListener: %v", err)
	}
	if got != bound {
		t.Fatalf("BindListener returned %q, want the listener's address %q", got, bound)
	}
	if addr := srv.ListenerAddr(ListenerHealth); addr != bound {
		t.Fatalf("ListenerAddr(health) = %q, want %q", addr, bound)
	}
}

// TestBindListenerRejectsNilListener keeps a nil listener from being recorded as a
// bound surface: /status must not claim a listener that does not exist.
func TestBindListenerRejectsNilListener(t *testing.T) {
	srv := NewServer(Options{})
	if _, err := srv.BindListener(nil); err == nil {
		t.Fatal("BindListener(nil) returned no error")
	}
	if addr := srv.ListenerAddr(ListenerHealth); addr != "" {
		t.Fatalf("ListenerAddr(health) = %q after a rejected bind, want empty", addr)
	}
}
