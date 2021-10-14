package refopts

import (
	"fmt"
	"io"

	"github.com/github/git-sizer/sizes"
)

// showRefFilter is a `git.ReferenceFilter` that logs its choices to Stderr.
type showRefGrouper struct {
	*refGrouper
	w io.Writer
}

func (refGrouper showRefGrouper) Categorize(refname string) (bool, []sizes.RefGroupSymbol) {
	walk, symbols := refGrouper.refGrouper.Categorize(refname)
	if walk {
		fmt.Fprintf(refGrouper.w, "+ %s\n", refname)
	} else {
		fmt.Fprintf(refGrouper.w, "  %s\n", refname)
	}
	return walk, symbols
}
