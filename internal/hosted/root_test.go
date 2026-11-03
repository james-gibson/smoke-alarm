package hosted

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveThroughMux(srv *Server, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(rec, httptest.NewRequest(method, path, http.NoBody))
	return rec
}

// TestRootServesHostedIdentity is smoke-alarm#22 on the hosted surface: the
// port is handed out as a bare endpoint, so "/" has to say what it is instead
// of 404ing like a dead service.
func TestRootServesHostedIdentity(t *testing.T) {
	t.Parallel()

	srv := NewServer(Options{
		ServiceName: "hosted-root",
		Version:     "1.2.3",
		ListenAddr:  "localhost:8091",
		EnableHTTP:  true,
		EnableMCP:   true,
		MCPEndpoint: "/mcp",
	})

	rec := serveThroughMux(srv, http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want %d", rec.Code, http.StatusOK)
	}

	var got struct {
		Service    string            `json:"service"`
		Version    string            `json:"version"`
		Surface    string            `json:"surface"`
		Endpoints  map[string]string `json:"endpoints"`
		Transports []string          `json:"transports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET / body is not JSON: %v", err)
	}
	if got.Service != "hosted-root" || got.Version != "1.2.3" {
		t.Fatalf("GET / identity = %q/%q, want hosted-root/1.2.3", got.Service, got.Version)
	}
	if got.Surface != "hosted" {
		t.Fatalf("GET / surface = %q, want hosted", got.Surface)
	}
	if got.Endpoints["mcp"] != "/mcp" {
		t.Fatalf("GET / endpoints.mcp = %q, want /mcp", got.Endpoints["mcp"])
	}
	if got.Endpoints["status"] != "/hosted/status" {
		t.Fatalf("GET / endpoints.status = %q, want /hosted/status", got.Endpoints["status"])
	}
	if len(got.Transports) != 1 || got.Transports[0] != "http" {
		t.Fatalf("GET / transports = %v, want [http]", got.Transports)
	}
}

// TestRootSkipsDisabledProtocols keeps the document honest: an endpoint that is
// not served must not be advertised at the root.
func TestRootSkipsDisabledProtocols(t *testing.T) {
	t.Parallel()

	srv := NewServer(Options{
		ServiceName: "hosted-root-mcp-only",
		EnableHTTP:  true,
		EnableMCP:   true,
		EnableACP:   false,
	})

	rec := serveThroughMux(srv, http.MethodGet, "/")
	var got struct {
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET / body is not JSON: %v", err)
	}
	if _, ok := got.Endpoints["acp"]; ok {
		t.Fatalf("GET / advertises acp although it is disabled: %v", got.Endpoints)
	}
}

// TestHostedUnknownPathStillNotFound guards the catch-all: answering "/" must
// not turn the whole surface into a 200.
func TestHostedUnknownPathStillNotFound(t *testing.T) {
	t.Parallel()

	srv := NewServer(Options{ServiceName: "hosted-root"})

	for _, path := range []string{"/nope", "/hosted/nope", "/mcp/extra"} {
		rec := serveThroughMux(srv, http.MethodGet, path)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}
