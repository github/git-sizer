package counts

import (
	"fmt"
	"math"
)

type Prefix struct {
	Name       string
	Multiplier uint64
}

type Humaner interface {
	Human([]Prefix, string) (string, string)
	ToUint64() uint64
}

var MetricPrefixes []Prefix

func init() {
	MetricPrefixes = []Prefix{
		{"", 1},
		{"k", 1e3},
		{"M", 1e6},
		{"G", 1e9},
		{"T", 1e12},
		{"P", 1e15},
	}
}

var BinaryPrefixes []Prefix

func init() {
	BinaryPrefixes = []Prefix{
		{"", 1 << (10 * 0)},
		{"Ki", 1 << (10 * 1)},
		{"Mi", 1 << (10 * 2)},
		{"Gi", 1 << (10 * 3)},
		{"Ti", 1 << (10 * 4)},
		{"Pi", 1 << (10 * 5)},
	}
}

// Format values, aligned, in `len(unit) + 10` or fewer characters
// (except for extremely large numbers).
func Human(n uint64, prefixes []Prefix, unit string) (string, string) {
	prefix := prefixes[0]
	wholePart := n
	for _, p := range prefixes {
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

func (n Count32) Human(prefixes []Prefix, unit string) (string, string) {
	if n == math.MaxUint32 {
		return "∞", unit
	} else {
		return Human(uint64(n), prefixes, unit)
	}
}

func (n Count64) Human(prefixes []Prefix, unit string) (string, string) {
	if n == math.MaxUint64 {
		return "∞", unit
	} else {
		return Human(uint64(n), prefixes, unit)
	}
}
