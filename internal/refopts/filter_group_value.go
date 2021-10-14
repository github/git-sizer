package refopts

import (
	"fmt"

	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/sizes"
)

// filterGroupValue handles `--refgroup=REFGROUP` options, which
// affect the top-level filter. These are a little bit tricky, because
// the references matched by a refgroup depend on its parents (because
// if the parents don't allow the reference, it won't even get tested
// by the regroup's own filter) and also its children (because if the
// refgroup doesn't have its own filter, then it is defined to be the
// union of its children). Meanwhile, when testing parents, we
// shouldn't test the top-level group, because that's what we are
// trying to affect.
//
// The filtering itself is implemented using a `refGroupFilter`, which
// contains a pointer to a `refGroup` and uses it (including its
// `parent` and `subgroups` to figure out what should be allowed.
type filterGroupValue struct {
	filter *git.ReferenceFilter
	groups map[sizes.RefGroupSymbol]*refGroup
}

func (v *filterGroupValue) Set(symbolString string) error {
	symbol := sizes.RefGroupSymbol(symbolString)

	refGroup, ok := v.groups[symbol]

	if !ok || symbol == "" {
		return fmt.Errorf("refgroup '%s' is not defined", symbol)
	}

	*v.filter = git.Include.Combine(*v.filter, refGroupFilter{refGroup})

	return nil
}

func (v *filterGroupValue) Get() interface{} {
	return nil
}

func (v *filterGroupValue) String() string {
	return ""
}

func (v *filterGroupValue) Type() string {
	return "name"
}

// refGroupFilter is a filter based on what would be allowed through
// by a particular refGroup. This is used as part of a top-level
// filter, so it ignores what the top-level filter would say.
type refGroupFilter struct {
	refGroup *refGroup
}

func (f refGroupFilter) Filter(refname string) bool {
	return refGroupPasses(f.refGroup.parent, refname) &&
		refGroupMatches(f.refGroup, refname)
}

// refGroupMatches retruns true iff `rg` would allow `refname`
// through, not considering its parents. If `rg` doesn't have its own
// filter, this consults its children.
func refGroupMatches(rg *refGroup, refname string) bool {
	if rg.filter != nil {
		return rg.filter.Filter(refname)
	}

	for _, sg := range rg.subgroups {
		if refGroupMatches(sg, refname) {
			return true
		}
	}

	return false
}

// refGroupPasses returns true iff `rg` and the parents of `rg` (not
// including the top-level group) would allow `refname` through. This
// does not consider children of `rg`, which we would still need to
// consult if `rg` doesn't have a filter of its own.
func refGroupPasses(rg *refGroup, refname string) bool {
	if rg.Symbol == "" {
		return true
	}
	if !refGroupPasses(rg.parent, refname) {
		return false
	}
	return rg.filter == nil || rg.filter.Filter(refname)
}
