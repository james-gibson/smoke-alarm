package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/james-gibson/isotope"
)

func TestUpsertAndRemoveIsotope(t *testing.T) {
	t.Parallel()

	s := NewServer(Options{})

	s.UpsertIsotope(isotope.Record{Name: "alarm-a", Role: "alarm", TrustRung: 4})
	s.UpsertIsotope(isotope.Record{Name: "alarm-b", Role: "alarm", TrustRung: 4})
	// Blank names are ignored.
	s.UpsertIsotope(isotope.Record{Name: "   "})

	got := listIsotopes(t, s)
	if len(got) != 2 {
		t.Fatalf("after upserts, isotope count = %d, want 2", len(got))
	}
	if got[0].RegisteredAt.IsZero() {
		t.Fatal("RegisteredAt was not defaulted on upsert")
	}

	// Upsert replaces an existing record.
	s.UpsertIsotope(isotope.Record{Name: "alarm-a", Role: "alarm", TrustRung: 6})
	got = listIsotopes(t, s)
	if len(got) != 2 {
		t.Fatalf("after replace, isotope count = %d, want 2", len(got))
	}
	if got[0].Name != "alarm-a" || got[0].TrustRung != 6 {
		t.Fatalf("replace failed: got %+v", got[0])
	}

	s.RemoveIsotope("alarm-a")
	s.RemoveIsotope("does-not-exist") // must be a no-op, not a panic
	got = listIsotopes(t, s)
	if len(got) != 1 || got[0].Name != "alarm-b" {
		t.Fatalf("after remove, got %+v, want only alarm-b", got)
	}
}

func listIsotopes(t *testing.T, s *Server) []isotope.Record {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/isotope/list", http.NoBody)
	rec := httptest.NewRecorder()
	s.handleIsotopeList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleIsotopeList status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []isotope.Record
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode isotope list: %v", err)
	}
	return got
}
