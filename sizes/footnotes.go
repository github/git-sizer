package sizes

import (
	"bytes"
	"fmt"
)

// Footnotes collects and numbers footnotes for a `table`.
type Footnotes struct {
	footnotes []string
	indexes   map[string]int
}

// NewFootnotes creates and returns a new `Footnotes` instance.
func NewFootnotes() *Footnotes {
	return &Footnotes{
		indexes: make(map[string]int),
	}
}

// CreateCitation adds a footnote with the specified text and returns
// the string that should be used to refer to it (e.g., "[2]"). If
// there is already a footnote with the exact same text, reuse its
// number.
func (f *Footnotes) CreateCitation(footnote string) string {
	if footnote == "" {
		return ""
	}

	index, ok := f.indexes[footnote]
	if !ok {
		index = len(f.indexes) + 1
		f.footnotes = append(f.footnotes, footnote)
		f.indexes[footnote] = index
	}
	return fmt.Sprintf("[%d]", index)
}

// String returns a string representation of the footnote, including a
// trailing LF.
func (f *Footnotes) String() string {
	if len(f.footnotes) == 0 {
		return ""
	}

	buf := &bytes.Buffer{}
	buf.WriteByte('\n')
	for i, footnote := range f.footnotes {
		index := i + 1
		citation := fmt.Sprintf("[%d]", index)
		fmt.Fprintf(buf, "%-4s %s\n", citation, footnote)
	}
	return buf.String()
}
