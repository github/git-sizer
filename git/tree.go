package git

import (
	"errors"
	"strconv"
	"strings"

	"github.com/github/git-sizer/counts"
)

// Tree represents a Git tree object.
type Tree struct {
	data string
}

// ParseTree parses the tree object whose contents are contained in
// `data`. `oid` is currently unused.
func ParseTree(oid OID, data []byte) (*Tree, error) {
	return &Tree{string(data)}, nil
}

// Size returns the size of the tree object.
func (tree Tree) Size() counts.Count32 {
	return counts.NewCount32(uint64(len(tree.data)))
}

// TreeEntry represents an entry in a Git tree object. Note that Name
// shares memory with the tree data that were originally read; i.e.,
// retaining a pointer to Name keeps the tree data reachable.
type TreeEntry struct {
	Name     string
	OID      OID
	Filemode uint
}

// TreeIter is an iterator over the entries in a Git tree object.
type TreeIter struct {
	// The as-yet-unread part of the tree's data.
	data string
}

// Iter returns an iterator over the entries in `tree`.
func (tree *Tree) Iter() *TreeIter {
	return &TreeIter{
		data: tree.data,
	}
}

// NextEntry returns either the next entry in a Git tree, or a `false`
// boolean value if there are no more entries.
func (iter *TreeIter) NextEntry() (TreeEntry, bool, error) {
	var entry TreeEntry

	if len(iter.data) == 0 {
		return TreeEntry{}, false, nil
	}

	spAt := strings.IndexByte(iter.data, ' ')
	if spAt < 0 {
		return TreeEntry{}, false, errors.New("failed to find SP after mode")
	}
	mode, err := strconv.ParseUint(iter.data[:spAt], 8, 32)
	if err != nil {
		return TreeEntry{}, false, err
	}
	entry.Filemode = uint(mode)

	iter.data = iter.data[spAt+1:]
	nulAt := strings.IndexByte(iter.data, 0)
	if nulAt < 0 {
		return TreeEntry{}, false, errors.New("failed to find NUL after filename")
	}

	entry.Name = iter.data[:nulAt]

	iter.data = iter.data[nulAt+1:]
	if len(iter.data) < 20 {
		return TreeEntry{}, false, errors.New("tree entry ends unexpectedly")
	}

	copy(entry.OID.v[0:20], iter.data[0:20])
	iter.data = iter.data[20:]

	return entry, true, nil
}
