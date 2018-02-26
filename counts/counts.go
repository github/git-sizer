package counts

import (
	"math"
)

// A count of something, capped at math.MaxUint32.
type Count32 uint32

func NewCount32(n uint64) Count32 {
	if n > math.MaxUint32 {
		return Count32(math.MaxUint32)
	}
	return Count32(n)
}

func (n Count32) ToUint64() uint64 {
	return uint64(n)
}

// Return the sum of two Count32s, capped at math.MaxUint32.
func (n1 Count32) Plus(n2 Count32) Count32 {
	n := n1 + n2
	if n < n1 {
		// Overflow
		return math.MaxUint32
	}
	return n
}

// Increment `*n1` by `n2`, capped at math.MaxUint32.
func (n1 *Count32) Increment(n2 Count32) {
	*n1 = n1.Plus(n2)
}

// Adjust `*n1` to be `max(*n1, n2)` and return true iff n2 was
// bigger. Favor `*n1` if they are equal.
func (n1 *Count32) AdjustMaxIfNecessary(n2 Count32) bool {
	if n2 > *n1 {
		*n1 = n2
		return true
	} else {
		return false
	}
}

// Adjust `*n1` to be `max(*n1, n2)` and return true iff n2 was
// bigger. Favor `n2` if they are equal.
func (n1 *Count32) AdjustMaxIfPossible(n2 Count32) bool {
	if n2 >= *n1 {
		*n1 = n2
		return true
	} else {
		return false
	}
}

// A count of something, capped at math.MaxUint64.
type Count64 uint64

func NewCount64(n uint64) Count64 {
	return Count64(n)
}

func (n Count64) ToUint64() uint64 {
	return uint64(n)
}

// Return the sum of two Count64s, capped at math.MaxUint64.
func (n1 Count64) Plus(n2 Count64) Count64 {
	n := n1 + n2
	if n < n1 {
		// Overflow
		return math.MaxUint64
	}
	return n
}

// Increment `*n1` by `n2`, capped at math.MaxUint64.
func (n1 *Count64) Increment(n2 Count64) {
	*n1 = n1.Plus(n2)
}

// Adjust `*n1` to be `max(*n1, n2)` and return true iff n2 was
// bigger. Favor `*n1` if they are equal.
func (n1 *Count64) AdjustMaxIfNecessary(n2 Count64) bool {
	if n2 > *n1 {
		*n1 = n2
		return true
	} else {
		return false
	}
}

// Adjust `*n1` to be `max(*n1, n2)` and return true iff n2 was
// bigger. Favor `n2` if they are equal.
func (n1 *Count64) AdjustMaxIfPossible(n2 Count64) bool {
	if n2 > *n1 {
		*n1 = n2
		return true
	} else {
		return false
	}
}
