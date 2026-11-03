package mcp_faults

import (
	"hash/fnv"
	"math"
)

// RotationFilter is the sidechannel summary of one epoch's rotations.
//
// A fire alarm forces a rotation announcement, and rather than verify the whole herd
// item by item the committers share a compact set of parental rotations - a Bloom
// filter over rotation identities. Membership is cheap and has no false negatives
// for anything that was inserted, so a legitimate rotation is never refused. What it
// can do is admit an item that was never inserted, at a rate set by bits per item.
//
// That direction is deliberate: the hot path prefers availability, and the price of
// skipping per-item verification is paid as a false-admission rate rather than as a
// refused fire.
type RotationFilter struct {
	bits []uint64
	m    int // bit count
	k    int // hash count
	n    int // inserted items
}

// NewRotationFilter sizes a filter for capacity items at bitsPerItem bits each.
// Optimal hash count follows from the same ratio.
func NewRotationFilter(capacity, bitsPerItem int) *RotationFilter {
	if capacity < 1 {
		capacity = 1
	}
	if bitsPerItem < 1 {
		bitsPerItem = 1
	}

	m := capacity * bitsPerItem
	k := int(math.Round(float64(bitsPerItem) * math.Ln2))
	if k < 1 {
		k = 1
	}

	return &RotationFilter{
		bits: make([]uint64, (m+63)/64),
		m:    m,
		k:    k,
	}
}

// Add inserts a parental rotation identity.
func (f *RotationFilter) Add(id string) {
	h1, h2 := f.hashes(id)
	for i := 0; i < f.k; i++ {
		f.set((h1 + uint64(i)*h2) % uint64(f.m))
	}
	f.n++
}

// Present reports whether the rotation may have been published. False means
// certainly not; true means probably.
func (f *RotationFilter) Present(id string) bool {
	h1, h2 := f.hashes(id)
	for i := 0; i < f.k; i++ {
		if !f.test((h1 + uint64(i)*h2) % uint64(f.m)) {
			return false
		}
	}
	return true
}

// Len is how many rotations were inserted.
func (f *RotationFilter) Len() int { return f.n }

// Hashes is the number of probes a membership test costs.
func (f *RotationFilter) Hashes() int { return f.k }

// TheoryFalsePositive is the expected false-admission rate: (1-e^(-kn/m))^k.
func (f *RotationFilter) TheoryFalsePositive() float64 {
	if f.m == 0 {
		return 1
	}
	return math.Pow(1-math.Exp(-float64(f.k)*float64(f.n)/float64(f.m)), float64(f.k))
}

// BitsPerItem is the budget the filter was built to.
func (f *RotationFilter) BitsPerItem() float64 {
	if f.n == 0 {
		return math.Inf(1)
	}
	return float64(f.m) / float64(f.n)
}

func (f *RotationFilter) set(bit uint64) {
	f.bits[bit/64] |= 1 << (bit % 64)
}

func (f *RotationFilter) test(bit uint64) bool {
	return f.bits[bit/64]&(1<<(bit%64)) != 0
}

func (f *RotationFilter) hashes(id string) (h1, h2 uint64) {
	first := fnv.New64a()
	_, _ = first.Write([]byte(id))
	h1 = first.Sum64()

	second := fnv.New64a()
	_, _ = second.Write([]byte("salt:" + id))
	h2 = second.Sum64() | 1 // odd, so the probe sequence covers the filter
	return h1, h2
}

// BitsForFalseAdmission returns the bits per item needed to hold a per-epoch
// false-admission budget of `budget` across `queries` membership tests in that epoch.
//
// A Bloom filter's per-query rate compounds over the epoch, so the per-item budget is
// budget/queries, and bits/item = ln(p)/ln(0.6185) is the standard approximation.
func BitsForFalseAdmission(queries int, budget float64) int {
	if queries < 1 {
		queries = 1
	}
	perQuery := budget / float64(queries)
	if perQuery >= 0.6185 {
		return 1
	}
	bits := math.Log(perQuery) / math.Log(0.6185)

	return int(math.Ceil(bits))
}
