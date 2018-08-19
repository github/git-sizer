package sizes

import (
	"bytes"
	"fmt"
)

type Footnotes struct {
	footnotes []string
	indexes   map[string]int
}

func NewFootnotes() *Footnotes {
	return &Footnotes{
		indexes: make(map[string]int),
	}
}

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
