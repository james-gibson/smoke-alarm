package netbind

import (
	"context"
	"fmt"
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

// holdBlock reserves n consecutive loopback ports and returns the first address
// together with every listener it holds. It reports ok=false when no run of n
// consecutive ports is available, letting the caller try a different block
// instead of asserting on a range somebody else is using.
//
// Consecutive ports are what make a scan observable. What a test may not do is
// assume a particular neighbor is free: any other process on the machine can hold
// port+1, in which case Listen correctly scans past it and an exact-port assertion
// fails for a reason that has nothing to do with the code (smoke-alarm#74). Here
// the block is ours by construction, so "the scan went past it" is a fact.
func holdBlock(t *testing.T, n int) (string, []net.Listener, bool) {
	t.Helper()

	for attempt := 0; attempt < 20; attempt++ {
		var lc net.ListenConfig
		first, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
		if err != nil {
			return "", nil, false
		}
		base := portOf(t, first.Addr().String())
		block := []net.Listener{first}

		for port := base + 1; port < base+n; port++ {
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			ln, bindErr := lc.Listen(context.Background(), "tcp", addr)
			if bindErr != nil {
				break // the run collides with something else; release and retry
			}
			block = append(block, ln)
		}

		if len(block) == n {
			t.Cleanup(func() {
				for _, ln := range block {
					_ = ln.Close()
				}
			})
			return net.JoinHostPort("127.0.0.1", strconv.Itoa(base)), block, true
		}
		for _, ln := range block {
			_ = ln.Close()
		}
	}

	return "", nil, false
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
	// A port that was free a moment ago can be taken by another process, or its
	// whole neighborhood can be, before Listen binds it. That is the environment
	// moving, not Listen misbehaving, so retry with a fresh address; the assertion
	// itself stays exact.
	const attempts = 5

	var last string
	for attempt := 1; attempt <= attempts; attempt++ {
		addr := freeAddr(t)

		ln, bound, err := Listen(addr, DefaultRetries)
		if err != nil {
			last = err.Error()
			continue
		}
		_ = ln.Close()

		if bound == addr {
			return
		}
		last = fmt.Sprintf("bound address %q != configured %q", bound, addr)
	}
	t.Fatalf("Listen did not bind a free configured address in %d attempts: %s", attempts, last)
}

func TestListenScansPastHeldPort(t *testing.T) {
	// The block is held by us, so every assertion below is about Listen and not
	// about the machine: it must not bind a held port, it must bind above the
	// block, it must stay inside the configured range, and it must keep the host.
	// Whether it stopped at the *first* free port is not asserted - after the fact
	// a port that was held during the scan and released since is indistinguishable
	// from one Listen skipped, so that claim cannot be made without flaking.
	const (
		blockSize = 3
		attempts  = 5
	)

	var last string
	for attempt := 1; attempt <= attempts; attempt++ {
		first, block, ok := holdBlock(t, blockSize)
		if !ok {
			last = "could not reserve a block"
			continue
		}
		base := portOf(t, first)
		blockEnd := base + len(block) - 1

		ln, bound, err := Listen(first, DefaultRetries)
		if err != nil {
			last = err.Error() // the range around the block was taken around us
			continue
		}
		got := portOf(t, bound)
		host, _, splitErr := net.SplitHostPort(bound)
		_ = ln.Close()

		if bound == first {
			t.Fatalf("bound the held port %q", first)
		}
		if got <= blockEnd {
			t.Fatalf("bound port %d, inside the held block %d-%d", got, base, blockEnd)
		}
		if maxPort := base + DefaultRetries; got > maxPort {
			t.Fatalf("bound port %d, beyond the scanned range %d-%d", got, base, maxPort)
		}
		if splitErr != nil || host != "127.0.0.1" {
			t.Fatalf("bound address %q did not keep the configured host", bound)
		}

		return
	}
	t.Fatalf("Listen did not scan past a held block in %d attempts: %s", attempts, last)
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
