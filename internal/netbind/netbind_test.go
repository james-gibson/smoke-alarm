package netbind

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
)

// freeAddr returns a loopback address whose port was free a moment ago.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}
	return addr
}

// holdAddr keeps addr bound for the duration of the test and returns the
// address that was actually held.
func holdAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind occupier: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func portOf(t *testing.T, addr string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi %q: %v", portStr, err)
	}
	return port
}

func TestListenBindsConfiguredAddressWhenFree(t *testing.T) {
	addr := freeAddr(t)

	ln, bound, err := Listen(addr, DefaultRetries)
	if err != nil {
		t.Fatalf("Listen(%q): %v", addr, err)
	}
	defer func() { _ = ln.Close() }()

	if bound != addr {
		t.Fatalf("bound address %q != configured %q", bound, addr)
	}
}

func TestListenScansPastHeldPort(t *testing.T) {
	held := holdAddr(t)

	ln, bound, err := Listen(held, DefaultRetries)
	if err != nil {
		t.Fatalf("Listen(%q): %v", held, err)
	}
	defer func() { _ = ln.Close() }()

	if bound == held {
		t.Fatalf("expected to yield the held port %q", held)
	}
	if got, want := portOf(t, bound), portOf(t, held)+1; got != want {
		t.Fatalf("bound port %d, want the next free port %d", got, want)
	}
}

func TestListenReportsWholeRangeTaken(t *testing.T) {
	// Hold the next two ports from a free one so a scan of 1 retry cannot find
	// anything: the error must name the configured address and the range.
	first := holdAddr(t)
	base := portOf(t, first)
	var held []net.Listener
	for port := base + 1; port <= base+2; port++ {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		var lc net.ListenConfig
		ln, err := lc.Listen(context.Background(), "tcp", addr)
		if err != nil {
			t.Skipf("cannot hold %s in this environment: %v", addr, err)
		}
		held = append(held, ln)
	}
	t.Cleanup(func() {
		for _, ln := range held {
			_ = ln.Close()
		}
	})

	_, _, err := Listen(first, 1)
	if err == nil {
		t.Fatal("expected an error when every address in the range is held")
	}
	if !strings.Contains(err.Error(), first) {
		t.Fatalf("error %q does not name the configured address %q", err, first)
	}
}

func TestListenEphemeralReportsAssignedPort(t *testing.T) {
	ln, bound, err := Listen("127.0.0.1:0", 0)
	if err != nil {
		t.Fatalf("Listen(127.0.0.1:0): %v", err)
	}
	defer func() { _ = ln.Close() }()

	if portOf(t, bound) == 0 {
		t.Fatalf("ephemeral bind reported port 0 in %q", bound)
	}
	if bound != ln.Addr().String() {
		t.Fatalf("bound address %q != listener address %q", bound, ln.Addr().String())
	}
}

func TestListenRejectsInvalidAddress(t *testing.T) {
	for _, addr := range []string{"localhost", "localhost:http"} {
		if _, _, err := Listen(addr, 1); err == nil {
			t.Fatalf("Listen(%q) returned no error", addr)
		}
	}
}

func TestIsAddrInUse(t *testing.T) {
	if IsAddrInUse(nil) {
		t.Fatal("nil error is not an address-in-use error")
	}
	if !IsAddrInUse(errString("listen tcp 127.0.0.1:8877: bind: address already in use")) {
		t.Fatal("address in use error not recognized")
	}
	if IsAddrInUse(errString("listen tcp: lookup nope: no such host")) {
		t.Fatal("unrelated error classified as address in use")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
