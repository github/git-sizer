package git

import (
	"regexp"
	"strings"
)

type ReferenceFilter func(refname string) bool

func AllReferencesFilter(_ string) bool {
	return true
}

type Polarity uint8

const (
	Include Polarity = iota
	Exclude
)

func (p Polarity) Inverted() Polarity {
	switch p {
	case Include:
		return Exclude
	case Exclude:
		return Include
	default:
		// This shouldn't happen:
		return Exclude
	}
}

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

func (ief *IncludeExcludeFilter) Filter(refname string) bool {
	for i := len(ief.filters); i > 0; i-- {
		f := ief.filters[i-1]
		if !f.filter(refname) {
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
		return func(refname string) bool {
			return strings.HasPrefix(refname, prefix)
		}
	}

	return func(refname string) bool {
		return strings.HasPrefix(refname, prefix) &&
			(len(refname) == len(prefix) || refname[len(prefix)] == '/')
	}
}

// RegexpFilter returns a `ReferenceFilter` that matches references
// whose names match the specified `prefix`, which must match the
// whole reference name.
func RegexpFilter(pattern string) (ReferenceFilter, error) {
	pattern = "^" + pattern + "$"
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	return func(refname string) bool {
		return re.MatchString(refname)
	}, nil
}
