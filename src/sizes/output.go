package sizes

import (
	"bytes"
	"fmt"
	"io"
	"math"
)

func (s BlobSize) String() string {
	return fmt.Sprintf("blob_size=%d", s.Size)
}

func (s TreeSize) String() string {
	return fmt.Sprintf(
		"max_path_depth=%d, max_path_length=%d, "+
			"expanded_tree_count=%d, "+
			"expanded_blob_count=%d, expanded_blob_size=%d, "+
			"expanded_link_count=%d, expanded_submodule_count=%d",
		s.MaxPathDepth, s.MaxPathLength,
		s.ExpandedTreeCount,
		s.ExpandedBlobCount, s.ExpandedBlobSize,
		s.ExpandedLinkCount, s.ExpandedSubmoduleCount,
	)
}

func (s CommitSize) String() string {
	return fmt.Sprintf(
		"max_ancestor_depth=%d",
		s.MaxAncestorDepth,
	)
}

func (s TagSize) String() string {
	return fmt.Sprintf("tag_depth=%d", s.TagDepth)
}

func (s HistorySize) String() string {
	return fmt.Sprintf(
		"unique_commit_count=%d, unique_commit_count = %d, max_commit_size = %d, "+
			"max_history_depth=%d, max_parent_count=%d, "+
			"unique_tree_count=%d, unique_tree_entries=%d, max_tree_entries=%d, "+
			"unique_blob_count=%d, unique_blob_size=%d, max_blob_size=%d, "+
			"unique_tag_count=%d, "+
			"reference_count=%d, "+
			"max_path_depth=%d, max_path_length=%d, "+
			"max_expanded_tree_count=%d, "+
			"max_expanded_blob_count=%d, max_expanded_blob_size=%d, "+
			"max_expanded_link_count=%d, max_expanded_submodule_count=%d",
		s.UniqueCommitCount, s.UniqueCommitSize, s.MaxCommitSize,
		s.MaxHistoryDepth, s.MaxParentCount,
		s.UniqueTreeCount, s.UniqueTreeEntries, s.MaxTreeEntries,
		s.UniqueBlobCount, s.UniqueBlobSize, s.MaxBlobSize,
		s.UniqueTagCount,
		s.ReferenceCount,
		s.MaxPathDepth, s.MaxPathLength,
		s.MaxExpandedTreeCount, s.MaxExpandedBlobCount,
		s.MaxExpandedBlobSize, s.MaxExpandedLinkCount,
		s.MaxExpandedSubmoduleCount,
	)
}

type Prefix struct {
	Name       string
	Multiplier uint64
}

type Humaner interface {
	Human([]Prefix, string) (string, string)
	ToUint64() uint64
}

var MetricPrefixes []Prefix

func init() {
	MetricPrefixes = []Prefix{
		{"", 1},
		{"k", 1e3},
		{"M", 1e6},
		{"G", 1e9},
		{"T", 1e12},
		{"P", 1e15},
	}
}

var BinaryPrefixes []Prefix

func init() {
	BinaryPrefixes = []Prefix{
		{"", 1 << (10 * 0)},
		{"Ki", 1 << (10 * 1)},
		{"Mi", 1 << (10 * 2)},
		{"Gi", 1 << (10 * 3)},
		{"Ti", 1 << (10 * 4)},
		{"Pi", 1 << (10 * 5)},
	}
}

// Format values, aligned, in `len(unit) + 10` or fewer characters
// (except for extremely large numbers).
func Human(n uint64, prefixes []Prefix, unit string) (string, string) {
	prefix := prefixes[0]
	wholePart := n
	for _, p := range prefixes {
		w := n / p.Multiplier
		if w >= 1 {
			wholePart = w
			prefix = p
		}
	}

	if prefix.Multiplier == 1 {
		return fmt.Sprintf("%d", n), unit
	} else {
		mantissa := float64(n) / float64(prefix.Multiplier)
		var format string

		if wholePart >= 100 {
			// `mantissa` can actually be up to 1023.999.
			format = "%.0f"
		} else if wholePart >= 10 {
			format = "%.1f"
		} else {
			format = "%.2f"
		}
		return fmt.Sprintf(format, mantissa), prefix.Name + unit
	}
}

func (n Count32) Human(prefixes []Prefix, unit string) (string, string) {
	if n == math.MaxUint32 {
		return "∞", ""
	} else {
		return Human(uint64(n), prefixes, unit)
	}
}

func (n Count64) Human(prefixes []Prefix, unit string) (string, string) {
	if n == math.MaxUint64 {
		return "∞", unit
	} else {
		return Human(uint64(n), prefixes, unit)
	}
}

const (
	spaces = "                            "
	stars  = "******************************"
)

// A set of lines in the tabular output.
type lineSet interface {
	Emit(t *table, buf io.Writer, indent int)
}

// A section of lines in the tabular output, consisting of a header
// and a number of bullet lines. The lines in a section can themselves
// be bulletized, in which case the header becomes a top-level bullet
// and the lines become second-level bullets.
type section struct {
	name     string
	lineSets []lineSet
}

func newSection(name string, lineSets ...lineSet) *section {
	return &section{name, lineSets}
}

func (s *section) Emit(t *table, buf io.Writer, indent int) {
	t.emitRow(buf, indent, s.name, "", "", "", "")
	for _, ls := range s.lineSets {
		ls.Emit(t, buf, indent+1)
	}
}

// A line containing data in the tabular output.
type item struct {
	name     string
	path     *Path
	value    Humaner
	prefixes []Prefix
	unit     string
	scale    float64
}

func newItem(
	name string,
	path *Path,
	value Humaner,
	prefixes []Prefix,
	unit string,
	scale float64,
) *item {
	return &item{
		name:     name,
		path:     path,
		value:    value,
		prefixes: prefixes,
		unit:     unit,
		scale:    scale,
	}
}

func (l *item) Emit(t *table, buf io.Writer, indent int) {
	valueString, unitString := l.value.Human(l.prefixes, l.unit)
	t.emitRow(
		buf,
		indent,
		l.name, l.Footnote(t.nameStyle),
		valueString, unitString,
		l.LevelOfConcern(),
	)
}

func (l *item) Footnote(nameStyle NameStyle) string {
	if l.path == nil || l.path.Oid == NullOid {
		return ""
	}
	switch nameStyle {
	case NameStyleNone:
		return ""
	case NameStyleHash:
		return l.path.Oid.String()
	case NameStyleFull:
		return l.path.String()
	default:
		panic("unexpected NameStyle")
	}
}

func (l *item) LevelOfConcern() string {
	var warning string
	if l.scale == 0 {
		warning = ""
	} else {
		alert := float64(l.value.ToUint64()) / l.scale
		if alert > 30 {
			warning = "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
		} else {
			alert := int(alert)
			warning = stars[:alert]
		}
	}
	return warning
}

type NameStyle int

const (
	NameStyleNone NameStyle = iota
	NameStyleHash
	NameStyleFull
)

// Methods to implement flag.Value:
func (n *NameStyle) String() string {
	if n == nil {
		return "UNSET"
	} else {
		switch *n {
		case NameStyleNone:
			return "none"
		case NameStyleHash:
			return "hash"
		case NameStyleFull:
			return "full"
		default:
			panic("Unexpected NameStyle value")
		}
	}
}

func (n *NameStyle) Set(s string) error {
	switch s {
	case "none":
		*n = NameStyleNone
	case "hash", "sha-1", "sha1":
		*n = NameStyleHash
	case "full":
		*n = NameStyleFull
	default:
		return fmt.Errorf("not a valid name style: %v", s)
	}
	return nil
}

type table struct {
	contents        []lineSet
	nameStyle       NameStyle
	footnotes       []string
	footnoteIndexes map[string]int
}

func (s HistorySize) TableString(nameStyle NameStyle) string {
	S := newSection
	I := newItem
	t := &table{
		contents: []lineSet{
			S(
				"Overall repository size",
				S(
					"Commits",
					I("Count", nil, s.UniqueCommitCount, MetricPrefixes, " ", 500e3),
					I("Total size", nil, s.UniqueCommitSize, BinaryPrefixes, "B", 250e6),
				),

				S(
					"Trees",
					I("Count", nil, s.UniqueTreeCount, MetricPrefixes, " ", 1.5e6),
					I("Total size", nil, s.UniqueTreeSize, BinaryPrefixes, "B", 2e9),
					I("Total tree entries", nil, s.UniqueTreeEntries, MetricPrefixes, " ", 50e6),
				),

				S(
					"Blobs",
					I("Count", nil, s.UniqueBlobCount, MetricPrefixes, " ", 1.5e6),
					I("Total size", nil, s.UniqueBlobSize, BinaryPrefixes, "B", 10e9),
				),

				S(
					"Annotated tags",
					I("Count", nil, s.UniqueTagCount, MetricPrefixes, " ", 25e3),
				),

				S(
					"References",
					I("Count", nil, s.ReferenceCount, MetricPrefixes, " ", 25e3),
				),
			),

			S(
				"Biggest commit objects",
				I("Maximum size", s.MaxCommitSizeCommit, s.MaxCommitSize, BinaryPrefixes, "B", 50e3),
				I("Maximum parents", s.MaxParentCountCommit, s.MaxParentCount, MetricPrefixes, " ", 10),
			),

			S(
				"Biggest tree objects",
				I("Maximum tree entries", s.MaxTreeEntriesTree, s.MaxTreeEntries, MetricPrefixes, " ", 2.5e3),
			),

			S(
				"Biggest blob objects",
				I("Maximum size", s.MaxBlobSizeBlob, s.MaxBlobSize, BinaryPrefixes, "B", 10e6),
			),

			S(
				"History structure",
				I("Maximum history depth", nil, s.MaxHistoryDepth, MetricPrefixes, " ", 500e3),
				I("Maximum tag depth", s.MaxTagDepthTag, s.MaxTagDepth, MetricPrefixes, " ", 1),
			),

			S(
				"Biggest checkouts",
				I("Number of directories", s.MaxExpandedTreeCountTree, s.MaxExpandedTreeCount, MetricPrefixes, " ", 2000),
				I("Maximum path depth", s.MaxPathDepthTree, s.MaxPathDepth, MetricPrefixes, " ", 10),
				I("Maximum path length", s.MaxPathLengthTree, s.MaxPathLength, BinaryPrefixes, "B", 100),

				I("Number of files", s.MaxExpandedBlobCountTree, s.MaxExpandedBlobCount, MetricPrefixes, " ", 50e3),
				I("Total size of files", s.MaxExpandedBlobSizeTree, s.MaxExpandedBlobSize, BinaryPrefixes, "B", 1e9),

				I("Number of symlinks", s.MaxExpandedLinkCountTree, s.MaxExpandedLinkCount, MetricPrefixes, " ", 25e3),

				I("Number of submodules", s.MaxExpandedSubmoduleCountTree, s.MaxExpandedSubmoduleCount, MetricPrefixes, " ", 100),
			),
		},
		nameStyle:       nameStyle,
		footnoteIndexes: make(map[string]int),
	}

	return t.String()
}

func (t *table) String() string {
	header := t.generateHeader()
	lines := t.generateLines()
	footnotes := t.generateFootnotes()
	return header + lines + footnotes
}

func (t *table) generateHeader() string {
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "| Name                         | Value     | Level of concern               |")
	fmt.Fprintln(buf, "| ---------------------------- | --------- | ------------------------------ |")
	return buf.String()
}

func (t *table) generateLines() string {
	buf := &bytes.Buffer{}
	linesEmitted := false
	for _, ls := range t.contents {
		if linesEmitted {
			t.emitBlankRow(buf)
		}
		ls.Emit(t, buf, 0)
		linesEmitted = true
	}
	return buf.String()
}

func (t *table) emitBlankRow(buf io.Writer) {
	t.emitRow(buf, 0, "", "", "", "", "")
}

func (t *table) emitRow(
	buf io.Writer, indent int,
	name, footnote, valueString, unitString, levelOfConcern string,
) {
	prefix := ""
	if indent != 0 {
		prefix = spaces[:2*(indent-1)] + "* "
	}
	citation := t.createCitation(footnote)
	spacer := ""
	l := len(prefix) + len(name) + len(citation)
	if l < 28 {
		spacer = spaces[:28-l]
	}
	fmt.Fprintf(
		buf, "| %s%s%s%s | %5s %-3s | %-30s |\n",
		prefix, name, spacer, citation, valueString, unitString, levelOfConcern,
	)
}

func (t *table) createCitation(footnote string) string {
	if footnote == "" {
		return ""
	}

	index, ok := t.footnoteIndexes[footnote]
	if !ok {
		index = len(t.footnoteIndexes) + 1
		t.footnotes = append(t.footnotes, footnote)
		t.footnoteIndexes[footnote] = index
	}
	return fmt.Sprintf("[%d]", index)
}

func (t *table) generateFootnotes() string {
	if len(t.footnotes) == 0 {
		return ""
	}

	buf := &bytes.Buffer{}
	buf.WriteByte('\n')
	for i, footnote := range t.footnotes {
		index := i + 1
		citation := fmt.Sprintf("[%d]", index)
		fmt.Fprintf(buf, "%-4s %s\n", citation, footnote)
	}
	return buf.String()
}
