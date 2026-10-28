package integration_test

// listeners_test.go — every integration test serves a listener it already owns.
//
// These tests used to pick a port the only way you can without cooperation from
// the server: bind ":0", read the port, close. The server then bound the same port
// again, leaving a window in which the port belonged to nobody. Every test here is
// t.Parallel(), so another test — or anything else on the machine — could take it,
// the server failed to start on the address the test was already dialing, and the
// failure surfaced three seconds later as "connection refused", pointing at the
// server rather than at the reservation (smoke-alarm#70).
//
// The window is not needed: all three servers keep their listener and serve it
// (BindWithRetry already hands a pre-bound one to http.Server.Serve), so a test can
// bind, take the address, and hand the listener over. The port is owned from the
// first bind until the server shuts down, and nothing has to be re-picked.

import (
	"net"
	"testing"
)

// boundListener opens a loopback listener and returns it with the address it is
// bound to. The caller hands the listener to the server it is about to start, so
// the port is never released in between.
func boundListener(t *testing.T) (net.Listener, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind listener: %v", err)
	}
	// The server closes it on Shutdown; this covers a test that fails before start.
	t.Cleanup(func() { _ = ln.Close() })

	return ln, ln.Addr().String()
}
