package sizes

import (
	"bytes"
	"fmt"
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
	Lines() []line
}

// A single line in the tabular output.
type line interface {
	Name() string
	Footnote(NameStyle) string
	Value() (string, string)
	LevelOfConcern() string
}

// A blank line in the tabular output.
type blank struct {
}

func (l *blank) Lines() []line {
	return []line{l}
}

func (l *blank) Name() string {
	return ""
}

func (l *blank) Footnote(_ NameStyle) string {
	return ""
}

func (l *blank) Value() (string, string) {
	return "", ""
}

func (l *blank) LevelOfConcern() string {
	return ""
}

// A header line in the tabular output.
type header struct {
	name string
}

func (l *header) Name() string {
	return l.name
}

func (l *header) Footnote(_ NameStyle) string {
	return ""
}

func (l *header) Value() (string, string) {
	return "", ""
}

func (l *header) LevelOfConcern() string {
	return ""
}

// A bullet point in the tabular output.
type bullet struct {
	prefix string
	line   line
}

// Turn `line` into a `bullet`. If it is already a bullet, just
// increase its level of indentation. Leave blank lines unchanged.
func newBullet(line line) line {
	switch line := line.(type) {
	case *bullet:
		return &bullet{"  " + line.prefix, line.line}
	case *blank:
		return line
	default:
		return &bullet{"* ", line}
	}
}

func (l *bullet) Name() string {
	return l.prefix + l.line.Name()
}

func (l *bullet) Footnote(nameStyle NameStyle) string {
	return l.line.Footnote(nameStyle)
}

func (l *bullet) Value() (string, string) {
	return l.line.Value()
}

func (l *bullet) LevelOfConcern() string {
	return l.line.LevelOfConcern()
}

// A section of lines in the tabular output, consisting of a header
// and a number of bullet lines. The lines in a section can themselves
// be bulletized, in which case the header becomes a top-level bullet
// and the lines become second-level bullets.
type section struct {
	name     string
	lineSets []lineSet
}

func (s *section) Lines() []line {
	var lines []line
	lines = append(lines, &header{s.name})
	for _, ls := range s.lineSets {
		for _, l := range ls.Lines() {
			lines = append(lines, newBullet(l))
		}
	}
	return lines
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

func (l *item) Lines() []line {
	return []line{l}
}

func (l *item) Name() string {
	return l.name
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

func (l *item) Value() (string, string) {
	return l.value.Human(l.prefixes, l.unit)
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

func (s HistorySize) TableString(nameStyle NameStyle) string {
	buf := &bytes.Buffer{}

	fmt.Fprintln(buf, "| Name                         | Value     | Level of concern               |")
	fmt.Fprintln(buf, "| ---------------------------- | --------- | ------------------------------ |")

	var footnotes []string
	footnoteIndexes := make(map[string]int)

	for _, ls := range []lineSet{
		&section{"Overall repository size",
			[]lineSet{
				&section{"Commits",
					[]lineSet{
						&item{"Count", nil, s.UniqueCommitCount, MetricPrefixes, " ", 500e3},
						&item{"Total size", nil, s.UniqueCommitSize, BinaryPrefixes, "B", 250e6},
					},
				},

				&section{"Trees",
					[]lineSet{
						&item{"Count", nil, s.UniqueTreeCount, MetricPrefixes, " ", 1.5e6},
						&item{"Total size", nil, s.UniqueTreeSize, BinaryPrefixes, "B", 2e9},
						&item{"Total tree entries", nil, s.UniqueTreeEntries, MetricPrefixes, " ", 50e6},
					},
				},

				&section{"Blobs",
					[]lineSet{
						&item{"Count", nil, s.UniqueBlobCount, MetricPrefixes, " ", 1.5e6},
						&item{"Total size", nil, s.UniqueBlobSize, BinaryPrefixes, "B", 10e9},
					},
				},

				&section{"Annotated tags",
					[]lineSet{
						&item{"Count", nil, s.UniqueTagCount, MetricPrefixes, " ", 25e3},
					},
				},

				&section{"References",
					[]lineSet{
						&item{"Count", nil, s.ReferenceCount, MetricPrefixes, " ", 25e3},
					},
				},
			},
		},
		&blank{},

		&section{"Biggest commit objects",
			[]lineSet{
				&item{"Maximum size", s.MaxCommitSizeCommit, s.MaxCommitSize, BinaryPrefixes, "B", 50e3},
				&item{"Maximum parents", s.MaxParentCountCommit, s.MaxParentCount, MetricPrefixes, " ", 10},
			},
		},
		&blank{},

		&section{"Biggest tree objects",
			[]lineSet{
				&item{"Maximum tree entries", s.MaxTreeEntriesTree, s.MaxTreeEntries, MetricPrefixes, " ", 2.5e3},
			},
		},
		&blank{},

		&section{"Biggest blob objects",
			[]lineSet{
				&item{"Maximum size", s.MaxBlobSizeBlob, s.MaxBlobSize, BinaryPrefixes, "B", 10e6},
			},
		},
		&blank{},

		&section{"History structure",
			[]lineSet{
				&item{"Maximum history depth", nil, s.MaxHistoryDepth, MetricPrefixes, " ", 500e3},
				&item{"Maximum tag depth", s.MaxTagDepthTag, s.MaxTagDepth, MetricPrefixes, " ", 1},
			},
		},
		&blank{},

		&section{"Biggest checkouts",
			[]lineSet{
				&item{"Number of directories", s.MaxExpandedTreeCountTree, s.MaxExpandedTreeCount, MetricPrefixes, " ", 2000},
				&item{"Maximum path depth", s.MaxPathDepthTree, s.MaxPathDepth, MetricPrefixes, " ", 10},
				&item{"Maximum path length", s.MaxPathLengthTree, s.MaxPathLength, BinaryPrefixes, "B", 100},

				&item{"Number of files", s.MaxExpandedBlobCountTree, s.MaxExpandedBlobCount, MetricPrefixes, " ", 50e3},
				&item{"Total size of files", s.MaxExpandedBlobSizeTree, s.MaxExpandedBlobSize, BinaryPrefixes, "B", 1e9},

				&item{"Number of symlinks", s.MaxExpandedLinkCountTree, s.MaxExpandedLinkCount, MetricPrefixes, " ", 25e3},

				&item{"Number of submodules", s.MaxExpandedSubmoduleCountTree, s.MaxExpandedSubmoduleCount, MetricPrefixes, " ", 100},
			},
		},
	} {
		for _, l := range ls.Lines() {
			valueString, unitString := l.Value()
			footnote := l.Footnote(nameStyle)
			var citation string
			if footnote == "" {
				citation = ""
			} else {
				index, ok := footnoteIndexes[footnote]
				if !ok {
					index = len(footnoteIndexes) + 1
					footnotes = append(footnotes, footnote)
					footnoteIndexes[footnote] = index
				}
				citation = fmt.Sprintf("[%d]", index)
			}
			nameString := l.Name()
			if len(nameString)+len(citation) < 28 {
				nameString += spaces[:28-len(nameString)-len(citation)]
			}
			nameString += citation
			fmt.Fprintf(buf, "| %s | %5s %-3s | %-30s |\n", nameString, valueString, unitString, l.LevelOfConcern())
		}
	}

	// Output the footnotes:
	buf.WriteByte('\n')
	for i, footnote := range footnotes {
		index := i + 1
		citation := fmt.Sprintf("[%d]", index)
		fmt.Fprintf(buf, "%-4s %s\n", citation, footnote)
	}

	return buf.String()
}
