package sizes

import (
	"math"
)

// A count of something, capped at math.MaxUint64.
type Count uint64

// Return the sum of two Counts, capped at math.MaxUint64.
func (n1 Count) Plus(n2 Count) Count {
	n := n1 + n2
	if n < n1 {
		// Overflow
		return math.MaxUint64
	}
	return n
}

// Increment `*n1` by `n2`, capped at math.MaxUint64.
func (n1 *Count) Increment(n2 Count) {
	*n1 = n1.Plus(n2)
}

// Adjust `*n1` to be `max(*n1, n2)`.
func (n1 *Count) AdjustMax(n2 Count) {
	if n2 > *n1 {
		*n1 = n2
	}
}
