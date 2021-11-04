package counts

import (
	"math"
)

// Count32 is a count of something, capped at math.MaxUint32.
type Count32 uint32

// NewCount32 initializes a Count32 from a uint64, capped at
// math.MaxUint32.
func NewCount32(n uint64) Count32 {
	if n > math.MaxUint32 {
		return Count32(math.MaxUint32)
	}
	return Count32(n)
}

// ToUint64 returns the value of `n` as a `uint64`. If the value has
// overflowed, it returns `(math.MaxUint32, true)`.
func (n Count32) ToUint64() (uint64, bool) {
	return uint64(n), n == math.MaxUint32
}

// Plus returns the sum of two Count32s, capped at math.MaxUint32.
func (n1 Count32) Plus(n2 Count32) Count32 {
	n := n1 + n2
	if n < n1 {
		// Overflow
		return math.MaxUint32
	}
	return n
}

// Increment increases `*n1` by `n2`, capped at math.MaxUint32.
func (n1 *Count32) Increment(n2 Count32) {
	*n1 = n1.Plus(n2)
}

// AdjustMaxIfNecessary adjusts `*n1` to be `max(*n1, n2)`. Return
// true iff `n2` was greater than `*n1`.
func (n1 *Count32) AdjustMaxIfNecessary(n2 Count32) bool {
	if n2 <= *n1 {
		return false
	}

	*n1 = n2
	return true
}

// AdjustMaxIfPossible adjusts `*n1` to be `max(*n1, n2)`. Return true
// iff `n2` was greater than or equal to `*n1`.
func (n1 *Count32) AdjustMaxIfPossible(n2 Count32) bool {
	if n2 < *n1 {
		return false
	}

	*n1 = n2
	return true
}

// Count64 is a count of something, capped at math.MaxUint64.
type Count64 uint64

// NewCount64 initializes a Count64 from a uint64.
func NewCount64(n uint64) Count64 {
	return Count64(n)
}

// ToUint64 returns the value of `n` as a `uint64`. If the value has
// overflowed, it returns `(math.MaxUint64, true)`.
func (n Count64) ToUint64() (uint64, bool) {
	return uint64(n), n == math.MaxUint64
}

// Plus returns the sum of two Count64s, capped at math.MaxUint64.
func (n1 Count64) Plus(n2 Count64) Count64 {
	n := n1 + n2
	if n < n1 {
		// Overflow
		return math.MaxUint64
	}
	return n
}

// Increment increases `*n1` by `n2`, capped at math.MaxUint64.
func (n1 *Count64) Increment(n2 Count64) {
	*n1 = n1.Plus(n2)
}

// AdjustMaxIfNecessary adjusts `*n1` to be `max(*n1, n2)`. Return
// true iff `n2` was greater than `*n1`.
func (n1 *Count64) AdjustMaxIfNecessary(n2 Count64) bool {
	if n2 <= *n1 {
		return false
	}

	*n1 = n2
	return true
}

// AdjustMaxIfPossible adjusts `*n1` to be `max(*n1, n2)`. Return true
// iff `n2` was greater than or equal to `*n1`.
func (n1 *Count64) AdjustMaxIfPossible(n2 Count64) bool {
	if n2 <= *n1 {
		return false
	}

	*n1 = n2
	return true
}
