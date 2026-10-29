package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/james-gibson/isotope"
)

const (
	attCluster = "demo-4321"
	attSecret  = "cluster-launch-secret"
)

// stubAds is a Browser that sees exactly what a test says it sees. The real
// browser is useless in a unit test — and worse, on a host without local-network
// permission it sees nothing and would let a test pass by accident.
type stubAds struct{ ads []isotope.Advertisement }

func (b stubAds) Browse(context.Context, string) ([]isotope.Advertisement, error) {
	return b.ads, nil
}

// attestedServer is a health server with real verification and a fake registry
// and browser: the evidence chain is exercised, the network is not.
func attestedServer(t *testing.T, secrets map[string]string, ads ...isotope.Advertisement) (*Server, *httptest.Server, *atomic.Int32) {
	t.Helper()

	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]map[string]string{
			attCluster: {"adhd_mcp": "http://127.0.0.1:64400/mcp"},
		})
	}))
	t.Cleanup(reg.Close)

	var calls atomic.Int32
	s := NewServer(Options{
		IsotopeTTL: time.Minute,
		TrustConfig: isotope.TrustConfig{
			Secrets:     secrets,
			RegistryURL: reg.URL + "/cluster",
			Browser:     stubAds{ads: ads},
		},
		VerifyRegistration: func(ctx context.Context, cfg isotope.TrustConfig, r isotope.Registration) isotope.TrustVerdict {
			calls.Add(1)
			return isotope.VerifyRegistration(ctx, cfg, r)
		},
	})
	return s, httptest.NewServer(s.httpSrv.Handler), &calls
}

func registrationBody(t *testing.T, endpoint string, att *isotope.Attestation) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"name":        "adhd-1",
		"role":        "cluster",
		"endpoint":    endpoint,
		"protocol":    "mcp",
		"attestation": att,
	})
	if err != nil {
		t.Fatalf("marshal registration: %v", err)
	}
	return string(body)
}

// TestAttestedRegistrationReachesCertification is the end-to-end chain: a
// receipt for the cluster, a registry entry naming this endpoint, and an
// advertisement naming this cluster.
func TestAttestedRegistrationReachesCertification(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	_, srv, _ := attestedServer(t, map[string]string{attCluster: attSecret},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	att := isotope.SignAttestation(attSecret, attCluster, endpoint, time.Now())
	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att))
	if view.TrustRung != 6 {
		t.Fatalf("trust rung = %d, want 6 (evidence: %v)", view.TrustRung, view.Evidence)
	}
	if view.AttestedBy != attCluster {
		t.Fatalf("attested_by = %q, want %q", view.AttestedBy, attCluster)
	}
	if len(view.Evidence) == 0 {
		t.Fatal("a rung was granted with no evidence recorded")
	}
}

// TestUnattestedRegistrationStaysAtTheFloor: the same endpoint the registry
// names, but no receipt. This is #23's exploit, and it must not buy anything.
func TestUnattestedRegistrationStaysAtTheFloor(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	_, srv, _ := attestedServer(t, map[string]string{attCluster: attSecret},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, nil))
	if view.TrustRung != 2 {
		t.Fatalf("trust rung = %d, want 2 for an unauthenticated claim (evidence: %v)", view.TrustRung, view.Evidence)
	}
	if view.AttestedBy != "" {
		t.Fatalf("attested_by = %q, want empty", view.AttestedBy)
	}
}

// TestRenewalInheritsAnAttestedRungOnlyWithAReceipt: a heartbeat must stay cheap
// without letting a repeat of someone else's endpoint inherit their rung.
func TestRenewalInheritsAnAttestedRungOnlyWithAReceipt(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	_, srv, calls := attestedServer(t, map[string]string{attCluster: attSecret},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	att := isotope.SignAttestation(attSecret, attCluster, endpoint, time.Now())
	if got := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att)); got.TrustRung != 6 {
		t.Fatalf("first registration rung = %d, want 6", got.TrustRung)
	}
	if calls.Load() != 1 {
		t.Fatalf("verifier calls after the first registration = %d, want 1", calls.Load())
	}

	// A heartbeat that presents the same receipt is the same claim: no re-probe.
	if got := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att)); got.TrustRung != 6 {
		t.Fatalf("attested renewal rung = %d, want 6", got.TrustRung)
	}
	if calls.Load() != 1 {
		t.Fatalf("attested renewal re-verified (%d calls), but a heartbeat must stay cheap", calls.Load())
	}

	// A renewal with no receipt is a new, unproven claim. It must be verified,
	// and it cannot inherit the attested rung.
	got := registerIsotope(t, srv.URL, registrationBody(t, endpoint, nil))
	if got.TrustRung != 2 {
		t.Fatalf("unattested renewal rung = %d, want 2", got.TrustRung)
	}
	if calls.Load() != 2 {
		t.Fatalf("unattested renewal did not verify (%d calls), so it inherited an unearned rung", calls.Load())
	}
}

// TestCertificationRequiresACertifiedRegistrant is the other half of #23:
// /features/certify used to accept a POST from anyone.
func TestCertificationRequiresACertifiedRegistrant(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	s, srv, _ := attestedServer(t, map[string]string{attCluster: attSecret},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()
	s.RegisterFeature(Feature{ID: "isotope.features.trust"})

	// Anonymous: refused while the alarm holds cluster secrets.
	if code := postCertify(t, srv.URL, map[string]any{"id": "isotope.features.trust"}); code != http.StatusForbidden {
		t.Fatalf("anonymous certify status = %d, want 403", code)
	}

	// A receipt alone is not enough: the named instance must hold rung 6.
	att := isotope.SignAttestation(attSecret, attCluster, endpoint, time.Now())
	code := postCertify(t, srv.URL, map[string]any{
		"id": "isotope.features.trust", "name": "adhd-1", "endpoint": endpoint, "attestation": att,
	})
	if code != http.StatusForbidden {
		t.Fatalf("certify by an unregistered instance = %d, want 403", code)
	}

	// Register properly first — that is what puts it at rung 6 — then certify.
	if got := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att)); got.TrustRung != 6 {
		t.Fatalf("registration rung = %d, want 6", got.TrustRung)
	}
	code = postCertify(t, srv.URL, map[string]any{
		"id": "isotope.features.trust", "name": "adhd-1", "endpoint": endpoint, "attestation": att,
	})
	if code != http.StatusOK {
		t.Fatalf("certify by a certified instance = %d, want 200", code)
	}
}

// TestCertificationStaysOpenWithoutSecrets keeps a secret-less alarm working:
// it has nothing to attest, so it has no certified instance to ask. The rung
// this alarm can grant is the floor, which is the trade recorded in #23.
func TestCertificationStaysOpenWithoutSecrets(t *testing.T) {
	s := NewServer(Options{IsotopeTTL: time.Minute})
	srv := httptest.NewServer(s.httpSrv.Handler)
	defer srv.Close()
	s.RegisterFeature(Feature{ID: "isotope.features.trust"})

	if code := postCertify(t, srv.URL, map[string]any{"id": "isotope.features.trust"}); code != http.StatusOK {
		t.Fatalf("certify without secrets = %d, want 200", code)
	}
}

func postCertify(t *testing.T, baseURL string, body map[string]any) int {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal certify body: %v", err)
	}
	resp, err := http.Post(baseURL+"/features/certify", "application/json", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("certify: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}
