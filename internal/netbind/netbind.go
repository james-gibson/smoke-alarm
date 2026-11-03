// Package netbind binds a TCP listener to a preferred address, yielding to the
// next free port when that address is already held by an unrelated process.
//
// A listener that hard-fails on a taken port and a listener that runs without
// its surface fail the same way from the outside: a member that is unreachable,
// with nothing in the report saying which port was taken (smoke-alarm#53).
// Scanning upward and reporting the address actually bound lets the caller
// advertise and self-reference the real port instead of the configured one.
package netbind

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// DefaultRetries is the number of higher ports a listener scans past its
// configured one before it gives up and reports the whole range as taken.
const DefaultRetries = 10

// Listen binds addr over TCP and returns the listener together with the address
// it actually bound.
//
// When the port in addr is held, Listen scans up to maxRetries higher ports. A
// port of 0 asks the kernel for an ephemeral port and does not scan: the answer
// is whatever the kernel assigned, reported from the listener itself.
//
// The returned address is the configured host with the bound port, so callers
// can compare it against their configuration; for an ephemeral bind it is the
// listener's own address. Errors name the address that could not be bound.
func Listen(addr string, maxRetries int) (net.Listener, string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, "", fmt.Errorf("netbind: invalid listen addr %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, "", fmt.Errorf("netbind: invalid port in %q: %w", addr, err)
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	// An ephemeral request is already "any free port": there is nothing to scan
	// past, and the kernel does not report which port it chose until we look.
	if port == 0 {
		ln, listenErr := listen(addr)
		if listenErr != nil {
			return nil, "", fmt.Errorf("netbind: listen %s: %w", addr, listenErr)
		}
		return ln, ln.Addr().String(), nil
	}
	for i := 0; i <= maxRetries; i++ {
		candidate := net.JoinHostPort(host, strconv.Itoa(port+i))
		ln, listenErr := listen(candidate)
		if listenErr == nil {
			return ln, candidate, nil
		}
		if !IsAddrInUse(listenErr) {
			return nil, "", fmt.Errorf("netbind: listen %s: %w", candidate, listenErr)
		}
	}
	return nil, "", fmt.Errorf("netbind: no free port in %s – %s:%d (all %d addresses are in use)",
		addr, host, port+maxRetries, maxRetries+1)
}

func listen(addr string) (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(context.Background(), "tcp", addr)
}

// IsAddrInUse reports whether err means the address is held by another process
// rather than being unusable for a reason retrying cannot fix (bad host, no
// permission, a non-TCP address).
func IsAddrInUse(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "address already in use") ||
		strings.Contains(err.Error(), "bind: can't assign requested address"))
}
