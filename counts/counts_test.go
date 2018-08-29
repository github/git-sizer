package counts_test

import (
	"testing"

	"github.com/github/git-sizer/counts"

	"github.com/stretchr/testify/assert"
)

func TestCount32(t *testing.T) {
	assert := assert.New(t)

	var value uint64
	var overflow bool

	c := counts.NewCount32(0)
	value, overflow = c.ToUint64()
	assert.Equalf(uint64(0), value, "NewCount32(0).ToUint64() should be 0")
	assert.False(overflow, "NewCount32(0).ToUint64() does not overflow")

	c.Increment(counts.Count32(0xf0000000))
	value, overflow = c.ToUint64()
	assert.Equalf(uint64(0xf0000000), value, "Count32(0xf0000000).ToUint64() value")
	assert.False(overflow, "NewCount32(0xf0000000).ToUint64() does not overflow")

	c.Increment(counts.Count32(0xf0000000))
	value, overflow = c.ToUint64()
	assert.Equalf(uint64(0xffffffff), value, "Count32(0xffffffff).ToUint64() value")
	assert.True(overflow, "NewCount32(0xffffffff).ToUint64() overflows")
}

func TestCount64(t *testing.T) {
	assert := assert.New(t)

	var value uint64
	var overflow bool

	c := counts.NewCount64(0)
	value, overflow = c.ToUint64()
	assert.Equalf(uint64(0), value, "NewCount64(0).ToUint64() should be 0")
	assert.False(overflow, "NewCount64(0).ToUint64() does not overflow")

	c.Increment(counts.Count64(0xf000000000000000))
	value, overflow = c.ToUint64()
	assert.Equalf(uint64(0xf000000000000000), value, "Count64(0xf000000000000000).ToUint64() value")
	assert.False(overflow, "NewCount64(0xf000000000000000).ToUint64() does not overflow")

	c.Increment(counts.Count64(0xf000000000000000))
	value, overflow = c.ToUint64()
	assert.Equalf(uint64(0xffffffffffffffff), value, "Count64(0xffffffffffffffff).ToUint64() value")
	assert.True(overflow, "NewCount64(0xffffffffffffffff).ToUint64() overflows")
}
