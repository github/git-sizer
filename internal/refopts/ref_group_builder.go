package refopts

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/sizes"
)

type Configger interface {
	Config(prefix string) (*git.Config, error)
}

// RefGroupBuilder handles reference-related options and puts together
// a `sizes.RefGrouper` to be used by the main part of the program.
type RefGroupBuilder struct {
	Filter   git.ReferenceFilter
	ShowRefs bool
}

// Add some reference-related options to `flags`.
func (rgb *RefGroupBuilder) AddRefopts(flags *pflag.FlagSet, configger Configger) {
	flags.Var(
		&filterValue{&rgb.Filter, git.Include, "", false}, "include",
		"include specified references",
	)
	flags.Var(
		&filterValue{&rgb.Filter, git.Include, "", true}, "include-regexp",
		"include references matching the specified regular expression",
	)
	flags.Var(
		&filterValue{&rgb.Filter, git.Exclude, "", false}, "exclude",
		"exclude specified references",
	)
	flags.Var(
		&filterValue{&rgb.Filter, git.Exclude, "", true}, "exclude-regexp",
		"exclude references matching the specified regular expression",
	)

	flag := flags.VarPF(
		&filterValue{&rgb.Filter, git.Include, "refs/heads", false}, "branches", "",
		"process all branches",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Exclude, "refs/heads", false}, "no-branches", "",
		"exclude all branches",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Include, "refs/tags", false}, "tags", "",
		"process all tags",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Exclude, "refs/tags", false}, "no-tags", "",
		"exclude all tags",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Include, "refs/remotes", false}, "remotes", "",
		"process all remote-tracking references",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Exclude, "refs/remotes", false}, "no-remotes", "",
		"exclude all remote-tracking references",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Include, "refs/notes", false}, "notes", "",
		"process all git-notes references",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Exclude, "refs/notes", false}, "no-notes", "",
		"exclude all git-notes references",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Include, "refs/stash", true}, "stash", "",
		"process refs/stash",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&rgb.Filter, git.Exclude, "refs/stash", true}, "no-stash", "",
		"exclude refs/stash",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterGroupValue{&rgb.Filter, configger}, "refgroup", "",
		"process references in refgroup defined by gitconfig",
	)

	flags.BoolVar(&rgb.ShowRefs, "show-refs", false, "list the references being processed")
}

// Finish collects the information gained from processing the options
// and returns a `sizes.RefGrouper`.
func (rgb *RefGroupBuilder) Finish() (sizes.RefGrouper, error) {
	if rgb.Filter == nil {
		rgb.Filter = git.AllReferencesFilter
	}

	if rgb.ShowRefs {
		fmt.Fprintf(os.Stderr, "References (included references marked with '+'):\n")
		rgb.Filter = showRefFilter{rgb.Filter}
	}

	return &refGrouper{
		filter: rgb.Filter,
	}, nil

}

type refGrouper struct {
	filter git.ReferenceFilter
}

func (rg *refGrouper) Categorize(refname string) (bool, []sizes.RefGroupSymbol) {
	return rg.filter.Filter(refname), nil
}

func (rg *refGrouper) Groups() []sizes.RefGroup {
	return nil
}

// showRefFilter is a `git.ReferenceFilter` that logs its choices to Stderr.
type showRefFilter struct {
	f git.ReferenceFilter
}

func (f showRefFilter) Filter(refname string) bool {
	b := f.f.Filter(refname)
	if b {
		fmt.Fprintf(os.Stderr, "+ %s\n", refname)
	} else {
		fmt.Fprintf(os.Stderr, "  %s\n", refname)
	}
	return b
}
