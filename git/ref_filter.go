package git

import (
	"strings"
)

type ReferenceFilter func(Reference) bool

func AllReferencesFilter(_ Reference) bool {
	return true
}

type Polarity uint8

const (
	Include Polarity = iota
	Exclude
)

// polarizedFilter is a filter that might match, in which case it
// includes or excludes the reference (according to its polarity). If
// it doesn't match, then it doesn't say anything about the reference.
type polarizedFilter struct {
	polarity Polarity
	filter   ReferenceFilter
}

// IncludeExcludeFilter is a filter based on a bunch of
// `polarizedFilter`s. The last one that matches a reference wins. If
// none match, then the result is based on the polarity of the first
// polarizedFilter: if it is `Include`, then return `false`; if it is
// `Exclude`, then return `true`.
type IncludeExcludeFilter struct {
	filters []polarizedFilter
}

func (ief *IncludeExcludeFilter) Include(f ReferenceFilter) {
	ief.filters = append(ief.filters, polarizedFilter{Include, f})
}

func (ief *IncludeExcludeFilter) Exclude(f ReferenceFilter) {
	ief.filters = append(ief.filters, polarizedFilter{Exclude, f})
}

func (ief *IncludeExcludeFilter) Filter(r Reference) bool {
	for i := len(ief.filters); i > 0; i-- {
		f := ief.filters[i-1]
		if !f.filter(r) {
			continue
		}
		return f.polarity == Include
	}

	return len(ief.filters) == 0 || ief.filters[0].polarity == Exclude
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
