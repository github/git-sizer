package sizes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/github/git-sizer/counts"
	"github.com/github/git-sizer/git"

	"github.com/spf13/pflag"
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

const (
	spaces = "                            "
	stars  = "******************************"
)

// Zero or more lines in the tabular output.
type tableContents interface {
	Emit(t *table)
	CollectItems(items map[string]*item)
}

// A section of lines in the tabular output, consisting of a header
// and a number of bullet lines. The lines in a section can themselves
// be bulletized, in which case the header becomes a top-level bullet
// and the lines become second-level bullets.
type section struct {
	name     string
	contents []tableContents
}

func newSection(name string, contents ...tableContents) *section {
	return &section{
		name:     name,
		contents: contents,
	}
}

func (s *section) Emit(t *table) {
	for _, c := range s.contents {
		subTable := t.subTable(s.name)
		c.Emit(subTable)
		t.addSection(subTable)
	}
}

func (s *section) CollectItems(items map[string]*item) {
	for _, c := range s.contents {
		c.CollectItems(items)
	}
}

// A line containing data in the tabular output.
type item struct {
	symbol      string
	name        string
	description string
	path        *Path
	value       counts.Humanable
	humaner     counts.Humaner
	unit        string
	scale       float64
}

func newItem(
	symbol string,
	name string,
	description string,
	path *Path,
	value counts.Humanable,
	humaner counts.Humaner,
	unit string,
	scale float64,
) *item {
	return &item{
		symbol:      symbol,
		name:        name,
		description: description,
		path:        path,
		value:       value,
		humaner:     humaner,
		unit:        unit,
		scale:       scale,
	}
}

func (l *item) Emit(t *table) {
	levelOfConcern, interesting := l.levelOfConcern(t.threshold)
	if !interesting {
		return
	}
	valueString, unitString := l.humaner.Format(l.value, l.unit)
	t.formatRow(
		l.name, t.footnotes.CreateCitation(l.Footnote(t.nameStyle)),
		valueString, unitString,
		levelOfConcern,
	)
}

func (l *item) Footnote(nameStyle NameStyle) string {
	if l.path == nil || l.path.OID == git.NullOID {
		return ""
	}
	switch nameStyle {
	case NameStyleNone:
		return ""
	case NameStyleHash:
		return l.path.OID.String()
	case NameStyleFull:
		return l.path.String()
	default:
		panic("unexpected NameStyle")
	}
}

// If this item's alert level is at least as high as the threshold,
// return the string that should be used as its "level of concern" and
// `true`; otherwise, return `"", false`.
func (l *item) levelOfConcern(threshold Threshold) (string, bool) {
	value, _ := l.value.ToUint64()
	alert := Threshold(float64(value) / l.scale)
	if alert < threshold {
		return "", false
	}
	if alert > 30 {
		return "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", true
	}
	return stars[:int(alert)], true
}

func (i *item) CollectItems(items map[string]*item) {
	items[i.symbol] = i
}

func (i *item) MarshalJSON() ([]byte, error) {
	// How we want to emit an item as JSON.
	value, _ := i.value.ToUint64()

	stat := struct {
		Description       string  `json:"description"`
		Value             uint64  `json:"value"`
		Unit              string  `json:"unit"`
		Prefixes          string  `json:"prefixes"`
		ReferenceValue    float64 `json:"referenceValue"`
		LevelOfConcern    float64 `json:"levelOfConcern"`
		ObjectName        string  `json:"objectName,omitempty"`
		ObjectDescription string  `json:"objectDescription,omitempty"`
	}{
		Description:    i.description,
		Value:          value,
		Unit:           i.unit,
		Prefixes:       i.humaner.Name(),
		ReferenceValue: i.scale,
		LevelOfConcern: float64(value) / i.scale,
	}

	if i.path != nil && i.path.OID != git.NullOID {
		stat.ObjectName = i.path.OID.String()
		stat.ObjectDescription = i.path.Path()
	}

	return json.Marshal(stat)
}

type Threshold float64

// Methods to implement pflag.Value:
func (t *Threshold) String() string {
	if t == nil {
		return "UNSET"
	} else {
		switch *t {
		case 0:
			return "--verbose"
		case 1:
			return "--threshold=1"
		case 30:
			return "--critical"
		default:
			return fmt.Sprintf("--threshold=%g", *t)
		}
	}
}

func (t *Threshold) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("error parsing floating-point value %q: %s", s, err)
	}
	*t = Threshold(v)
	return nil
}

func (t *Threshold) Type() string {
	return "threshold"
}

// A `pflag.Value` that can be used as a boolean option that sets a
// `Threshold` variable to a fixed value. For example,
//
//		pflag.Var(
//			sizes.NewThresholdFlagValue(&threshold, 30),
//			"critical", "only report critical statistics",
//		)
//
// adds a `--critical` flag that sets `threshold` to 30.
type thresholdFlagValue struct {
	b         bool
	threshold *Threshold
	value     Threshold
}

func NewThresholdFlagValue(threshold *Threshold, value Threshold) pflag.Value {
	return &thresholdFlagValue{false, threshold, value}
}

func (v *thresholdFlagValue) String() string {
	return strconv.FormatBool(v.b)
}

func (v *thresholdFlagValue) Set(s string) error {
	value, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	v.b = value
	if value {
		*v.threshold = v.value
	} else {
		*v.threshold = 1
	}
	return nil
}

func (v *thresholdFlagValue) Type() string {
	return "bool"
}

type NameStyle int

const (
	NameStyleNone NameStyle = iota
	NameStyleHash
	NameStyleFull
)

// Methods to implement pflag.Value:
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

func (n *NameStyle) Type() string {
	return "nameStyle"
}

type table struct {
	threshold     Threshold
	nameStyle     NameStyle
	sectionHeader string
	footnotes     *Footnotes
	indent        int
	buf           bytes.Buffer
}

func (s HistorySize) TableString(threshold Threshold, nameStyle NameStyle) string {
	contents := s.contents()
	t := table{
		threshold: threshold,
		nameStyle: nameStyle,
		footnotes: NewFootnotes(),
		indent:    -1,
	}

	contents.Emit(&t)

	if t.buf.Len() == 0 {
		return "No problems above the current threshold were found\n"
	}

	return t.generateHeader() + t.buf.String() + t.footnotes.String()
}

func (t *table) subTable(sectionHeader string) *table {
	return &table{
		threshold:     t.threshold,
		nameStyle:     t.nameStyle,
		sectionHeader: sectionHeader,
		footnotes:     t.footnotes,
		indent:        t.indent + 1,
	}
}

func (t *table) addSection(subTable *table) {
	if subTable.buf.Len() > 0 {
		if t.buf.Len() == 0 {
			// Add the section title:
			if subTable.sectionHeader != "" {
				t.formatSectionHeader(subTable.sectionHeader)
			}
		} else if t.indent == -1 {
			// The top-level section gets blank lines between its
			// subsections:
			t.emitBlankRow()
		}
		fmt.Fprint(&t.buf, subTable.buf.String())
	}
}

func (t *table) generateHeader() string {
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "| Name                         | Value     | Level of concern               |")
	fmt.Fprintln(buf, "| ---------------------------- | --------- | ------------------------------ |")
	return buf.String()
}

func (t *table) emitBlankRow() {
	fmt.Fprintln(&t.buf, "|                              |           |                                |")
}

func (t *table) formatSectionHeader(name string) {
	t.formatRow(name, "", "", "", "")
}

func (t *table) formatRow(
	name, citation, valueString, unitString, levelOfConcern string,
) {
	prefix := ""
	if t.indent != 0 {
		prefix = spaces[:2*(t.indent-1)] + "* "
	}
	spacer := ""
	l := len(prefix) + len(name) + len(citation)
	if l < 28 {
		spacer = spaces[:28-l]
	}
	fmt.Fprintf(
		&t.buf, "| %s%s%s%s | %5s %-3s | %-30s |\n",
		prefix, name, spacer, citation, valueString, unitString, levelOfConcern,
	)
}

func (s HistorySize) JSON(threshold Threshold, nameStyle NameStyle) ([]byte, error) {
	contents := s.contents()
	items := make(map[string]*item)
	contents.CollectItems(items)
	j, err := json.MarshalIndent(items, "", "    ")
	return j, err
}

func (s HistorySize) contents() tableContents {
	S := newSection
	I := newItem
	metric := counts.Metric
	binary := counts.Binary
	return S(
		"",
		S(
			"Overall repository size",
			S(
				"Commits",
				I("uniqueCommitCount", "Count",
					"The total number of distinct commit objects",
					nil, s.UniqueCommitCount, metric, "", 500e3),
				I("uniqueCommitSize", "Total size",
					"The total size of all commit objects",
					nil, s.UniqueCommitSize, binary, "B", 250e6),
			),

			S(
				"Trees",
				I("uniqueTreeCount", "Count",
					"The total number of distinct tree objects",
					nil, s.UniqueTreeCount, metric, "", 1.5e6),
				I("uniqueTreeSize", "Total size",
					"The total size of all distinct tree objects",
					nil, s.UniqueTreeSize, binary, "B", 2e9),
				I("uniqueTreeEntries", "Total tree entries",
					"The total number of entries in all distinct tree objects",
					nil, s.UniqueTreeEntries, metric, "", 50e6),
			),

			S(
				"Blobs",
				I("uniqueBlobCount", "Count",
					"The total number of distinct blob objects",
					nil, s.UniqueBlobCount, metric, "", 1.5e6),
				I("uniqueBlobSize", "Total size",
					"The total size of all distinct blob objects",
					nil, s.UniqueBlobSize, binary, "B", 10e9),
			),

			S(
				"Annotated tags",
				I("uniqueTagCount", "Count",
					"The total number of annotated tags",
					nil, s.UniqueTagCount, metric, "", 25e3),
			),

			S(
				"References",
				I("referenceCount", "Count",
					"The total number of references",
					nil, s.ReferenceCount, metric, "", 25e3),
			),
		),

		S("Biggest objects",
			S("Commits",
				I("maxCommitSize", "Maximum size",
					"The size of the largest single commit",
					s.MaxCommitSizeCommit, s.MaxCommitSize, binary, "B", 50e3),
				I("maxCommitParentCount", "Maximum parents",
					"The most parents of any single commit",
					s.MaxParentCountCommit, s.MaxParentCount, metric, "", 10),
			),

			S("Trees",
				I("maxTreeEntries", "Maximum entries",
					"The most entries in any single tree",
					s.MaxTreeEntriesTree, s.MaxTreeEntries, metric, "", 1000),
			),

			S("Blobs",
				I("maxBlobSize", "Maximum size",
					"The size of the largest blob object",
					s.MaxBlobSizeBlob, s.MaxBlobSize, binary, "B", 10e6),
			),
		),

		S("History structure",
			I("maxHistoryDepth", "Maximum history depth",
				"The longest chain of commits in history",
				nil, s.MaxHistoryDepth, metric, "", 500e3),
			I("maxTagDepth", "Maximum tag depth",
				"The longest chain of annotated tags pointing at one another",
				s.MaxTagDepthTag, s.MaxTagDepth, metric, "", 1.001),
		),

		S("Biggest checkouts",
			I("maxCheckoutTreeCount", "Number of directories",
				"The number of directories in the largest checkout",
				s.MaxExpandedTreeCountTree, s.MaxExpandedTreeCount, metric, "", 2000),
			I("maxCheckoutPathDepth", "Maximum path depth",
				"The maximum path depth in any checkout",
				s.MaxPathDepthTree, s.MaxPathDepth, metric, "", 10),
			I("maxCheckoutPathLength", "Maximum path length",
				"The maximum path length in any checkout",
				s.MaxPathLengthTree, s.MaxPathLength, binary, "B", 100),

			I("maxCheckoutBlobCount", "Number of files",
				"The maximum number of files in any checkout",
				s.MaxExpandedBlobCountTree, s.MaxExpandedBlobCount, metric, "", 50e3),
			I("maxCheckoutBlobSize", "Total size of files",
				"The maximum sum of file sizes in any checkout",
				s.MaxExpandedBlobSizeTree, s.MaxExpandedBlobSize, binary, "B", 1e9),

			I("maxCheckoutLinkCount", "Number of symlinks",
				"The maximum number of symlinks in any checkout",
				s.MaxExpandedLinkCountTree, s.MaxExpandedLinkCount, metric, "", 25e3),

			I("maxCheckoutSubmoduleCount", "Number of submodules",
				"The maximum number of submodules in any checkout",
				s.MaxExpandedSubmoduleCountTree, s.MaxExpandedSubmoduleCount, metric, "", 100),
		),
	)
}
