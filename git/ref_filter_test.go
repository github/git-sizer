package git_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/git-sizer/git"
)

func TestPrefixFilter(t *testing.T) {
	t.Parallel()

	for _, p := range []struct {
		prefix   string
		refname  string
		expected bool
	}{
		{"refs/heads", "refs/heads/master", true},
		{"refs/heads", "refs/tags/master", false},
		{"refs/heads", "refs/he", false},
		{"refs/heads", "refs/headstrong", false},
		{"refs/heads", "refs/tags/refs/heads", false},

		{"refs/foo", "refs/foo/bar", true},
		{"refs/foo", "refs/foo", true},
		{"refs/foo", "refs/foobar", false},

		{"refs/foo/", "refs/foo/bar", true},
		{"refs/foo/", "refs/foo", false},
		{"refs/foo/", "refs/foobar", false},

		{"refs/stash", "refs/stash", true},
		{"refs/remotes", "refs/remotes/origin/master", true},
	} {
		t.Run(
			fmt.Sprintf("prefix '%s', refname '%s'", p.prefix, p.refname),
			func(t *testing.T) {
				assert.Equal(t, p.expected, git.PrefixFilter(p.prefix)(p.refname))
			},
		)
	}
}

func regexpFilter(t *testing.T, pattern string) git.ReferenceFilter {
	t.Helper()

	f, err := git.RegexpFilter(pattern)
	require.NoError(t, err)
	return f
}

func TestRegexpFilter(t *testing.T) {
	t.Parallel()

	for _, p := range []struct {
		pattern  string
		refname  string
		expected bool
	}{
		{`refs/heads/master`, "refs/heads/master", true},
		{`refs/heads/.*`, "refs/heads/master", true},
		{`.*/heads/.*`, "refs/heads/master", true},
		{`.*/heads/`, "refs/heads/master", false},
		{`.*/heads`, "refs/heads/master", false},
		{`/heads/.*`, "refs/heads/master", false},
		{`heads/.*`, "refs/heads/master", false},
		{`refs/tags/release-\d+\.\d+\.\d+`, "refs/tags/release-1.22.333", true},
		{`refs/tags/release-\d+\.\d+\.\d+`, "refs/tags/release-1.2.3rc1", false},
	} {
		t.Run(
			fmt.Sprintf("pattern '%s', refname '%s'", p.pattern, p.refname),
			func(t *testing.T) {
				assert.Equal(t, p.expected, regexpFilter(t, p.pattern)(p.refname))
			},
		)
	}
}

func TestIncludeExcludeFilter(t *testing.T) {
	t.Parallel()

	var filter git.IncludeExcludeFilter
	filter.Include(git.PrefixFilter("refs/heads"))
	filter.Exclude(regexpFilter(t, "refs/heads/.*foo.*"))
	filter.Include(git.PrefixFilter("refs/remotes"))
	filter.Exclude(git.PrefixFilter("refs/remotes/foo"))

	for _, p := range []struct {
		refname  string
		expected bool
	}{
		{"refs/heads/master", true},
		{"refs/heads/buffoon", false},
		{"refs/remotes/origin/master", true},
		{"refs/remotes/foo/master", false},
		{"refs/not-mentioned", false},
	} {
		t.Run(
			fmt.Sprintf("include-exclude '%s'", p.refname),
			func(t *testing.T) {
				assert.Equal(t, p.expected, filter.Filter(p.refname))
			},
		)
	}

}
