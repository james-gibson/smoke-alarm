package health

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFederationReportDoesNotReNamespacePeerComponents(t *testing.T) {
	t.Parallel()

	s := NewServer(Options{})

	peer := StatusResponse{
		Components: []ComponentStatus{
			{Name: "engine", Healthy: false, Detail: "waiting"},
			{Name: "federation", Healthy: true, Detail: "received 1 targets from upstream"},
			{Name: "federation/abc/engine", Healthy: false, Detail: "waiting"},
		},
	}

	for range 3 {
		body, err := json.Marshal(peer)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/federation/report", bytes.NewReader(body))
		s.handleFederationReport(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	}

	names := map[string]bool{}
	for _, c := range s.Snapshot().Components {
		names[c.Name] = true
	}
	for name := range names {
		if name == "engine" || name == "federation" {
			continue
		}
		t.Errorf("unexpected component %q; peer components must not be re-namespaced", name)
	}
	if !names["engine"] {
		t.Error("peer component engine was not merged")
	}
}
