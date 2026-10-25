package health

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/james-gibson/isotope"
)

func isotopeRecord(name string) isotope.Record {
	return isotope.Record{
		Name:      name,
		Role:      "adhd",
		Endpoint:  "127.0.0.1:9999",
		TrustRung: 6,
		RungName:  "Certification",
	}
}

// isotopeTestServer starts a health server whose trust verifier is a stub, so
// the registry tests never touch the lezz registry or mDNS.
func isotopeTestServer(t *testing.T, ttl time.Duration, verified *int) (*Server, *httptest.Server) {
	t.Helper()

	s := NewServer(Options{
		IsotopeTTL: ttl,
		VerifyTrust: func(_ context.Context, _ string) int {
			if verified != nil {
				*verified++
			}
			return 4
		},
	})
	return s, httptest.NewServer(s.httpSrv.Handler)
}

func registerIsotope(t *testing.T, serverURL, body string) IsotopeView {
	t.Helper()

	resp, err := http.Post(serverURL+"/isotope/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register status = %d, want 200", resp.StatusCode)
	}

	var view IsotopeView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	return view
}

func listIsotopeViews(t *testing.T, serverURL string) []IsotopeView {
	t.Helper()

	resp, err := http.Get(serverURL + "/isotope/list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}

	var views []IsotopeView
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return views
}

func getIsotope(t *testing.T, serverURL, name string) (int, IsotopeView) {
	t.Helper()

	resp, err := http.Get(serverURL + "/isotope/" + name)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var view IsotopeView
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
			t.Fatalf("decode get: %v", err)
		}
	}
	return resp.StatusCode, view
}

func deleteIsotope(t *testing.T, serverURL, name string) int {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, serverURL+"/isotope/"+name, http.NoBody)
	if err != nil {
		t.Fatalf("build delete: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// A registration is a heartbeat: the view says how long the name is live, and
// re-registering renews it without re-running the trust probe.
func TestIsotopeRegistrationIsLiveAndRenews(t *testing.T) {
	verified := 0
	_, srv := isotopeTestServer(t, time.Minute, &verified)
	defer srv.Close()

	first := registerIsotope(t, srv.URL, `{"name":"workspace-42","role":"standalone","endpoint":"127.0.0.1:64194","protocol":"mcp"}`)
	if first.Status != IsotopeStatusLive {
		t.Fatalf("status = %q, want %q", first.Status, IsotopeStatusLive)
	}
	if first.ExpiresAt == nil {
		t.Fatal("a self-registered isotope has no expiry; a name is not a heartbeat")
	}
	if first.TrustRung != 4 {
		t.Fatalf("trust rung = %d, want the verifier's 4", first.TrustRung)
	}

	second := registerIsotope(t, srv.URL, `{"name":"workspace-42","role":"standalone","endpoint":"127.0.0.1:64194","protocol":"mcp"}`)
	if !second.RegisteredAt.Equal(first.RegisteredAt) {
		t.Fatalf("re-registration changed registered_at: %s -> %s", first.RegisteredAt, second.RegisteredAt)
	}
	if second.LastSeen.Before(first.LastSeen) {
		t.Fatalf("re-registration moved last_seen backwards: %s -> %s", first.LastSeen, second.LastSeen)
	}
	if second.ExpiresAt == nil || !second.ExpiresAt.After(*first.ExpiresAt) {
		t.Fatal("re-registration did not extend the expiry")
	}
	// The endpoint did not change, so the 4s trust probe must not run again.
	if verified != 1 {
		t.Fatalf("trust verified %d time(s) for one endpoint, want 1", verified)
	}
}

// A changed endpoint is a new claim, not a heartbeat.
func TestIsotopeReregisterNewEndpointVerifiesAgain(t *testing.T) {
	verified := 0
	_, srv := isotopeTestServer(t, time.Minute, &verified)
	defer srv.Close()

	registerIsotope(t, srv.URL, `{"name":"adhd","endpoint":"127.0.0.1:9000"}`)
	registerIsotope(t, srv.URL, `{"name":"adhd","endpoint":"127.0.0.1:9001"}`)

	if verified != 2 {
		t.Fatalf("trust verified %d time(s) across a rebind, want 2", verified)
	}
	_, view := getIsotope(t, srv.URL, "adhd")
	if view.Endpoint != "127.0.0.1:9001" {
		t.Fatalf("endpoint = %q, want the rebound endpoint", view.Endpoint)
	}
}

// A name that stops renewing must stop resolving.
func TestIsotopeExpiresWithoutRenewal(t *testing.T) {
	reg, srv := isotopeTestServer(t, time.Minute, nil)
	defer srv.Close()

	registerIsotope(t, srv.URL, `{"name":"workspace-42","endpoint":"127.0.0.1:64194"}`)
	if got := listIsotopeViews(t, srv.URL); len(got) != 1 {
		t.Fatalf("listed %d isotopes, want 1", len(got))
	}

	// Age the heartbeat past its expiry without waiting a minute.
	reg.mu.Lock()
	entry := reg.isotopes["workspace-42"]
	entry.expiresAt = time.Now().Add(-time.Second)
	reg.isotopes["workspace-42"] = entry
	reg.mu.Unlock()

	if got := listIsotopeViews(t, srv.URL); len(got) != 0 {
		t.Fatalf("an expired name is still listed: %+v", got)
	}
	if code, _ := getIsotope(t, srv.URL, "workspace-42"); code != http.StatusNotFound {
		t.Fatalf("resolving an expired name = %d, want 404", code)
	}
}

func TestIsotopeDeleteDeregisters(t *testing.T) {
	_, srv := isotopeTestServer(t, time.Minute, nil)
	defer srv.Close()

	registerIsotope(t, srv.URL, `{"name":"workspace-42","endpoint":"127.0.0.1:64194"}`)

	if code := deleteIsotope(t, srv.URL, "workspace-42"); code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", code)
	}
	if code, _ := getIsotope(t, srv.URL, "workspace-42"); code != http.StatusNotFound {
		t.Fatalf("deleted name still resolves: %d", code)
	}
	if code := deleteIsotope(t, srv.URL, "workspace-42"); code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", code)
	}
}

func TestIsotopeRegisterRejectsInvalidTTL(t *testing.T) {
	_, srv := isotopeTestServer(t, time.Minute, nil)
	defer srv.Close()

	for _, ttl := range []string{`"nope"`, `"0s"`, `"-5m"`} {
		body := `{"name":"x","endpoint":"127.0.0.1:1","ttl":` + ttl + `}`
		resp, err := http.Post(srv.URL+"/isotope/register", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("ttl %s: status = %d, want 400", ttl, resp.StatusCode)
		}
	}
}

// A registration may choose its own heartbeat window; a short one expires
// sooner than the server default.
func TestIsotopeRegistrationAcceptsTTL(t *testing.T) {
	_, srv := isotopeTestServer(t, time.Hour, nil)
	defer srv.Close()

	view := registerIsotope(t, srv.URL, `{"name":"workspace-42","endpoint":"127.0.0.1:64194","ttl":"30s"}`)
	if view.ExpiresAt == nil {
		t.Fatal("no expiry on a TTL'd registration")
	}
	if remaining := time.Until(*view.ExpiresAt); remaining > 31*time.Second {
		t.Fatalf("expiry is %s away, want the requested 30s window", remaining)
	}
}

// Federation-mirrored records have no heartbeat of their own: the membership
// feed removes them, so the sweep must leave them alone and the view must not
// claim they are live.
func TestUpsertIsotopeIsDurable(t *testing.T) {
	reg, srv := isotopeTestServer(t, time.Minute, nil)
	defer srv.Close()

	reg.UpsertIsotope(isotopeRecord("member-a"))

	views := listIsotopeViews(t, srv.URL)
	if len(views) != 1 {
		t.Fatalf("listed %d isotopes, want 1", len(views))
	}
	if views[0].Status != IsotopeStatusRegistered {
		t.Fatalf("status = %q, want %q", views[0].Status, IsotopeStatusRegistered)
	}
	if views[0].ExpiresAt != nil {
		t.Fatalf("mirrored record gained a heartbeat expiry: %s", views[0].ExpiresAt)
	}

	// Age it far beyond the TTL: a mirrored record is not swept.
	reg.mu.Lock()
	entry := reg.isotopes["member-a"]
	entry.lastSeen = time.Now().Add(-24 * time.Hour)
	reg.isotopes["member-a"] = entry
	reg.mu.Unlock()

	if got := listIsotopeViews(t, srv.URL); len(got) != 1 {
		t.Fatalf("a durable record was swept: %+v", got)
	}
}

// A membership upsert must not erase the heartbeat of a self-registered name.
func TestUpsertKeepsSelfRegistrationHeartbeat(t *testing.T) {
	reg, srv := isotopeTestServer(t, time.Minute, nil)
	defer srv.Close()

	registered := registerIsotope(t, srv.URL, `{"name":"adhd","endpoint":"127.0.0.1:9000"}`)

	reg.UpsertIsotope(isotopeRecord("adhd"))

	_, view := getIsotope(t, srv.URL, "adhd")
	if view.ExpiresAt == nil {
		t.Fatal("a membership upsert removed the registration's heartbeat")
	}
	if view.Status != IsotopeStatusLive {
		t.Fatalf("status = %q, want %q", view.Status, IsotopeStatusLive)
	}
	if !view.RegisteredAt.Equal(registered.RegisteredAt) {
		t.Fatalf("upsert changed registered_at: %s -> %s", registered.RegisteredAt, view.RegisteredAt)
	}
}
