package git

import (
	"fmt"

	"github.com/github/git-sizer/counts"
)

// Commit represents the parts of a commit object that we need.
type Commit struct {
	Size    counts.Count32
	Parents []OID
	Tree    OID
}

// ParseCommit parses the commit object whose contents are in `data`.
// `oid` is used only in error messages.
func ParseCommit(oid OID, data []byte) (*Commit, error) {
	var parents []OID
	var tree OID
	var treeFound bool
	iter, err := NewObjectHeaderIter(oid.String(), data)
	if err != nil {
		return nil, err
	}
	for iter.HasNext() {
		key, value, err := iter.Next()
		if err != nil {
			return nil, err
		}
		switch key {
		case "parent":
			parent, err := NewOID(value)
			if err != nil {
				return nil, fmt.Errorf("malformed parent header in commit %s", oid)
			}
			parents = append(parents, parent)
		case "tree":
			if treeFound {
				return nil, fmt.Errorf("multiple trees found in commit %s", oid)
			}
			tree, err = NewOID(value)
			if err != nil {
				return nil, fmt.Errorf("malformed tree header in commit %s", oid)
			}
			treeFound = true
		}
	}
	if !treeFound {
		return nil, fmt.Errorf("no tree found in commit %s", oid)
	}
	return &Commit{
		Size:    counts.NewCount32(uint64(len(data))),
		Parents: parents,
		Tree:    tree,
	}, nil
}
