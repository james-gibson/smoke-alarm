package federation

import (
	"strings"
	"time"
)

// stringsTrim normalizes whitespace trimming for ID/endpoint fields.
func stringsTrim(v string) string {
	return strings.TrimSpace(v)
}

// durationOrDefault attempts to parse a duration string and falls back to the
// provided default when parsing fails or the value is non-positive.
func durationOrDefault(raw string, fallback time.Duration) time.Duration {
	raw = stringsTrim(raw)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// clampDuration ensures val stays within [minD, maxD]. Zero or negative bounds
// disable clamping on that side.
func clampDuration(val, minD, maxD time.Duration) time.Duration {
	if minD > 0 && val < minD {
		return minD
	}
	if maxD > 0 && val > maxD {
		return maxD
	}
	return val
}
