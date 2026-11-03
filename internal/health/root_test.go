package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// serveThroughMux exercises the real route table. The root handler is a
// catch-all pattern, so a direct handler call would not prove that every other
// path still 404s.
func serveThroughMux(srv *Server, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(rec, httptest.NewRequest(method, path, http.NoBody))
	return rec
}

// TestRootServesSelfDescription is smoke-alarm#22: mDNS advertises a bare
// http://host:port and nothing else, so "/" is what a discovery consumer
// probes before it asks for a path. A 404 there reads as a dead instance.
func TestRootServesSelfDescription(t *testing.T) {
	t.Parallel()

	srv := NewServer(Options{ServiceName: "root-test", Version: "9.9.9"})
	srv.SetSelfDescription(func() any {
		return map[string]any{"service": map[string]any{"name": "root-test"}}
	})

	root := serveThroughMux(srv, http.MethodGet, "/")
	if root.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want %d", root.Code, http.StatusOK)
	}
	if ct := root.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("GET / Content-Type = %q, want JSON", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(root.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET / body is not JSON: %v", err)
	}
	if _, ok := got["service"]; !ok {
		t.Fatalf("GET / body has no service field: %s", root.Body.String())
	}

	// The root is an alias for the well-known document, not a second identity
	// that can drift from it.
	wellKnown := serveThroughMux(srv, http.MethodGet, srv.opts.SelfDescriptionPath)
	if root.Body.String() != wellKnown.Body.String() {
		t.Fatalf("GET / and GET %s disagree:\nroot: %s\nwell-known: %s",
			srv.opts.SelfDescriptionPath, root.Body.String(), wellKnown.Body.String())
	}
}

// TestRootAnswersWithoutSelfDescriptionFactory covers a server that was never
// wired with a factory: "not configured" is not an absence, so the root still
// states what it can rather than 404ing.
func TestRootAnswersWithoutSelfDescriptionFactory(t *testing.T) {
	t.Parallel()

	srv := NewServer(Options{ServiceName: "bare-root", Version: "0.0.1"})

	rec := serveThroughMux(srv, http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want %d", rec.Code, http.StatusOK)
	}

	var got struct {
		Service   string            `json:"service"`
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET / body is not JSON: %v", err)
	}
	if got.Service != "bare-root" {
		t.Fatalf("GET / service = %q, want bare-root", got.Service)
	}
	if got.Endpoints["healthz"] != srv.opts.HealthzPath {
		t.Fatalf("GET / endpoints.healthz = %q, want %q", got.Endpoints["healthz"], srv.opts.HealthzPath)
	}
	if got.Endpoints["self_description"] != srv.opts.SelfDescriptionPath {
		t.Fatalf("GET / endpoints.self_description = %q, want %q",
			got.Endpoints["self_description"], srv.opts.SelfDescriptionPath)
	}
}

// TestUnknownPathStillNotFound guards the catch-all: answering "/" must not
// turn the whole surface into a 200.
func TestUnknownPathStillNotFound(t *testing.T) {
	t.Parallel()

	srv := NewServer(Options{ServiceName: "root-test"})
	srv.SetSelfDescription(func() any { return map[string]any{"service": "root-test"} })

	for _, path := range []string{"/nope", "/healthz/extra", "/.well-known/nope.json"} {
		rec := serveThroughMux(srv, http.MethodGet, path)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}
