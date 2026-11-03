package mcp_faults

// Verdict is the three states an item can be in. Two are not enough: an item that
// was never examined is not innocent, and a boolean would report it as such.
type Verdict string

const (
	// VerdictTrusted is an item that was admitted, resolved, and agreed.
	VerdictTrusted Verdict = "trusted"
	// VerdictUntrusted is an item that was admitted, resolved, and refused.
	VerdictUntrusted Verdict = "untrusted"
	// VerdictUnexamined is an item never admitted to a slot, or admitted and
	// never resolved. Absence of evidence, recorded as absence - never as innocence.
	VerdictUnexamined Verdict = "unexamined"
)

// Classify maps what happened to one item onto its state. The short circuit on
// admission is the whole point: an unexamined item cannot be untrusted, and it
// cannot be trusted either.
func Classify(admitted, trusted bool) Verdict {
	switch {
	case !admitted:
		return VerdictUnexamined
	case trusted:
		return VerdictTrusted
	default:
		return VerdictUntrusted
	}
}

// Resolved reports whether the verdict is a finding at all.
func (v Verdict) Resolved() bool { return v != VerdictUnexamined }
