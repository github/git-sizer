package refopts

import (
	"fmt"
	"io"

	"github.com/github/git-sizer/sizes"
)

// showRefFilter is a `git.ReferenceFilter` that logs its choices to
// an `io.Writer`.
type showRefGrouper struct {
	sizes.RefGrouper
	w io.Writer
}

// Return a `sizes.RefGrouper` that wraps its argument and behaves
// like it except that it also logs its decisions to an `io.Writer`.
func NewShowRefGrouper(rg sizes.RefGrouper, w io.Writer) sizes.RefGrouper {
	return showRefGrouper{
		RefGrouper: rg,
		w:          w,
	}
}

func (rg showRefGrouper) Categorize(refname string) (bool, []sizes.RefGroupSymbol) {
	walk, symbols := rg.RefGrouper.Categorize(refname)
	if walk {
		fmt.Fprintf(rg.w, "+ %s\n", refname)
	} else {
		fmt.Fprintf(rg.w, "  %s\n", refname)
	}
	return walk, symbols
}
