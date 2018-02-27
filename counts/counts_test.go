package counts_test

import (
	"testing"

	"github.com/github/git-sizer/counts"

	"github.com/stretchr/testify/assert"
)

func TestCount32(t *testing.T) {
	assert := assert.New(t)

	c := counts.NewCount32(0)
	assert.Equalf(uint64(0), c.ToUint64(), "NewCount32(0).ToUint64() should be 0")

	c.Increment(counts.Count32(0xf0000000))
	assert.Equalf(uint64(0xf0000000), c.ToUint64(), "Count32(0xf0000000).ToUint64() value")

	c.Increment(counts.Count32(0xf0000000))
	assert.Equalf(uint64(0xffffffff), c.ToUint64(), "Count32(0xffffffff).ToUint64() value")
}

func TestCount64(t *testing.T) {
	assert := assert.New(t)

	c := counts.NewCount64(0)
	assert.Equalf(uint64(0), c.ToUint64(), "NewCount64(0).ToUint64() should be 0")

	c.Increment(counts.Count64(0xf000000000000000))
	assert.Equalf(uint64(0xf000000000000000), c.ToUint64(), "Count64(0xf000000000000000).ToUint64() value")

	c.Increment(counts.Count64(0xf000000000000000))
	assert.Equalf(uint64(0xffffffffffffffff), c.ToUint64(), "Count64(0xffffffffffffffff).ToUint64() value")
}
