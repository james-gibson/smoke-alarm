package health

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/james-gibson/isotope"
)

// The certificate half of the keyring (isotope#25): a cluster that presents a
// certificate chaining it to a pinned root is attested without this alarm ever
// holding a shared secret, so a third party can check the receipt and cannot
// mint as the cluster.

func b64(key []byte) string { return base64.StdEncoding.EncodeToString(key) }

// seedRoot makes a root keypair and returns the pinned form of the public half.
func seedRoot(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate root key: %v", err)
	}
	return b64(pub), priv
}

// certifiedCluster mints the cluster's keypair and the certificate a root issues
// for it. The private half stays with the cluster, which is the point: the alarm
// only ever holds the root's public key.
func certifiedCluster(t *testing.T, issuer string, issuerKey ed25519.PrivateKey) (isotope.Certificate, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate cluster key: %v", err)
	}
	cert, err := isotope.SignCertificate(isotope.Certificate{
		Cluster:   attCluster,
		PublicKey: b64(pub),
		Issuer:    issuer,
		Scope:     []string{isotope.ScopeAttest},
	}, issuerKey)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	return cert, priv
}

// TestCertificateRegistrationReachesCertification is the public-cluster case: a
// certificate issued by a pinned root, a registry entry naming this endpoint,
// and an advertisement naming this cluster. No secret is configured at all.
func TestCertificateRegistrationReachesCertification(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	rootPub, rootPriv := seedRoot(t)
	cert, clusterKey := certifiedCluster(t, isotope.PublicSeedCluster, rootPriv)

	_, srv, _ := attestedServer(t, nil,
		map[string]string{isotope.PublicSeedCluster: rootPub},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	att := isotope.SignReceiptEd25519(clusterKey, cert, endpoint, time.Now())
	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att))
	if view.TrustRung != 6 {
		t.Fatalf("trust rung = %d, want 6 for a seeded certificate (evidence: %v)", view.TrustRung, view.Evidence)
	}
	if view.AttestedBy != attCluster {
		t.Fatalf("attested_by = %q, want %q", view.AttestedBy, attCluster)
	}
}

// TestCertificateRegistrationReachesHigherAuthorityWithoutMDNS is the rung the
// issue names: a certificate plus a registry entry that names this endpoint is
// evidence even when no advertisement confirms the cluster, so the rung is 4
// rather than the floor.
func TestCertificateRegistrationReachesHigherAuthorityWithoutMDNS(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	rootPub, rootPriv := seedRoot(t)
	cert, clusterKey := certifiedCluster(t, isotope.PublicSeedCluster, rootPriv)

	_, srv, _ := attestedServer(t, nil, map[string]string{isotope.PublicSeedCluster: rootPub})
	defer srv.Close()

	att := isotope.SignReceiptEd25519(clusterKey, cert, endpoint, time.Now())
	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att))
	if view.TrustRung != 4 {
		t.Fatalf("trust rung = %d, want 4 without an mDNS confirmation (evidence: %v)",
			view.TrustRung, view.Evidence)
	}
}

// TestCertificateWithoutAPinnedRootStaysAtTheFloor: naming seed-42 is not
// trusting it. An alarm that pins no key accepts no public cluster, so the same
// certificate buys nothing — which is what makes an offline alarm's trust the
// same as an online one's.
func TestCertificateWithoutAPinnedRootStaysAtTheFloor(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	_, rootPriv := seedRoot(t)
	cert, clusterKey := certifiedCluster(t, isotope.PublicSeedCluster, rootPriv)

	_, srv, _ := attestedServer(t, nil, nil,
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	att := isotope.SignReceiptEd25519(clusterKey, cert, endpoint, time.Now())
	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att))
	if view.TrustRung != 2 {
		t.Fatalf("trust rung = %d, want 2 when no root is pinned (evidence: %v)", view.TrustRung, view.Evidence)
	}
	if view.AttestedBy != "" {
		t.Fatalf("attested_by = %q, want empty", view.AttestedBy)
	}
}

// TestCertificateRenewalInheritsTheRung keeps the heartbeat cheap for a
// certificate the same way it is for a secret: the renewal proves the same thing
// the first registration did, so it need not re-run the trust probe.
func TestCertificateRenewalInheritsTheRung(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	rootPub, rootPriv := seedRoot(t)
	cert, clusterKey := certifiedCluster(t, isotope.PublicSeedCluster, rootPriv)

	_, srv, calls := attestedServer(t, nil,
		map[string]string{isotope.PublicSeedCluster: rootPub},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	if got := registerIsotope(t, srv.URL, registrationBody(t, endpoint,
		renewableCert(cert, clusterKey, endpoint))); got.TrustRung != 6 {
		t.Fatalf("first registration rung = %d, want 6", got.TrustRung)
	}
	if calls.Load() != 1 {
		t.Fatalf("verifier calls after the first registration = %d, want 1", calls.Load())
	}

	// A fresh receipt (a new issue time) is a heartbeat, not a new claim.
	if got := registerIsotope(t, srv.URL, registrationBody(t, endpoint,
		renewableCert(cert, clusterKey, endpoint))); got.TrustRung != 6 {
		t.Fatalf("renewal rung = %d, want 6", got.TrustRung)
	}
	if calls.Load() != 1 {
		t.Fatalf("certificate renewal re-verified (%d calls), but a heartbeat must stay cheap", calls.Load())
	}
}

func renewableCert(cert isotope.Certificate, key ed25519.PrivateKey, endpoint string) *isotope.Attestation {
	att := isotope.SignReceiptEd25519(key, cert, endpoint, time.Now())
	return &att
}

// TestCertificationAcceptsACertifiedRegistrant: with only roots pinned there is
// no secret, and the certification gate must still demand proof from the
// certificate. Checking only Secrets would leave this alarm open exactly where
// certificates are the only evidence available.
func TestCertificationAcceptsACertifiedRegistrant(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	rootPub, rootPriv := seedRoot(t)
	cert, clusterKey := certifiedCluster(t, isotope.PublicSeedCluster, rootPriv)

	s, srv, _ := attestedServer(t, nil,
		map[string]string{isotope.PublicSeedCluster: rootPub},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()
	s.RegisterFeature(Feature{ID: "isotope.features.trust"})

	if code := postCertify(t, srv.URL, map[string]any{"id": "isotope.features.trust"}); code != http.StatusForbidden {
		t.Fatalf("anonymous certify with roots pinned = %d, want 403", code)
	}

	att := isotope.SignReceiptEd25519(clusterKey, cert, endpoint, time.Now())
	if got := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att)); got.TrustRung != 6 {
		t.Fatalf("registration rung = %d, want 6", got.TrustRung)
	}
	code := postCertify(t, srv.URL, map[string]any{
		"id": "isotope.features.trust", "name": "adhd-1", "endpoint": endpoint, "attestation": att,
	})
	if code != http.StatusOK {
		t.Fatalf("certify by a certified instance = %d, want 200", code)
	}
}

// TestCertificateIssuerIsNotTheCluster pins the one-hop chain's boundary: a
// certificate a cluster signed for itself is valid only where that cluster's own
// key is pinned, so an alarm that pins seed-42 does not accept it.
func TestCertificateIssuerIsNotTheCluster(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	rootPub, _ := seedRoot(t)

	selfPub, selfKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate self key: %v", err)
	}
	selfCert, err := isotope.SignCertificate(isotope.Certificate{
		Cluster:   attCluster,
		PublicKey: b64(selfPub),
		Issuer:    attCluster, // self-signed private root
		Scope:     []string{isotope.ScopeAttest},
	}, selfKey)
	if err != nil {
		t.Fatalf("sign self certificate: %v", err)
	}

	_, srv, _ := attestedServer(t, nil,
		map[string]string{isotope.PublicSeedCluster: rootPub},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	att := isotope.SignReceiptEd25519(selfKey, selfCert, endpoint, time.Now())
	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att))
	if view.TrustRung != 2 {
		t.Fatalf("a self-signed cluster reached rung %d; want 2 unless its own key is pinned (evidence: %v)",
			view.TrustRung, view.Evidence)
	}
}
