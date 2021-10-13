package refopts

import (
	"fmt"
	"os"

	"github.com/github/git-sizer/git"
)

type filterGroupValue struct {
	filter    *git.ReferenceFilter
	configger Configger
}

func (v *filterGroupValue) Set(name string) error {
	// At this point, it is not yet certain that the command was run
	// inside a Git repository. If not, ignore this option (the
	// command will error out anyway).
	if v.configger == nil {
		fmt.Fprintf(
			os.Stderr,
			"warning: not in Git repository; ignoring '--refgroup' option.\n",
		)
		return nil
	}

	config, err := v.configger.Config(fmt.Sprintf("refgroup.%s", name))
	if err != nil {
		return err
	}
	for _, entry := range config.Entries {
		switch entry.Key {
		case "include":
			*v.filter = git.Include.Combine(
				*v.filter, git.PrefixFilter(entry.Value),
			)
		case "includeregexp":
			filter, err := git.RegexpFilter(entry.Value)
			if err != nil {
				return fmt.Errorf(
					"invalid regular expression for 'refgroup.%s.%s': %w",
					name, entry.Key, err,
				)
			}
			*v.filter = git.Include.Combine(*v.filter, filter)
		case "exclude":
			*v.filter = git.Exclude.Combine(
				*v.filter, git.PrefixFilter(entry.Value),
			)
		case "excluderegexp":
			filter, err := git.RegexpFilter(entry.Value)
			if err != nil {
				return fmt.Errorf(
					"invalid regular expression for 'refgroup.%s.%s': %w",
					name, entry.Key, err,
				)
			}
			*v.filter = git.Exclude.Combine(*v.filter, filter)
		default:
			// Ignore unrecognized keys.
		}
	}
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
