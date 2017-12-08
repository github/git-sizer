package sizes

import (
	"bytes"
	"fmt"
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

type item struct {
	Name     string
	Value    Humaner
	Prefixes []Prefix
	Unit     string
	Scale    float64
}

func (s HistorySize) TableString() string {
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "| Name                         | Value     | Level of concern               |")
	fmt.Fprintln(buf, "| ---------------------------- | --------- | ------------------------------ |")
	stars := "******************************"
	for _, i := range []item{
		{"unique_commit_count", s.UniqueCommitCount, MetricPrefixes, " ", 500e3},
		{"unique_commit_size", s.UniqueCommitSize, BinaryPrefixes, "B", 250e6},
		{"max_commit_size", s.MaxCommitSize, BinaryPrefixes, "B", 50e3},
		{"max_history_depth", s.MaxHistoryDepth, MetricPrefixes, " ", 500e3},
		{"max_parent_count", s.MaxParentCount, MetricPrefixes, " ", 10},
		{"unique_tree_count", s.UniqueTreeCount, MetricPrefixes, " ", 1.5e6},
		{"unique_tree_size", s.UniqueTreeSize, BinaryPrefixes, "B", 2e9},
		{"unique_tree_entries", s.UniqueTreeEntries, MetricPrefixes, " ", 50e6},
		{"max_tree_entries", s.MaxTreeEntries, MetricPrefixes, " ", 2.5e3},
		{"unique_blob_count", s.UniqueBlobCount, MetricPrefixes, " ", 1.5e6},
		{"unique_blob_size", s.UniqueBlobSize, BinaryPrefixes, "B", 10e9},
		{"max_blob_size", s.MaxBlobSize, BinaryPrefixes, "B", 10e6},
		{"unique_tag_count", s.UniqueTagCount, MetricPrefixes, " ", 25e3},
		{"max_tag_depth", s.MaxTagDepth, MetricPrefixes, " ", 1},
		{"reference_count", s.ReferenceCount, MetricPrefixes, " ", 25e3},
		{"max_path_depth", s.MaxPathDepth, MetricPrefixes, " ", 10},
		{"max_path_length", s.MaxPathLength, BinaryPrefixes, "B", 100},
		{"max_expanded_tree_count", s.MaxExpandedTreeCount, MetricPrefixes, " ", 2000},
		{"max_expanded_blob_count", s.MaxExpandedBlobCount, MetricPrefixes, " ", 50e3},
		{"max_expanded_blob_size", s.MaxExpandedBlobSize, BinaryPrefixes, "B", 1e9},
		{"max_expanded_link_count", s.MaxExpandedLinkCount, MetricPrefixes, " ", 25e3},
		{"max_expanded_submodule_count", s.MaxExpandedSubmoduleCount, MetricPrefixes, " ", 100},
	} {
		valueString, unitString := i.Value.Human(i.Prefixes, i.Unit)
		var warning string
		if i.Scale == 0 {
			warning = ""
		} else {
			alert := float64(i.Value.ToUint64()) / i.Scale
			if alert > 30 {
				warning = "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
			} else {
				alert := int(alert)
				warning = stars[:alert]
			}
		}
		fmt.Fprintf(buf, "| %-28s | %5s %-3s | %-30s |\n", i.Name, valueString, unitString, warning)
	}
	return buf.String()
}
