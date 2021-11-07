package refopts

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/sizes"
)

// Configger is an abstraction for a thing that can read gitconfig.
type Configger interface {
	GetConfig(prefix string) (*git.Config, error)
}

// RefGroupBuilder handles reference-related options and puts together
// a `sizes.RefGrouper` to be used by the main part of the program.
type RefGroupBuilder struct {
	topLevelGroup *refGroup
	groups        map[sizes.RefGroupSymbol]*refGroup
}

// NewRefGroupBuilder creates and returns a `RefGroupBuilder`
// instance.
func NewRefGroupBuilder(configger Configger) (*RefGroupBuilder, error) {
	tlg := refGroup{
		RefGroup: sizes.RefGroup{
			Symbol: "",
			Name:   "Refs to walk",
		},
	}

	rgb := RefGroupBuilder{
		topLevelGroup: &tlg,
		groups: map[sizes.RefGroupSymbol]*refGroup{
			"": &tlg,
		},
	}

	rgb.initializeStandardRefgroups()
	if err := rgb.readRefgroupsFromGitconfig(configger); err != nil {
		return nil, err
	}

	return &rgb, nil
}

// getGroup returns the `refGroup` for the symbol with the specified
// name, first creating it (and any missing parents) if needed.
func (rgb *RefGroupBuilder) getGroup(symbol sizes.RefGroupSymbol) *refGroup {
	if rg, ok := rgb.groups[symbol]; ok {
		return rg
	}

	parentSymbol := parentName(symbol)
	parent := rgb.getGroup(parentSymbol)

	rg := refGroup{
		RefGroup: sizes.RefGroup{
			Symbol: symbol,
		},
		parent: parent,
	}

	rgb.groups[symbol] = &rg
	parent.subgroups = append(parent.subgroups, &rg)
	return &rg
}

// parentName returns the symbol of the refgroup that is the parent of
// `symbol`, or "" if `symbol` is the top-level group.
func parentName(symbol sizes.RefGroupSymbol) sizes.RefGroupSymbol {
	i := strings.LastIndexByte(string(symbol), '.')
	if i == -1 {
		return ""
	}
	return symbol[:i]
}

// initializeStandardRefgroups initializes the built-in refgroups
// ("branches", "tags", etc).
func (rgb *RefGroupBuilder) initializeStandardRefgroups() {
	initializeGroup := func(
		symbol sizes.RefGroupSymbol, name string, filter git.ReferenceFilter,
	) {
		rg := rgb.getGroup(symbol)
		rg.Name = name
		rg.filter = filter
	}

	initializeGroup("branches", "Branches", git.PrefixFilter("refs/heads/"))
	initializeGroup("tags", "Tags", git.PrefixFilter("refs/tags/"))
	initializeGroup("remotes", "Remote-tracking refs", git.PrefixFilter("refs/remotes/"))
	initializeGroup("pulls", "Pull request refs", git.PrefixFilter("refs/pull/"))

	filter, err := git.RegexpFilter(`refs/changes/\d{2}/\d+/\d+`)
	if err != nil {
		panic("internal error")
	}
	initializeGroup("changes", "Changeset refs", filter)

	initializeGroup("notes", "Git notes", git.PrefixFilter("refs/notes/"))

	filter, err = git.RegexpFilter(`refs/stash`)
	if err != nil {
		panic("internal error")
	}
	initializeGroup("stash", "Git stash", filter)
}

// readRefgroupsFromGitconfig reads any refgroups defined in the
// gitconfig into `rgb`. Any configuration settings for the built-in
// groups are added to the pre-existing definitions of those groups.
func (rgb *RefGroupBuilder) readRefgroupsFromGitconfig(configger Configger) error {
	if configger == nil {
		// At this point, it is not yet certain that the command was
		// run inside a Git repository. If not, ignore this option
		// (the command will error out anyway).
		return nil
	}

	config, err := configger.GetConfig("refgroup")
	if err != nil {
		return err
	}

	seen := make(map[sizes.RefGroupSymbol]bool)
	for _, entry := range config.Entries {
		symbol, _ := splitKey(entry.Key)
		if symbol == "" || seen[symbol] {
			// The point of this loop is only to find
			// _which_ groups are defined, so we only need
			// to visit each one once.
			continue
		}

		rg := rgb.getGroup(symbol)
		if err := rg.augmentFromConfig(configger); err != nil {
			return err
		}

		seen[symbol] = true
	}

	return nil
}

// splitKey splits `key`, which is part of a gitconfig key, into the
// refgroup symbol to which it applies and the field name within that
// section.
func splitKey(key string) (sizes.RefGroupSymbol, string) {
	i := strings.LastIndexByte(key, '.')
	if i == -1 {
		return "", key
	}
	return sizes.RefGroupSymbol(key[:i]), key[i+1:]
}

// AddRefopts adds the reference-related options to `flags`.
func (rgb *RefGroupBuilder) AddRefopts(flags *pflag.FlagSet) {
	flags.Var(
		&filterValue{rgb, git.Include, "", false}, "include",
		"include specified references",
	)

	flag := flags.VarPF(
		&filterValue{rgb, git.Include, "", true}, "include-regexp", "",
		"include references matching the specified regular expression",
	)
	flag.Hidden = true
	flag.Deprecated = "use --include=/REGEXP/"

	flags.Var(
		&filterValue{rgb, git.Exclude, "", false}, "exclude",
		"exclude specified references",
	)

	flag = flags.VarPF(
		&filterValue{rgb, git.Exclude, "", true}, "exclude-regexp", "",
		"exclude references matching the specified regular expression",
	)
	flag.Hidden = true
	flag.Deprecated = "use --exclude=/REGEXP/"

	flag = flags.VarPF(
		&filterValue{rgb, git.Include, "refs/heads", false}, "branches", "",
		"process all branches",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Exclude, "refs/heads", false}, "no-branches", "",
		"exclude all branches",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Include, "refs/tags", false}, "tags", "",
		"process all tags",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Exclude, "refs/tags", false}, "no-tags", "",
		"exclude all tags",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Include, "refs/remotes", false}, "remotes", "",
		"process all remote-tracking references",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Exclude, "refs/remotes", false}, "no-remotes", "",
		"exclude all remote-tracking references",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Include, "refs/notes", false}, "notes", "",
		"process all git-notes references",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Exclude, "refs/notes", false}, "no-notes", "",
		"exclude all git-notes references",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Include, "refs/stash", true}, "stash", "",
		"process refs/stash",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{rgb, git.Exclude, "refs/stash", true}, "no-stash", "",
		"exclude refs/stash",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterGroupValue{rgb}, "refgroup", "",
		"process references in refgroup defined by gitconfig",
	)
	flag.Hidden = true
	flag.Deprecated = "use --include=@REFGROUP"
}

// Finish collects the information gained from processing the options
// and returns a `sizes.RefGrouper`.
func (rgb *RefGroupBuilder) Finish() (sizes.RefGrouper, error) {
	if rgb.topLevelGroup.filter == nil {
		rgb.topLevelGroup.filter = git.AllReferencesFilter
	}

	refGrouper := refGrouper{
		topLevelGroup: rgb.topLevelGroup,
	}

	if err := refGrouper.fillInTree(refGrouper.topLevelGroup); err != nil {
		return nil, err
	}

	if refGrouper.topLevelGroup.filter != nil {
		refGrouper.ignoredRefGroup = &sizes.RefGroup{
			Symbol: "ignored",
			Name:   "Ignored",
		}
		refGrouper.refGroups = append(refGrouper.refGroups, *refGrouper.ignoredRefGroup)
	}

	return &refGrouper, nil
}

// refGrouper is a `sizes.RefGrouper` based on a hierarchy of nested
// refgroups.
type refGrouper struct {
	topLevelGroup *refGroup
	refGroups     []sizes.RefGroup

	// ignoredRefGroup, if set, is the reference group for
	// tallying references that don't match at all.
	ignoredRefGroup *sizes.RefGroup
}

// fillInTree processes the refgroups in the tree rooted at `rg`,
// setting default names where they are missing, verifying that they
// are all defined, adding "Other" groups where needed, and adding the
// refgroups in depth-first-traversal order to `refGrouper.refGroups`.
func (refGrouper *refGrouper) fillInTree(rg *refGroup) error {
	if rg.Name == "" {
		_, rg.Name = splitKey(string(rg.Symbol))
	}

	if rg.filter == nil && len(rg.subgroups) == 0 {
		return fmt.Errorf("refgroup '%s' is not defined", rg.Symbol)
	}

	refGrouper.refGroups = append(refGrouper.refGroups, rg.RefGroup)

	for _, rg := range rg.subgroups {
		if err := refGrouper.fillInTree(rg); err != nil {
			return err
		}
	}

	if len(rg.subgroups) != 0 {
		var otherSymbol sizes.RefGroupSymbol
		if rg.Symbol == "" {
			otherSymbol = "other"
		} else {
			otherSymbol = sizes.RefGroupSymbol(fmt.Sprintf("%s.other", rg.Symbol))
		}
		rg.otherRefGroup = &sizes.RefGroup{
			Symbol: otherSymbol,
			Name:   "Other",
		}
		refGrouper.refGroups = append(refGrouper.refGroups, *rg.otherRefGroup)
	}

	return nil
}

// Categorize decides whether to walk the reference named `refname`
// and which refgroup(s) it should be counted in.
func (refGrouper *refGrouper) Categorize(refname string) (bool, []sizes.RefGroupSymbol) {
	walk, symbols := refGrouper.topLevelGroup.collectSymbols(refname)
	if !walk && refGrouper.ignoredRefGroup != nil {
		symbols = append(symbols, refGrouper.ignoredRefGroup.Symbol)
	}
	return walk, symbols
}

// Groups returns a list of all defined refgroups, in the order that
// they should be output.
func (refGrouper *refGrouper) Groups() []sizes.RefGroup {
	return refGrouper.refGroups
}
