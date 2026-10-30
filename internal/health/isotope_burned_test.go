package health

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/james-gibson/isotope"
	"golang.org/x/crypto/ssh"
)

// The burned vector: a keypair whose private half is published on purpose
// (testdata/burned/README.md, copied from isotope/testdata/burned). Testing the
// certificate path with it means the proof does not depend on a key this test
// generated: anyone can read the same files and reproduce the answer.
const (
	// burnedFingerprint is ssh-keygen's fingerprint of the published half: the
	// SHA256 of the OpenSSH wire blob, not of the raw 32-byte key. Asserting it
	// means a regenerated vector shows up as a diff.
	burnedFingerprint = "YzfLqN67xkxrW/BxBnrVTSNDIzrpDEYt0/Twc8jUjOo"
	burnedRoot        = "testdata:zaphod"
)

func burnedPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "burned", name)
}

// burnedKeypair reads both halves of the committed vector and asserts they are
// one pair with the vector's fingerprint. A copy that drifted would otherwise
// still "work" — it would just prove something else.
// sshFingerprint is the value `ssh-keygen -lf` prints: the SHA256 of the
// base64-decoded OpenSSH blob (type + key), base64 without padding.
func sshFingerprint(t *testing.T, line string) string {
	t.Helper()
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		t.Fatalf("not an OpenSSH key line: %q", line)
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		t.Fatalf("key body is not base64: %v", err)
	}
	sum := sha256.Sum256(blob)
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

func burnedKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()

	published, err := os.ReadFile(burnedPath(t, "id_ed25519.pub"))
	if err != nil {
		t.Fatalf("read published half: %v", err)
	}
	key, err := isotope.ParseSSHEd25519(string(published))
	if err != nil {
		t.Fatalf("parse published half: %v", err)
	}
	if got := sshFingerprint(t, string(published)); got != burnedFingerprint {
		t.Fatalf("published half is SHA256:%s, want the burned vector SHA256:%s", got, burnedFingerprint)
	}

	raw, err := os.ReadFile(burnedPath(t, "id_ed25519"))
	if err != nil {
		t.Fatalf("read private half: %v", err)
	}
	parsed, err := ssh.ParseRawPrivateKey(raw)
	if err != nil {
		t.Fatalf("parse private half: %v", err)
	}
	var priv ed25519.PrivateKey
	switch k := parsed.(type) {
	case ed25519.PrivateKey:
		priv = k
	case *ed25519.PrivateKey:
		priv = *k
	default:
		t.Fatalf("private half is %T, want an ed25519 key", parsed)
	}
	if !key.PublicKey.Equal(priv.Public().(ed25519.PublicKey)) {
		t.Fatalf("the committed private half is not the published half's pair")
	}
	return key.PublicKey, priv
}

// TestBurnedRootCertifiesACluster is the public-cluster chain with a key anyone
// can read: the burned root signs a cluster certificate, the alarm pins the
// published half, and the cluster's receipt reaches certification without the
// alarm ever holding the cluster's secret.
func TestBurnedRootCertifiesACluster(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	rootPub, rootPriv := burnedKeypair(t)
	cert, clusterKey := certifiedCluster(t, burnedRoot, rootPriv)

	_, srv, _ := attestedServer(t, nil,
		map[string]string{burnedRoot: base64.StdEncoding.EncodeToString(rootPub)},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	att := isotope.SignReceiptEd25519(clusterKey, cert, endpoint, time.Now())
	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att))
	if view.TrustRung != 6 {
		t.Fatalf("trust rung = %d, want 6 for the burned root's certificate (evidence: %v)",
			view.TrustRung, view.Evidence)
	}
	if view.AttestedBy != attCluster {
		t.Fatalf("attested_by = %q, want %q", view.AttestedBy, attCluster)
	}
}

// TestBurnedKeyIsAPrivateRootForItself is the other shape in docs/trust-roots.md:
// a private root signs its own cluster certificate (issuer == cluster), and is
// valid only where that key is pinned. Here the burned key is both.
func TestBurnedKeyIsAPrivateRootForItself(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	rootPub, rootPriv := burnedKeypair(t)

	selfCert, err := isotope.SignCertificate(isotope.Certificate{
		Cluster:   attCluster,
		PublicKey: base64.StdEncoding.EncodeToString(rootPub),
		Issuer:    attCluster, // self-signed private root
		Scope:     []string{isotope.ScopeAttest},
	}, rootPriv)
	if err != nil {
		t.Fatalf("sign self certificate: %v", err)
	}

	_, srv, _ := attestedServer(t, nil,
		map[string]string{attCluster: base64.StdEncoding.EncodeToString(rootPub)},
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	att := isotope.SignReceiptEd25519(rootPriv, selfCert, endpoint, time.Now())
	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att))
	if view.TrustRung != 6 {
		t.Fatalf("trust rung = %d, want 6 for a pinned self-signed root (evidence: %v)",
			view.TrustRung, view.Evidence)
	}
}

// TestBurnedRootIsNotTrustedUnpinned: the same burned certificate, an alarm with
// no pinned root. A burned key is worthless to a verifier that did not pin it,
// which is the one thing the vector cannot fake.
func TestBurnedRootIsNotTrustedUnpinned(t *testing.T) {
	endpoint := "http://127.0.0.1:64400/mcp"
	_, rootPriv := burnedKeypair(t)
	cert, clusterKey := certifiedCluster(t, burnedRoot, rootPriv)

	_, srv, _ := attestedServer(t, nil, nil,
		isotope.Advertisement{Cluster: attCluster, Host: "127.0.0.1", Port: 19100})
	defer srv.Close()

	att := isotope.SignReceiptEd25519(clusterKey, cert, endpoint, time.Now())
	view := registerIsotope(t, srv.URL, registrationBody(t, endpoint, &att))
	if view.TrustRung != 2 {
		t.Fatalf("trust rung = %d, want 2 when the burned root is not pinned (evidence: %v)",
			view.TrustRung, view.Evidence)
	}
}
