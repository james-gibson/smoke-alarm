package mcp_faults

import "fmt"

// RouteKey is the triple every check is made against: an address that several
// claimants may hold by design, the claimant speaking, and the epoch.
//
// The address alone is never a key. Because it is shared on purpose, it carries no
// trust; the claimant is what a receipt is bound to, and the epoch is what makes a
// stale receipt distinguishable from a forged one. Wall-clock time is deliberately
// absent: an epoch is a rotation sequence, so two rotations inside one clock tick
// stay distinguishable.
type RouteKey struct {
	Address  string // derived address of the route (shared, zero trust)
	Claimant string // the node speaking for it
	Epoch    int64  // rotation sequence, not a timestamp
}

// String renders the triple for logs and queue identities.
func (k RouteKey) String() string {
	return fmt.Sprintf("%s@%s#%d", k.Address, k.Claimant, k.Epoch)
}

// Matches reports whether two keys name the same (address, claimant, epoch).
func (k RouteKey) Matches(other RouteKey) bool { return k == other }
