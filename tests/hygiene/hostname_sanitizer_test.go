package hygiene_test

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// leakedHostPattern matches identifiers that must never be committed again:
// the leaked personal domain (any subdomain of it) and the developer machine
// mDNS name that shipped alongside it.
var leakedHostPattern = regexp.MustCompile(`(datawhore\.house|riversong-4\.local)`)

// ipv4Pattern finds IPv4 literals so they can be classified properly
// (a plain regex for RFC 1918 ranges also matches version strings).
var ipv4Pattern = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)

// scanExtensions are the text formats that ship configuration or docs.
var scanExtensions = map[string]bool{
	".yaml": true, ".yml": true, ".json": true, ".toml": true,
	".conf": true, ".env": true, ".txt": true, ".md": true, ".sh": true,
}

// TestNoInternalHostnames guards samples, acceptance configs and docs against
// leaking private infrastructure (issue #16).
func TestNoInternalHostnames(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !scanExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		return scanFile(t, path)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func scanFile(t *testing.T, path string) error {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	const maxTokenSize = 1024 * 1024
	scanner.Buffer(make([]byte, maxTokenSize), maxTokenSize)
	for i := 1; scanner.Scan(); i++ {
		line := scanner.Text()
		if leakedHostPattern.MatchString(line) {
			t.Errorf("%s:%d: internal hostname must not be committed: %s", path, i, line)
		}
		for _, candidate := range ipv4Pattern.FindAllString(line, -1) {
			if ip := net.ParseIP(candidate); ip != nil && ip.IsPrivate() {
				t.Errorf("%s:%d: private address must not be committed: %s", path, i, line)
			}
		}
	}
	return scanner.Err()
}
