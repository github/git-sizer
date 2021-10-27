package git

import (
	"regexp"
	"strings"
)

type ReferenceFilter interface {
	Filter(refname string) bool
}

// Combiner combines two `ReferenceFilter`s into one compound one.
// `f1` is allowed to be `nil`.
type Combiner interface {
	Combine(f1, f2 ReferenceFilter) ReferenceFilter
	Inverted() Combiner
}

type inverse struct {
	f ReferenceFilter
}

func (f inverse) Filter(refname string) bool {
	return !f.f.Filter(refname)
}

type intersection struct {
	f1, f2 ReferenceFilter
}

func (f intersection) Filter(refname string) bool {
	return f.f1.Filter(refname) && f.f2.Filter(refname)
}

// Include is a Combiner that includes the references matched by `f2`.
// If `f1` is `nil`, it is treated as including nothing.
type include struct{}

func (_ include) Combine(f1, f2 ReferenceFilter) ReferenceFilter {
	if f1 == nil {
		return f2
	}
	return union{f1, f2}
}

func (_ include) Inverted() Combiner {
	return Exclude
}

var Include include

type union struct {
	f1, f2 ReferenceFilter
}

func (f union) Filter(refname string) bool {
	return f.f1.Filter(refname) || f.f2.Filter(refname)
}

// Exclude is a Combiner that excludes the references matched by `f2`.
// If `f1` is `nil`, it is treated as including everything.
type exclude struct{}

func (_ exclude) Combine(f1, f2 ReferenceFilter) ReferenceFilter {
	if f1 == nil {
		return inverse{f2}
	}
	return intersection{f1, inverse{f2}}

}

func (_ exclude) Inverted() Combiner {
	return include{}
}

var Exclude exclude

type allReferencesFilter struct{}

func (_ allReferencesFilter) Filter(_ string) bool {
	return true
}

var AllReferencesFilter allReferencesFilter

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
	if prefix == "" {
		return AllReferencesFilter
	}
	return prefixFilter{prefix}
}

type prefixFilter struct {
	prefix string
}

func (f prefixFilter) Filter(refname string) bool {
	if strings.HasSuffix(f.prefix, "/") {
		return strings.HasPrefix(refname, f.prefix)
	}

	return strings.HasPrefix(refname, f.prefix) &&
		(len(refname) == len(f.prefix) || refname[len(f.prefix)] == '/')
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

	return regexpFilter{re}, nil
}

type regexpFilter struct {
	re *regexp.Regexp
}

func (f regexpFilter) Filter(refname string) bool {
	return f.re.MatchString(refname)
}
