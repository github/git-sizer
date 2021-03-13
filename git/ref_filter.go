package git

import (
	"strings"
)

type ReferenceFilter func(Reference) bool

func AllReferencesFilter(_ Reference) bool {
	return true
}

// PrefixFilter returns a `ReferenceFilter` that matches references
// whose names start with the specified `prefix`, which must match at
// a component boundary. For example,
//
// * Prefix "refs/foo" matches "refs/foo" and "refs/foo/bar" but not
//   "refs/foobar".
//
// * Prefix "refs/foo/" matches "refs/foo/bar" but not "refs/foo" or
//   "refs/foobar".
func PrefixFilter(prefix string) ReferenceFilter {
	if strings.HasSuffix(prefix, "/") {
		return func(r Reference) bool {
			return strings.HasPrefix(r.Refname, prefix)
		}
	}

	return func(r Reference) bool {
		return strings.HasPrefix(r.Refname, prefix) &&
			(len(r.Refname) == len(prefix) || r.Refname[len(prefix)] == '/')
	}
}

var (
	BranchesFilter ReferenceFilter = PrefixFilter("refs/heads/")
	TagsFilter     ReferenceFilter = PrefixFilter("refs/tags/")
	RemotesFilter  ReferenceFilter = PrefixFilter("refs/remotes/")
)

func notNilFilters(filters ...ReferenceFilter) []ReferenceFilter {
	var ret []ReferenceFilter
	for _, filter := range filters {
		if filter != nil {
			ret = append(ret, filter)
		}
	}
	return ret
}

func OrFilter(filters ...ReferenceFilter) ReferenceFilter {
	filters = notNilFilters(filters...)
	if len(filters) == 0 {
		return AllReferencesFilter
	} else if len(filters) == 1 {
		return filters[0]
	} else {
		return func(r Reference) bool {
			for _, filter := range filters {
				if filter(r) {
					return true
				}
			}
			return false
		}
	}
}

func AndFilter(filters ...ReferenceFilter) ReferenceFilter {
	filters = notNilFilters(filters...)
	if len(filters) == 0 {
		return AllReferencesFilter
	} else if len(filters) == 1 {
		return filters[0]
	} else {
		return func(r Reference) bool {
			for _, filter := range filters {
				if !filter(r) {
					return false
				}
			}
			return true
		}
	}
}

func NotFilter(filter ReferenceFilter) ReferenceFilter {
	return func(r Reference) bool {
		return !filter(r)
	}
}
