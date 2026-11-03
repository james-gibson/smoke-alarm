package mcp_faults

import (
	"sort"
	"time"
)

// RotationCommit is the committer's decision for one route. Node A and node B are
// redundant stores observed by node C; C commits, and the stores hold the result.
//
// The direction matters: because C is authoritative, the redundancy is not there to
// outvote C - it is there to catch C. Each store keeps its own observation, and a
// commit that disagrees with a store's observation is the discrepancy that a
// second store exists to produce.
type RotationCommit struct {
	// CommittedBy is the observer that decided, and At is when.
	CommittedBy string
	At          time.Time

	// RouteID is the derived identity of the route this commit is about, so a
	// commit cannot be replayed against another project's route.
	RouteID string

	// Rotations is the accepted rotation per node. A node absent from this map was
	// rejected or not observed at all.
	Rotations map[string]time.Time

	// Grace is the smallest accepted grace: one node claiming more cannot widen the
	// window for everyone on the route.
	Grace time.Duration

	// Rejected records why a node's announcement was not committed.
	Rejected []Rejection
}

// Rejection is a node whose announcement the committer would not commit.
type Rejection struct {
	NodeID string
	State  string // the VerifyRotation state that disqualified it
	Grace  time.Duration
}

// Accepted reports whether nodeID's rotation is in the commit.
func (c RotationCommit) Accepted(nodeID string) bool {
	_, ok := c.Rotations[nodeID]
	return ok
}

// The discrepancies a redundant store can raise against a commit.
const (
	CommitAgrees             = "agrees"
	CommitInventedRotation   = "committed-without-store-observation"
	CommitMismatchedRotation = "committed-different-rotation"
	CommitOmittedRotation    = "omitted-observed-rotation"
)

// Check is a redundant store comparing a commit against its own observation. This
// is the only check on the committer, so it must be run by the store, on its own
// record, not on anything the committer supplied.
func (c RotationCommit) Check(nodeID string, storeSaw time.Time) (bool, string) {
	committed, inCommit := c.Rotations[nodeID]

	switch {
	case inCommit && storeSaw.IsZero():
		return false, CommitInventedRotation
	case inCommit && !committed.Equal(storeSaw):
		return false, CommitMismatchedRotation
	case !inCommit && !storeSaw.IsZero():
		return false, CommitOmittedRotation
	default:
		return true, CommitAgrees
	}
}

// Commit decides one route from the committer's own observations of its stores.
//
// observed maps node -> the rotation the committer saw before application; claims
// maps node -> what each node announced. Each claim is verified independently, so a
// compromised store is rejected on its own evidence rather than by majority. The
// result is sorted, so the same observations always produce the same commit.
func Commit(
	ref RouteRef,
	committedBy string,
	at time.Time,
	policy RotationPolicy,
	observed map[string]time.Time,
	claims map[string]*RotationClaim,
) RotationCommit {
	nodes := make([]string, 0, len(observed))
	for node := range observed {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	commit := RotationCommit{
		CommittedBy: committedBy,
		At:          at,
		RouteID:     ref.ID(policy),
		Rotations:   map[string]time.Time{},
		Rejected:    []Rejection{},
	}

	graceSet := false
	for _, node := range nodes {
		verdict := VerifyRotation(ref, nil, observed[node], claims[node], policy)

		if verdict.State != RotationAnnounced {
			commit.Rejected = append(commit.Rejected, Rejection{
				NodeID: node,
				State:  verdict.State,
				Grace:  verdict.Grace,
			})
			continue
		}

		commit.Rotations[node] = verdict.Observed
		if !graceSet || verdict.Grace < commit.Grace {
			commit.Grace = verdict.Grace
			graceSet = true
		}
	}

	return commit
}
