package federation

import (
	"fmt"
	"time"

	"github.com/james-gibson/isotope"
)

// MeshMemberRung is the 42i trust rung granted to a peer that belongs to the
// same federation cluster. It maps to "inter-peer registration observed":
// membership in the shared cluster identity is the evidence, so a peer is
// trusted by construction instead of being re-verified against the discovery
// registry.
const MeshMemberRung = 4

// IsotopeForMember derives the isotope record for a federation instance. The
// record identifies a mesh member to the alarm's isotope list. Callers should
// gate on SameCluster before registering the result when trust matters; an
// empty instance record returns ok=false.
func IsotopeForMember(rec InstanceRecord) (isotope.Record, bool) {
	if rec.ID == "" {
		return isotope.Record{}, false
	}
	return isotope.Record{
		Name:         rec.ID,
		Role:         "alarm",
		Endpoint:     memberEndpoint(rec),
		Protocol:     "http",
		TrustRung:    MeshMemberRung,
		RungName:     rungName(MeshMemberRung),
		RegisteredAt: time.Now().UTC(),
	}, true
}

// SameCluster reports whether rec belongs to the same federation cluster as
// self. An empty cluster identity never matches, so self-trust stays opt-in.
func SameCluster(self, rec InstanceRecord) bool {
	return self.ClusterID != "" && self.ClusterID == rec.ClusterID
}

// IsotopeSink receives the internal isotopes derived from mesh membership. The
// health server satisfies it; tests can substitute a recorder.
type IsotopeSink interface {
	UpsertIsotope(isotope.Record)
	RemoveIsotope(name string)
}

// MirrorMember folds a membership event into the isotope sink: a same-cluster
// peer is registered as an isotope (trusted by construction) and a departed
// peer is dropped. Events for peers outside the local cluster are ignored, as
// are all events when self carries no cluster identity.
func MirrorMember(sink IsotopeSink, self InstanceRecord, ev Event) {
	switch ev.Type {
	case EventAdded, EventUpdated:
		if !SameCluster(self, ev.Record) {
			return
		}
		if rec, ok := IsotopeForMember(ev.Record); ok {
			sink.UpsertIsotope(rec)
		}
	case EventRemoved:
		sink.RemoveIsotope(ev.Record.ID)
	}
}

// memberEndpoint returns the reachable federation endpoint for a peer, falling
// back to an empty string when the record lacks addressing information.
func memberEndpoint(rec InstanceRecord) string {
	if rec.Hostname == "" || rec.Port == 0 {
		return ""
	}
	return fmt.Sprintf("http://%s:%d", rec.Hostname, rec.Port)
}

// rungName resolves the human-readable name for a rung from the canonical
// isotope thresholds.
func rungName(rung int) string {
	for _, rt := range isotope.DefaultRungThresholds {
		if rt.Rung == rung {
			return rt.Name
		}
	}
	return ""
}
