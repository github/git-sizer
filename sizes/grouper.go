package sizes

import (
	"context"

	"github.com/github/git-sizer/git"
)

// RefGroupSymbol is the string "identifier" that is used to refer to
// a refgroup, for example in the gitconfig. Nesting of refgroups is
// inferred from their names, using "." as separator between
// components. For example, if there are three refgroups with symbols
// "tags", "tags.releases", and "foo.bar", then "tags.releases" is
// considered to be nested within "tags", and "foo.bar" is considered
// to be nested within "foo", the latter being created automatically
// if it was not configured explicitly.
type RefGroupSymbol string

// RefGroup is a group of references, for example "branches" or
// "tags". Reference groups might overlap.
type RefGroup struct {
	// Symbol is the unique string by which this `RefGroup` is
	// identified and configured. It consists of dot-separated
	// components, which implicitly makes a nested tree-like
	// structure.
	Symbol RefGroupSymbol

	// Name is the name for this `ReferenceGroup` to be presented
	// in user-readable output.
	Name string
}

// RefGrouper describes a type that can collate reference names into
// groups and decide which ones to walk.
type RefGrouper interface {
	// Categorize tells whether `refname` should be walked at all,
	// and if so, the symbols of the reference groups to which it
	// belongs.
	Categorize(refname string) (bool, []RefGroupSymbol)

	// Groups returns the list of `ReferenceGroup`s, in the order
	// that they should be presented. The return value might
	// depend on which references have been seen so far.
	Groups() []RefGroup
}

type RefRoot struct {
	ref    git.Reference
	walk   bool
	groups []RefGroupSymbol
}

func (rr RefRoot) Name() string             { return rr.ref.Refname }
func (rr RefRoot) OID() git.OID             { return rr.ref.OID }
func (rr RefRoot) Reference() git.Reference { return rr.ref }
func (rr RefRoot) Walk() bool               { return rr.walk }
func (rr RefRoot) Groups() []RefGroupSymbol { return rr.groups }

func CollectReferences(
	ctx context.Context, repo *git.Repository, rg RefGrouper,
) ([]RefRoot, error) {
	refIter, err := repo.NewReferenceIter(ctx)
	if err != nil {
		return nil, err
	}

	var refsSeen []RefRoot
	for {
		ref, ok, err := refIter.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return refsSeen, nil
		}

		walk, groups := rg.Categorize(ref.Refname)

		refsSeen = append(
			refsSeen,
			RefRoot{
				ref:    ref,
				walk:   walk,
				groups: groups,
			},
		)
	}
}
