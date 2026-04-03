package dynamicconfig

import (
	"strings"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

func TestSanitizeID(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"Alpha Beta":                "alpha-beta",
		"Alpha___Beta!!!":           "alpha-beta",
		"  MIXED   Case/With\\Ops ": "mixed-case-with-ops",
		"":                          "dynamic-config",
		"***":                       "dynamic-config",
	}

	for input, want := range cases {
		if got := sanitizeID(input); got != want {
			t.Fatalf("sanitizeID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMakeStableTargetIDDeterministic(t *testing.T) {
	t.Parallel()

	target := targets.Target{
		Protocol:  targets.ProtocolMCP,
		Transport: targets.TransportHTTP,
		Endpoint:  "https://example.com/mcp",
	}

	first := makeStableTargetID(target)
	second := makeStableTargetID(target)

	if first != second {
		t.Fatalf("makeStableTargetID should be deterministic, got %q and %q", first, second)
	}

	target.Endpoint = "https://example.com/acp"
	if third := makeStableTargetID(target); third == first {
		t.Fatalf("expected different endpoint to produce different stable ID")
	}
}

func TestStoreMakeIDRespectsRequireUniqueIDs(t *testing.T) {
	t.Parallel()

	rec := discovery.Record{
		Source: "llms_txt",
		Target: targets.Target{
			ID:        "My Cool Target",
			Protocol:  targets.ProtocolMCP,
			Transport: targets.TransportHTTP,
			Endpoint:  "https://example.com/mcp",
		},
	}

	store := NewStore(StoreOptions{
		Directory:        t.TempDir(),
		Formats:          []string{"json"},
		RequireUniqueIDs: false,
		Now:              func() time.Time { return time.Unix(0, 0) },
	})

	id := store.makeID(rec)
	if id != "my-cool-target" {
		t.Fatalf("expected sanitized base ID, got %q", id)
	}

	uniqueStore := NewStore(StoreOptions{
		Directory:        t.TempDir(),
		Formats:          []string{"json"},
		RequireUniqueIDs: true,
		Now:              func() time.Time { return time.Unix(0, 0) },
	})

	uniqueID := uniqueStore.makeID(rec)
	if !strings.HasPrefix(uniqueID, "my-cool-target-") {
		t.Fatalf("expected unique ID to start with sanitized base, got %q", uniqueID)
	}

	suffix := strings.TrimPrefix(uniqueID, "my-cool-target-")
	if len(suffix) != 10 {
		t.Fatalf("expected short hash suffix length 10, got %d (%q)", len(suffix), suffix)
	}
	if strings.Contains(suffix, "-") {
		t.Fatalf("expected suffix to be continuous alphanumeric characters, got %q", suffix)
	}
}
