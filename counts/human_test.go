package counts_test

import (
	"testing"

	"github.com/github/git-sizer/counts"

	"github.com/stretchr/testify/assert"
)

type humanTest struct {
	n            uint64
	number, unit string
}

func TestMetric(t *testing.T) {
	assert := assert.New(t)

	for _, ht := range []humanTest{
		{0, "0", "cd"},
		{1, "1", "cd"},
		{999, "999", "cd"},
		{1000, "1.00", "kcd"},
		{1094, "1.09", "kcd"},
		{1096, "1.10", "kcd"},
		{9990, "9.99", "kcd"},
		{9999, "10.00", "kcd"}, // Not ideal, but ok
		{10000, "10.0", "kcd"},
		{10060, "10.1", "kcd"},
		{99999, "100.0", "kcd"}, // Not ideal, but ok
		{100000, "100", "kcd"},
		{999999, "1000", "kcd"}, // Not ideal, but ok
		{1000000, "1.00", "Mcd"},
		{9999999, "10.00", "Mcd"}, // Not ideal, but ok
		{10000000, "10.0", "Mcd"},
		{99999999, "100.0", "Mcd"}, // Not ideal, but ok
		{100000000, "100", "Mcd"},
		{999999999, "1000", "Mcd"}, // Not ideal, but ok
		{1000000000, "1.00", "Gcd"},
		{9999999999, "10.00", "Gcd"}, // Not ideal, but ok
		{10000000000, "10.0", "Gcd"},
		{99999999999, "100.0", "Gcd"}, // Not ideal, but ok
		{100000000000, "100", "Gcd"},
		{999999999999, "1000", "Gcd"}, // Not ideal, but ok
		{1000000000000, "1.00", "Tcd"},
		{999999999999999, "1000", "Tcd"}, // Not ideal, but ok
		{1000000000000000, "1.00", "Pcd"},
		{999999999999999999, "1000", "Pcd"},
		{1000000000000000000, "1000", "Pcd"},
		{9999999999999999999, "10000", "Pcd"},
		{10000000000000000000, "10000", "Pcd"},
		{12345678900000000000, "12346", "Pcd"}, // Not ideal, but ok
		{0xffffffffffffffff, "18447", "Pcd"},   // Not ideal, but ok
	} {
		number, unit := counts.Human(ht.n, counts.MetricPrefixes, "cd")
		assert.Equalf(ht.number, number, "Number for %d in metric", ht.n)
		assert.Equalf(ht.unit, unit, "Unit for %d in metric", ht.n)
		if ht.n < 0xffffffff {
			c := counts.NewCount32(ht.n)
			number, unit := c.Human(counts.MetricPrefixes, "cd")
			assert.Equalf(ht.number, number, "Number for Count32(%d) in metric", ht.n)
			assert.Equalf(ht.unit, unit, "Unit for Count32(%d) in metric", ht.n)
		}
		if ht.n < 0xffffffffffffffff {
			c := counts.NewCount64(ht.n)
			number, unit := c.Human(counts.MetricPrefixes, "cd")
			assert.Equalf(ht.number, number, "Number for Count64(%d) in metric", ht.n)
			assert.Equalf(ht.unit, unit, "Unit for Count64(%d) in metric", ht.n)
		}
	}
}

func TestBinary(t *testing.T) {
	assert := assert.New(t)

	for _, ht := range []humanTest{
		{0, "0", "B"},
		{1, "1", "B"},
		{1023, "1023", "B"},
		{1024, "1.00", "KiB"},
		{1234, "1.21", "KiB"},
		{1048575, "1024", "KiB"}, // Not ideal, but ok
		{1048576, "1.00", "MiB"},
		{1073741823, "1024", "MiB"}, // Not ideal, but ok
		{1073741824, "1.00", "GiB"},
		{1099511627775, "1024", "GiB"}, // Not ideal, but ok
		{1099511627776, "1.00", "TiB"},
		{1125899906842623, "1024", "TiB"}, // Not ideal, but ok
		{1125899906842624, "1.00", "PiB"},
		{1152921504606846975, "1024", "PiB"},
		{1152921504606846976, "1024", "PiB"},
		{0xffffffffffffffff, "16384", "PiB"},
	} {
		number, unit := counts.Human(ht.n, counts.BinaryPrefixes, "B")
		assert.Equalf(ht.number, number, "Number for %d in binary", ht.n)
		assert.Equalf(ht.unit, unit, "Unit for %d in binary", ht.n)
		if ht.n < 0xffffffff {
			c := counts.NewCount32(ht.n)
			number, unit := c.Human(counts.BinaryPrefixes, "B")
			assert.Equalf(ht.number, number, "Number for Count32(%d) in binary", ht.n)
			assert.Equalf(ht.unit, unit, "Unit for Count32(%d) in binary", ht.n)
		}
		if ht.n < 0xffffffffffffffff {
			c := counts.NewCount64(ht.n)
			number, unit := c.Human(counts.BinaryPrefixes, "B")
			assert.Equalf(ht.number, number, "Number for Count64(%d) in binary", ht.n)
			assert.Equalf(ht.unit, unit, "Unit for Count64(%d) in binary", ht.n)
		}
	}
}

func TestLimits32(t *testing.T) {
	assert := assert.New(t)

	c := counts.NewCount32(0xffffffff)
	number, unit := c.Human(counts.MetricPrefixes, "cd")
	assert.Equalf("∞", number, "Number for Count32(%d) in metric", c.ToUint64())
	assert.Equalf("cd", unit, "Unit for Count32(%d) in metric", c.ToUint64())
}

func TestLimits64(t *testing.T) {
	assert := assert.New(t)

	c := counts.NewCount64(0xffffffffffffffff)
	number, unit := c.Human(counts.MetricPrefixes, "B")
	assert.Equalf("∞", number, "Number for Count64(%d) in metric", c.ToUint64())
	assert.Equalf("B", unit, "Unit for Count64(%d) in metric", c.ToUint64())
}
