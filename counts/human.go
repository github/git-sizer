package counts

import (
	"fmt"
)

// A quantity that can be made human-readable using Human().
type Humanable interface {
	// Return the value as a uint64, and a boolean telling whether it
	// overflowed.
	ToUint64() (uint64, bool)
}

// An object that can format a Humanable in human-readable format.
type Humaner struct {
	name     string
	prefixes []Prefix
}

type Prefix struct {
	Name       string
	Multiplier uint64
}

var Metric = Humaner{
	name: "metric",
	prefixes: []Prefix{
		{"", 1},
		{"k", 1e3},
		{"M", 1e6},
		{"G", 1e9},
		{"T", 1e12},
		{"P", 1e15},
	},
}

var Binary = Humaner{
	name: "binary",
	prefixes: []Prefix{
		{"", 1 << (10 * 0)},
		{"Ki", 1 << (10 * 1)},
		{"Mi", 1 << (10 * 2)},
		{"Gi", 1 << (10 * 3)},
		{"Ti", 1 << (10 * 4)},
		{"Pi", 1 << (10 * 5)},
	},
}

func (h *Humaner) Name() string {
	return h.name
}

// Format n, aligned, in `len(unit) + 10` or fewer characters (except
// for extremely large numbers).
func (h *Humaner) FormatNumber(n uint64, unit string) (string, string) {
	prefix := h.prefixes[0]

	wholePart := n
	for _, p := range h.prefixes {
		w := n / p.Multiplier
		if w >= 1 {
			wholePart = w
			prefix = p
		}
	}

	if prefix.Multiplier == 1 {
		return fmt.Sprintf("%d", n), unit
	} else {
		mantissa := float64(n) / float64(prefix.Multiplier)
		var format string

		if wholePart >= 100 {
			// `mantissa` can actually be up to 1023.999.
			format = "%.0f"
		} else if wholePart >= 10 {
			format = "%.1f"
		} else {
			format = "%.2f"
		}
		return fmt.Sprintf(format, mantissa), prefix.Name + unit
	}
}

// Format values, aligned, in `len(unit) + 10` or fewer characters
// (except for extremely large numbers).
func (h *Humaner) Format(value Humanable, unit string) (string, string) {
	n, overflow := value.ToUint64()
	if overflow {
		return "∞", unit
	}

	return h.FormatNumber(n, unit)
}
