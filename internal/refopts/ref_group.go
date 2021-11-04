package refopts

import (
	"fmt"

	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/sizes"
)

// refGroup represents one reference group and also its relationship
// to its parent group and any subgroups.. Note that reference groups
// don't intrinsically have anything to do with the layout of the
// reference namespace, but they will often be used that way.
type refGroup struct {
	sizes.RefGroup

	// filter is the filter for just this reference group. Filters
	// for any parent groups must also be applied.
	filter git.ReferenceFilter

	parent *refGroup

	// subgroups are the `refGroup` instances representing any
	// direct subgroups.
	subgroups []*refGroup

	// otherRefGroup, if set, is the refGroup for tallying
	// references that match `filter` but don't match any of the
	// subgroups.
	otherRefGroup *sizes.RefGroup
}

func (rg *refGroup) collectSymbols(refname string) (bool, []sizes.RefGroupSymbol) {
	walk := false
	var symbols []sizes.RefGroupSymbol

	if rg.filter == nil {
		// The tree doesn't have its own filter. Consider it matched
		// iff at least one subtree matches it.

		for _, sg := range rg.subgroups {
			w, ss := sg.collectSymbols(refname)
			if w {
				walk = true
			}
			if len(ss) > 0 && len(symbols) == 0 {
				symbols = append(symbols, rg.Symbol)
			}
			symbols = append(symbols, ss...)
		}
	} else {
		// The tree has its own filter. If it doesn't match the
		// reference, then the subtrees don't even get a chance to
		// try.
		if !rg.filter.Filter(refname) {
			return false, nil
		}

		walk = true
		symbols = append(symbols, rg.Symbol)

		for _, sg := range rg.subgroups {
			_, ss := sg.collectSymbols(refname)
			symbols = append(symbols, ss...)
		}

		// References that match the tree filter but no subtree
		// filters are counted as "other":
		if rg.otherRefGroup != nil && len(symbols) == 1 {
			symbols = append(symbols, rg.otherRefGroup.Symbol)
		}
	}

	return walk, symbols
}

// augmentFromConfig augments `rg` based on configuration in the
// gitconfig and returns the result. It is not considered an error if
// there are no usable config entries for the filter.
func (rg *refGroup) augmentFromConfig(configger Configger) error {
	config, err := configger.GetConfig(fmt.Sprintf("refgroup.%s", rg.Symbol))
	if err != nil {
		return err
	}

	for _, entry := range config.Entries {
		switch entry.Key {
		case "name":
			rg.Name = entry.Value
		case "include":
			rg.filter = git.Include.Combine(
				rg.filter, git.PrefixFilter(entry.Value),
			)
		case "includeregexp":
			f, err := git.RegexpFilter(entry.Value)
			if err != nil {
				return fmt.Errorf(
					"invalid regular expression for '%s': %w",
					config.FullKey(entry.Key), err,
				)
			}
			rg.filter = git.Include.Combine(rg.filter, f)
		case "exclude":
			rg.filter = git.Exclude.Combine(
				rg.filter, git.PrefixFilter(entry.Value),
			)
		case "excluderegexp":
			f, err := git.RegexpFilter(entry.Value)
			if err != nil {
				return fmt.Errorf(
					"invalid regular expression for '%s': %w",
					config.FullKey(entry.Key), err,
				)
			}
			rg.filter = git.Exclude.Combine(rg.filter, f)
		default:
			// Ignore unrecognized keys.
		}
	}

	return nil
}
