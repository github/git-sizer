package sizes

import (
	"bytes"
	"fmt"
)

type Size interface {
	fmt.Stringer
}

type BlobSize struct {
	Size Count32
}

func (s BlobSize) String() string {
	return fmt.Sprintf("blob_size=%d", s.Size)
}

type TreeSize struct {
	// The maximum depth of trees and blobs starting at this object
	// (including this object).
	MaxPathDepth Count32 `json:"max_path_depth"`

	// The maximum length of any path relative to this object, in
	// characters.
	MaxPathLength Count32 `json:"max_path_length"`

	// The total number of trees, including duplicates.
	ExpandedTreeCount Count32 `json:"expanded_tree_count"`

	// The total number of blobs, including duplicates.
	ExpandedBlobCount Count32 `json:"expanded_blob_count"`

	// The total size of all blobs, including duplicates.
	ExpandedBlobSize Count64 `json:"expanded_blob_size"`

	// The total number of symbolic links, including duplicates.
	ExpandedLinkCount Count32 `json:"expanded_link_count"`

	// The total number of submodules referenced, including duplicates.
	ExpandedSubmoduleCount Count32 `json:"expanded_submodule_count"`
}

func (s *TreeSize) addDescendent(filename string, s2 TreeSize) {
	s.MaxPathDepth.AdjustMax(s2.MaxPathDepth)
	if s2.MaxPathLength > 0 {
		s.MaxPathLength.AdjustMax((NewCount32(uint64(len(filename))) + 1).Plus(s2.MaxPathLength))
	} else {
		s.MaxPathLength.AdjustMax(NewCount32(uint64(len(filename))))
	}
	s.ExpandedTreeCount.Increment(s2.ExpandedTreeCount)
	s.ExpandedBlobCount.Increment(s2.ExpandedBlobCount)
	s.ExpandedBlobSize.Increment(s2.ExpandedBlobSize)
	s.ExpandedLinkCount.Increment(s2.ExpandedLinkCount)
	s.ExpandedSubmoduleCount.Increment(s2.ExpandedSubmoduleCount)
}

func (s *TreeSize) adjustMaxima(s2 TreeSize) {
	s.MaxPathDepth.AdjustMax(s2.MaxPathDepth)
	s.MaxPathLength.AdjustMax(s2.MaxPathLength)
	s.ExpandedTreeCount.AdjustMax(s2.ExpandedTreeCount)
	s.ExpandedBlobCount.AdjustMax(s2.ExpandedBlobCount)
	s.ExpandedBlobSize.AdjustMax(s2.ExpandedBlobSize)
	s.ExpandedLinkCount.AdjustMax(s2.ExpandedLinkCount)
	s.ExpandedSubmoduleCount.AdjustMax(s2.ExpandedSubmoduleCount)
}

// Record that the object has a blob of the specified `size` as a
// direct descendant.
func (s *TreeSize) addBlob(filename string, size BlobSize) {
	s.MaxPathDepth.AdjustMax(1)
	s.MaxPathLength.AdjustMax(NewCount32(uint64(len(filename))))
	s.ExpandedBlobSize.Increment(Count64(size.Size))
	s.ExpandedBlobCount.Increment(1)
}

// Record that the object has a link as a direct descendant.
func (s *TreeSize) addLink(filename string) {
	s.MaxPathDepth.AdjustMax(1)
	s.MaxPathLength.AdjustMax(NewCount32(uint64(len(filename))))
	s.ExpandedLinkCount.Increment(1)
}

// Record that the object has a submodule as a direct descendant.
func (s *TreeSize) addSubmodule(filename string) {
	s.MaxPathDepth.AdjustMax(1)
	s.MaxPathLength.AdjustMax(NewCount32(uint64(len(filename))))
	s.ExpandedSubmoduleCount.Increment(1)
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

type CommitSize struct {
	// The height of the ancestor graph, including this commit.
	MaxAncestorDepth Count32 `json:"max_ancestor_depth"`
}

func (s *CommitSize) addParent(s2 CommitSize) {
	s.MaxAncestorDepth.AdjustMax(s2.MaxAncestorDepth)
}

func (s *CommitSize) addTree(s2 TreeSize) {
}

func (s CommitSize) String() string {
	return fmt.Sprintf(
		"max_ancestor_depth=%d",
		s.MaxAncestorDepth,
	)
}

type TagSize struct {
	// The number of tags that have to be traversed (including this
	// one) to get to an object.
	TagDepth Count32
}

func (s TagSize) String() string {
	return fmt.Sprintf("tag_depth=%d", s.TagDepth)
}

type HistorySize struct {
	// The total number of unique commits analyzed.
	UniqueCommitCount Count32 `json:"unique_commit_count"`

	// The total size of all commits analyzed.
	UniqueCommitSize Count64 `json:"unique_commit_size"`

	// The maximum size of any analyzed commit.
	MaxCommitSize Count32 `json:"max_commit_size"`

	// The maximum ancestor depth of any analyzed commit.
	MaxHistoryDepth Count32 `json:"max_history_depth"`

	// The maximum number of direct parents of any analyzed commit.
	MaxParentCount Count32 `json:"max_parent_count"`

	// The total number of unique trees analyzed.
	UniqueTreeCount Count32 `json:"unique_tree_count"`

	// The total size of all trees analyzed.
	UniqueTreeSize Count64 `json:"unique_tree_size"`

	// The total number of tree entries in all unique trees analyzed.
	UniqueTreeEntries Count64 `json:"unique_tree_entries"`

	// The maximum number of entries an a tree.
	MaxTreeEntries Count32 `json:"max_tree_entries"`

	// The total number of unique blobs analyzed.
	UniqueBlobCount Count32 `json:"unique_blob_count"`

	// The total size of all of the unique blobs analyzed.
	UniqueBlobSize Count64 `json:"unique_blob_size"`

	// The maximum size of any analyzed blob.
	MaxBlobSize Count32 `json:"max_blob_size"`

	// The total number of unique tag objects analyzed.
	UniqueTagCount Count32 `json:"unique_tag_count"`

	// The maximum number of tags in a chain.
	MaxTagDepth Count32 `json:"max_tag_depth"`

	// The number of references analyzed. Note that we don't eliminate
	// duplicates if the user passes the same reference more than
	// once.
	ReferenceCount Count32 `json:"reference_count"`

	// The maximum TreeSize in the analyzed history (where each
	// attribute is maximized separately).
	TreeSize
}

func (s *HistorySize) recordBlob(blobSize BlobSize) {
	s.UniqueBlobCount.Increment(1)
	s.UniqueBlobSize.Increment(Count64(blobSize.Size))
	s.MaxBlobSize.AdjustMax(blobSize.Size)
}

func (s *HistorySize) recordTree(treeSize TreeSize, size Count32, treeEntries Count32) {
	s.UniqueTreeCount.Increment(1)
	s.UniqueTreeSize.Increment(Count64(size))
	s.UniqueTreeEntries.Increment(Count64(treeEntries))
	s.MaxTreeEntries.AdjustMax(treeEntries)
	s.TreeSize.adjustMaxima(treeSize)
}

func (s *HistorySize) recordCommit(commitSize CommitSize, size Count32, parentCount Count32) {
	s.UniqueCommitCount.Increment(1)
	s.UniqueCommitSize.Increment(Count64(size))
	s.MaxCommitSize.AdjustMax(size)
	s.MaxHistoryDepth.AdjustMax(commitSize.MaxAncestorDepth)
	s.MaxParentCount.AdjustMax(parentCount)
}

func (s *HistorySize) recordTag(tagSize TagSize, size Count32) {
	s.UniqueTagCount.Increment(1)
	s.MaxTagDepth.AdjustMax(tagSize.TagDepth)
}

func (s *HistorySize) recordReference(ref Reference) {
	s.ReferenceCount.Increment(1)
}

func (s HistorySize) String() string {
	return fmt.Sprintf(
		"unique_commit_count=%d, unique_commit_count = %d, max_commit_size = %d, "+
			"max_history_depth=%d, max_parent_count=%d, "+
			"unique_tree_count=%d, unique_tree_entries=%d, max_tree_entries=%d, "+
			"unique_blob_count=%d, unique_blob_size=%d, max_blob_size=%d, "+
			"unique_tag_count=%d, "+
			"reference_count=%d, "+
			"%s",
		s.UniqueCommitCount, s.UniqueCommitSize, s.MaxCommitSize,
		s.MaxHistoryDepth, s.MaxParentCount,
		s.UniqueTreeCount, s.UniqueTreeEntries, s.MaxTreeEntries,
		s.UniqueBlobCount, s.UniqueBlobSize, s.MaxBlobSize,
		s.UniqueTagCount,
		s.ReferenceCount,
		s.TreeSize,
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
	fmt.Fprintln(buf, "| Name                      | Value     | Level of concern               |")
	fmt.Fprintln(buf, "| ------------------------- | --------- | ------------------------------ |")
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
		{"expanded_tree_count", s.ExpandedTreeCount, MetricPrefixes, " ", 2000},
		{"expanded_blob_count", s.ExpandedBlobCount, MetricPrefixes, " ", 50e3},
		{"expanded_blob_size", s.ExpandedBlobSize, BinaryPrefixes, "B", 1e9},
		{"expanded_link_count", s.ExpandedLinkCount, MetricPrefixes, " ", 25e3},
		{"expanded_submodule_count", s.ExpandedSubmoduleCount, MetricPrefixes, " ", 100},
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
		fmt.Fprintf(buf, "| %-25s | %5s %-3s | %-30s |\n", i.Name, valueString, unitString, warning)
	}
	return buf.String()
}
