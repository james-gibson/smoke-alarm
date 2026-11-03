package mcp_faults

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// RouteSchedule is the pre-planned rotation a route is derived from, rather than
// announced by.
//
// A route prefix used to be drawn once, at startup, which made the route
// un-enumerable and permanent at once: entropy prices guessing, and nothing
// priced replay. A prefix observed once - a proxy log, a cached capability list,
// a hop that read the body, a pre-stamped skill artifact - stayed a valid route
// for the life of the process.
//
// The fix is to make the prefix a function of (instance nonce, epoch) instead of
// a draw. Entropy is unchanged for a caller who does not hold the nonce: the
// route is still un-enumerable. What changes is that a listed prefix belongs to
// one epoch and stops being the route when the epoch closes, so a leaked map is
// worth one rotation period rather than one process lifetime.
//
// The schedule is what makes the trail pre-planned rather than announced. Both
// sides derive the same prefix at the same epoch from (nonce, base, origin,
// period), so the route moves without anyone saying where it went: silence is
// the mechanism, not a missing notification. A neighbor that has not been told
// is behind, not wrong, and rotation is precisely the event that distinguishes
// those two readings.
type RouteSchedule struct {
	// Nonce is the instance secret. Whoever holds it can derive every epoch's
	// prefix; whoever does not cannot derive any. It is the safe map.
	Nonce []byte

	// Base is the epoch that was current at Origin. Rotation re-bases the
	// schedule so that an out-of-band rotation (the mirror's, say) keeps a single
	// timeline instead of leaving the clock describing an obsolete one.
	Base int64

	// Origin is when Base opened.
	Origin time.Time

	// Period is how long one epoch lasts. Zero disables the schedule: a route
	// with no period only moves when someone calls Rotate.
	Period time.Duration
}

// RetiredEpochWindow is how many closed epochs stay attributable.
//
// The ledger is bounded on purpose. A prefix from further back than this is no
// longer a refresh problem we are prepared to recognize, so it falls to the
// enumeration tier - which is the honest reading: a capability we stopped
// tracking is a capability we can no longer vouch for. Eight epochs is
// comfortably longer than any notification latency and still inside a bounded
// replay window.
const RetiredEpochWindow = 8

// newNonce draws the instance secret every epoch's prefix is derived from.
//
// crypto/rand.Read never returns an error: it panics on platforms where the
// kernel cannot supply entropy, so there is nothing to fall back to and nothing
// to check. A route with a zero nonce would be derivable by anyone, so this is
// the one draw that must not be a guess.
func newNonce() []byte {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	return nonce
}

// NewRouteSchedule opens epoch zero at origin and rotates every period.
func NewRouteSchedule(origin time.Time, period time.Duration) RouteSchedule {
	return RouteSchedule{
		Nonce:  newNonce(),
		Base:   0,
		Origin: origin,
		Period: period,
	}
}

// Valid reports whether the schedule can derive a prefix at all.
func (s RouteSchedule) Valid() bool {
	return len(s.Nonce) > 0 && s.Period > 0
}

// EpochAt reports the epoch the schedule says is current at now.
//
// A clock behind Origin has not left epoch Base yet, so it reads Base rather than
// a negative epoch: skew backwards is the same as no elapsed time.
func (s RouteSchedule) EpochAt(now time.Time) int64 {
	if !s.Valid() || now.Before(s.Origin) {
		return s.Base
	}
	return s.Base + int64(now.Sub(s.Origin)/s.Period)
}

// EpochEnd reports when epoch closes on the schedule, rather than at whatever
// moment the node noticed. A lazy rotation must price staleness from the boundary
// the epoch actually crossed: stamping the retirement with "now" would hand a
// leaked prefix a free call after every boundary, because the retire and the call
// would always be simultaneous.
func (s RouteSchedule) EpochEnd(epoch int64) time.Time {
	if !s.Valid() {
		return s.Origin
	}
	return s.Origin.Add(time.Duration(epoch-s.Base+1) * s.Period)
}

// PrefixAt derives the route prefix for epoch.
//
// The whole 32-bit width of the old drawn prefix is preserved, so enumeration
// costs what it always cost. What is gone is the permanence: this value is the
// route for one epoch, and the derivation from a later epoch is independent of
// it.
func (s RouteSchedule) PrefixAt(epoch int64) string {
	if len(s.Nonce) == 0 {
		return ""
	}

	mac := hmac.New(sha256.New, s.Nonce)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(epoch))
	_, _ = mac.Write(counter[:])

	// Same shape as the drawn prefix it replaces: "honeypot_" + 8 hex + "_". The
	// marker is kept for log readability; decoys that carry no marker derive
	// through this same path.
	sum := mac.Sum(nil)
	return fmt.Sprintf("honeypot_%s_", hex.EncodeToString(sum[:4]))
}

// RetiredRoute is one prefix this instance used to present. It no longer
// resolves, but a call carrying it is still attributable to us: that is the
// difference between a client that has not caught up and one that is guessing.
type RetiredRoute struct {
	Epoch     int64
	Prefix    string
	RetiredAt time.Time
}
