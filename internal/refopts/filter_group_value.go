package refopts

import (
	"fmt"

	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/sizes"
)

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

	*v.filter = git.Include.Combine(*v.filter, refGroup.filter)

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
