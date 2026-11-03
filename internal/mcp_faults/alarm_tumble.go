package mcp_faults

// AlarmKind is how an alarm was addressed. A plain alias names the sender; an
// isotoped alarm carries its parental chain, which is what buys it tolerance.
type AlarmKind int

const (
	// PlainAlias is an alarm addressed by name. It proves nothing about descent, so
	// it must name the current chain.
	PlainAlias AlarmKind = iota
	// Isotoped carries the parental rotation, so an alarm one rotation behind is still
	// provably descended from an accepted chain.
	Isotoped
)

// String names the kind for logs.
func (k AlarmKind) String() string {
	if k == Isotoped {
		return "isotoped"
	}
	return "plain-alias"
}

// AlarmTumble is one ingress's conditional rotation chain.
//
// Accepting an alarm advances the chain: the ingress tumbles its own alarm route, so
// the cluster can tell inbound spam from traffic that is keeping up. The deal is the
// whole mechanism - behave, and your alarm is processed; a flood is not behaving.
//
// The load-bearing property is that a refused alarm does not advance the chain. Spam
// therefore cannot force rotations, cannot republish the filter, and cannot burn any
// skills: the flood costs the claimant and nothing else.
type AlarmTumble struct {
	name      string
	chain     int64
	tolerance int // rotations an alarm may fall behind, per kind
	accepted  int
	refused   int
	rotated   int
	tooNew    int
}

// NewAlarmTumble starts a chain at zero. tolerance must be 0 or 1: a plain alias
// cannot prove descent, so it may not lag at all.
func NewAlarmTumble(name string, tolerance int) *AlarmTumble {
	if tolerance < 0 {
		tolerance = 0
	}
	return &AlarmTumble{name: name, tolerance: tolerance}
}

// Chain is the current rotation the ingress will accept.
func (t *AlarmTumble) Chain() int64 { return t.chain }

// Offer presents an alarm that references chain position ref.
//
// Accepted alarms advance the chain. Refused alarms change nothing, which is what
// makes the flood inert.
func (t *AlarmTumble) Offer(ref int64, kind AlarmKind) (accepted bool, skew int64) {
	allowed := t.tolerance
	if kind == PlainAlias {
		allowed = 0 // no provenance, so no lag
	}

	skew = t.chain - ref
	switch {
	case skew < 0:
		// From a chain this ingress has not reached: a guess, or a replay of another
		// ingress's chain.
		t.refused++
		t.tooNew++
		return false, skew
	case skew > int64(allowed):
		// Behind the tumble: the caller is not keeping up with the chain it was given.
		t.refused++
		return false, skew
	default:
		t.accepted++
		t.chain++
		t.rotated++
		return true, skew
	}
}

// Accepted is how many alarms advanced the chain.
func (t *AlarmTumble) Accepted() int { return t.accepted }

// Refused is how many alarms changed nothing.
func (t *AlarmTumble) Refused() int { return t.refused }

// Rotated is how many times the chain advanced: one per accepted alarm.
func (t *AlarmTumble) Rotated() int { return t.rotated }

// TooNew is how many alarms referenced a chain the ingress never issued.
func (t *AlarmTumble) TooNew() int { return t.tooNew }
