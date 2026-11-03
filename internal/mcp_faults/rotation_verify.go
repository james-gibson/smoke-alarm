package mcp_faults

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"time"
)

// RotationPolicy is the cluster's ceiling on grace. A node may ask for less; a
// node that grants itself more is describing a grace the cluster never agreed
// to, which is how a leaked stamp is kept alive past its rotation.
type RotationPolicy struct {
	MaxGrace time.Duration
}

// DefaultRotationPolicy is the ceiling the cluster enforces, regardless of what a
// node claims. Peers must honor the mirror's verdict, never a node's claim.
//
// Deliberately a function, not a package variable: a mutable global in this path
// would be a way to raise the ceiling without meaning to.
func DefaultRotationPolicy() RotationPolicy {
	return RotationPolicy{MaxGrace: DefaultGraceWindow}
}

// EffectiveGrace caps a claimed grace at the policy ceiling.
func (p RotationPolicy) EffectiveGrace(claimed time.Duration) time.Duration {
	if claimed <= 0 {
		return 0
	}
	if claimed > p.MaxGrace {
		return p.MaxGrace
	}
	return claimed
}

// RotationClaim is what a node announces about a rotation it performed. Every
// field is untrusted: a compromised node can announce nothing, announce a
// rotation that did not happen, or attach a grace it was never given.
type RotationClaim struct {
	NodeID    string
	RotatedAt time.Time
	Grace     time.Duration
	Prefix    string
	RouteID   string // the inverse: canonical id of what this route *is*
}

// RouteRef names a logical route: the scope it serves and the tool it exposes.
//
// The cluster is deliberately absent, because clusters are replicas of a route
// rather than distinct routes: two clusters providing the same tool for the same
// project are one route with two hot paths. The scope is present because two
// projects providing a tool of the same name are *not* one route, however
// identical their names, and an identity that ignored the scope would let a claim
// about one project verify against the other.
//
// The tumbled prefix is absent for the same reason: the ref must survive the
// rotation it describes.
type RouteRef struct {
	Scope string // project (or session) the tool serves
	Tool  string
}

// ID is the inverse of the hot path: canonical over the definition, invariant
// across rotation and across replicas.
func (r RouteRef) ID(policy RotationPolicy) string {
	h := sha256.New()
	// Length-prefixed fields, so no two definitions concatenate to one encoding.
	writeField := func(hash hash.Hash, s string) {
		_, _ = fmt.Fprintf(hash, "%d:%s|", len(s), s)
	}
	writeField(h, "route")
	writeField(h, r.Scope)
	writeField(h, r.Tool)
	writeField(h, policy.MaxGrace.String())
	return hex.EncodeToString(h.Sum(nil))
}

// RouteRegistry is what the mirror expects to see: which logical routes exist and
// which nodes are replicas of each. Without it a mirror cannot tell a replica from
// a stranger announcing the same route name, which is the whole point of
// verifying a rotation against something other than the node's word.
type RouteRegistry struct {
	replicas map[string]map[string]struct{} // route id -> node ids
}

// NewRouteRegistry returns an empty registry.
func NewRouteRegistry() *RouteRegistry {
	return &RouteRegistry{replicas: map[string]map[string]struct{}{}}
}

// Register records nodeID as a replica of ref.
func (r *RouteRegistry) Register(ref RouteRef, policy RotationPolicy, nodeID string) {
	if r.replicas == nil {
		r.replicas = map[string]map[string]struct{}{}
	}
	id := ref.ID(policy)
	if r.replicas[id] == nil {
		r.replicas[id] = map[string]struct{}{}
	}
	r.replicas[id][nodeID] = struct{}{}
}

// Authorized reports whether nodeID is a known replica of ref.
func (r *RouteRegistry) Authorized(ref RouteRef, policy RotationPolicy, nodeID string) bool {
	if r == nil {
		return true // no replica policy configured
	}
	_, ok := r.replicas[ref.ID(policy)][nodeID]
	return ok
}

// Replicas lists the known replicas of ref.
func (r *RouteRegistry) Replicas(ref RouteRef, policy RotationPolicy) []string {
	if r == nil {
		return nil
	}
	ids := make([]string, 0, len(r.replicas[ref.ID(policy)]))
	for id := range r.replicas[ref.ID(policy)] {
		ids = append(ids, id)
	}
	return ids
}

// A node's rotation behavior, as the mirror reads it.
const (
	RotationAnnounced            = "announced"
	RotationUnannounced          = "unannounced-rotation"
	RotationMismatched           = "mismatched-rotation"
	RotationOverclaimedGrace     = "overclaimed-grace"
	RotationClaimedWithoutRotate = "claimed-without-rotation"
	RotationIdentityMismatch     = "identity-mismatch"
	RotationUnauthorizedReplica  = "unauthorized-replica"
)

// RotationVerdict is the mirror's finding, plus the grace the cluster will
// actually honor for this rotation.
type RotationVerdict struct {
	State    string
	Grace    time.Duration // policy-capped; the only grace a peer may apply
	Observed time.Time
	Claimed  time.Time
	RouteID  string // the id the mirror derived, not the one the node supplied
}

// VerifyRotation compares what a node claimed against what the mirror derived and
// observed for itself. observedAt is the mirror's own reading, not the node's
// word; a zero value means the mirror saw no rotation. requested is the route the
// mirror is watching - the id is derived from it here, never taken from the claim.
//
// The point of the check is that the observed node cannot be its own witness: it
// will not announce a rotation it did not perform, a compromised one will not
// announce the rotation it did, and neither can redefine what the route is.
func VerifyRotation(
	requested RouteRef,
	registry *RouteRegistry,
	observedAt time.Time,
	claim *RotationClaim,
	policy RotationPolicy,
) RotationVerdict {
	routeID := requested.ID(policy)

	if observedAt.IsZero() {
		if claim != nil {
			// The node claims to have rotated; the mirror saw nothing.
			return RotationVerdict{
				State:    RotationClaimedWithoutRotate,
				Grace:    0,
				Claimed:  claim.RotatedAt,
				Observed: observedAt,
				RouteID:  routeID,
			}
		}
		return RotationVerdict{State: RotationUnannounced, Grace: 0, RouteID: routeID}
	}

	if claim != nil && claim.RouteID != routeID {
		// The claim describes a different route than the one the mirror is
		// watching: a forged definition, another project's route, or the wrong
		// node's announcement.
		return RotationVerdict{
			State:    RotationIdentityMismatch,
			Grace:    0,
			Observed: observedAt,
			Claimed:  claim.RotatedAt,
			RouteID:  routeID,
		}
	}

	if claim != nil && !registry.Authorized(requested, policy, claim.NodeID) {
		// Right route, wrong voice: a node the mirror does not hold as a replica.
		return RotationVerdict{
			State:    RotationUnauthorizedReplica,
			Grace:    0,
			Observed: observedAt,
			Claimed:  claim.RotatedAt,
			RouteID:  routeID,
		}
	}

	if claim == nil {
		// Rotation observed, no announcement: either alerting is configured off
		// or the node is compromised. Either way the mirror records it, and the
		// cluster keeps the ceiling rather than the node's silence.
		return RotationVerdict{
			State:    RotationUnannounced,
			Grace:    policy.EffectiveGrace(policy.MaxGrace),
			Observed: observedAt,
			RouteID:  routeID,
		}
	}

	if !claim.RotatedAt.Equal(observedAt) {
		return RotationVerdict{
			State:    RotationMismatched,
			Grace:    policy.EffectiveGrace(claim.Grace),
			Observed: observedAt,
			Claimed:  claim.RotatedAt,
			RouteID:  routeID,
		}
	}

	if claim.Grace > policy.MaxGrace {
		return RotationVerdict{
			State:    RotationOverclaimedGrace,
			Grace:    policy.EffectiveGrace(claim.Grace),
			Observed: observedAt,
			Claimed:  claim.RotatedAt,
			RouteID:  routeID,
		}
	}

	return RotationVerdict{
		State:    RotationAnnounced,
		Grace:    policy.EffectiveGrace(claim.Grace),
		Observed: observedAt,
		Claimed:  claim.RotatedAt,
		RouteID:  routeID,
	}
}

// RouteID is this route's canonical identity: derived from the scope and tool it
// serves, invariant across rotation. Two clusters providing the same tool for the
// same project agree on it however differently their hot paths have tumbled; two
// projects sharing a tool name do not.
func (tp *ToolProxy) RouteID() string {
	return tp.Ref().ID(DefaultRotationPolicy())
}

// Ref returns the logical route this proxy serves.
func (tp *ToolProxy) Ref() RouteRef {
	return RouteRef{Scope: tp.Scope, Tool: tp.ToolName}
}

// AdoptLedger takes on another replica's stamp history. A node promoted into a
// route must attribute stamps its predecessor issued: without the ledger, every
// client still holding the previous holder's stamp scores as fuzz instead of as
// attributable staleness, and a failover becomes a false-positive storm.
//
// This is the second reason the mirror is useful: verifying a route means
// observing its rotations, and what it observed is exactly the ledger a promoted
// standby needs. Returns the number of stamps taken on.
func (tp *ToolProxy) AdoptLedger(from *ToolProxy) int {
	if from == nil {
		return 0
	}

	// Copy the predecessor's ledger under both locks: a standby takes on an
	// instance that is still serving, so neither side may see a half-adopted
	// history.
	from.mu.Lock()
	incoming := make([]SkillRoute, len(from.Skills))
	copy(incoming, from.Skills)
	fromRotated := from.LastRotation
	from.mu.Unlock()

	tp.mu.Lock()
	defer tp.mu.Unlock()

	adopted := 0
	for _, candidate := range incoming {
		seen := false
		for _, existing := range tp.Skills {
			if existing.Stamp == candidate.Stamp {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		tp.Skills = append(tp.Skills, candidate)
		adopted++
	}
	if fromRotated.After(tp.LastRotation) {
		tp.LastRotation = fromRotated
	}
	return adopted
}

// Announcement returns what this node would tell its neighbors about its last
// rotation, or nil if it has never rotated. It is a claim, not a fact.
func (tp *ToolProxy) Announcement() *RotationClaim {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if tp.LastRotation.IsZero() {
		return nil
	}
	return &RotationClaim{
		NodeID:    tp.ProxyID,
		RotatedAt: tp.LastRotation,
		Grace:     tp.grace(),
		Prefix:    tp.Prefix,
		RouteID:   tp.RouteID(),
	}
}
