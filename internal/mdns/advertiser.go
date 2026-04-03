// Package mdns provides mDNS service advertisement for Tuner integration.
// When enabled, smoke-alarm advertises itself as _smoke-alarm._tcp so that
// Tuner instances can passively discover it on the local network.
//
// Implementation uses betamos/zeroconf for cross-platform mDNS.
// The advertiser is optional -- it only starts when config.Tuner.Advertise is true.
package mdns

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/grandcat/zeroconf"
)

// Options configures the mDNS advertiser.
type Options struct {
	ServiceName string            // e.g. "ocd-smoke-alarm"
	ServiceType string            // e.g. "_smoke-alarm._tcp"
	Domain      string            // e.g. "local" (default)
	Port        int               // port number to advertise
	TXT         map[string]string // TXT record key-value pairs
}

// Advertiser manages mDNS service registration.
type Advertiser struct {
	opts   Options
	server *zeroconf.Server
	cancel context.CancelFunc
}

// NewAdvertiser creates an advertiser with the given options.
func NewAdvertiser(opts Options) *Advertiser {
	if opts.Domain == "" {
		opts.Domain = "local."
	} else if !strings.HasSuffix(opts.Domain, ".") {
		// Ensure domain is fully qualified with trailing dot
		opts.Domain += "."
	}
	return &Advertiser{opts: opts}
}

// Start begins advertising the service via mDNS.
// The advertisement continues until Shutdown is called or ctx is canceled.
func (a *Advertiser) Start(ctx context.Context) error {
	ctx, a.cancel = context.WithCancel(ctx)

	txtRecords := formatTXT(a.opts.TXT)

	// Get hostname for the advertisement
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	if !strings.HasSuffix(hostname, ".") {
		hostname += "."
	}

	// Gather local IP addresses for explicit advertisement
	ips, err := getLocalIPs()
	if err != nil {
		log.Printf("mdns: warning: could not get local IPs: %v", err)
	}

	log.Printf("mdns: advertising %s.%s on port %d (host: %s, ips: %v, txt: %v)",
		a.opts.ServiceType, a.opts.Domain, a.opts.Port, hostname, ips, txtRecords)

	// Use RegisterProxy for more control over advertisement
	server, err := zeroconf.RegisterProxy(
		a.opts.ServiceName,
		a.opts.ServiceType,
		a.opts.Domain,
		a.opts.Port,
		hostname,
		ips,
		txtRecords,
		nil, // all interfaces
	)
	if err != nil {
		return fmt.Errorf("mdns: register %s: %w", a.opts.ServiceType, err)
	}
	a.server = server

	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()

	return nil
}

// getLocalIPs returns a list of non-loopback IPv4 addresses.
func getLocalIPs() ([]string, error) {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		// Skip interfaces without multicast support
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil {
				continue // skip IPv6
			}
			ips = append(ips, ip.String())
		}
	}
	return ips, nil
}

// Shutdown stops the mDNS advertisement.
func (a *Advertiser) Shutdown() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.server != nil {
		a.server.Shutdown()
		a.server = nil
	}
	log.Println("mdns: advertisement stopped")
}

// ParsePort extracts the port number from a "host:port" address.
func ParsePort(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

// ServiceID returns a formatted string for logging/display.
func (a *Advertiser) ServiceID() string {
	return fmt.Sprintf("%s.%s:%d", a.opts.ServiceType, a.opts.Domain, a.opts.Port)
}

func formatTXT(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
