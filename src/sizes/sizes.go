package sizes

import (
	"fmt"
)

type Size interface {
	fmt.Stringer
}

type BlobSize struct {
	Size Count
}

func (s BlobSize) String() string {
	return fmt.Sprintf("blob_size=%d", Count(s.Size))
}

type TreeSize struct {
	// The maximum depth of trees and blobs starting at this object
	// (including this object).
	MaxPathDepth Count `json:"max_path_depth"`

	// The maximum length of any path relative to this object, in
	// characters.
	MaxPathLength Count `json:"max_path_length"`

	// The total number of trees, including duplicities.
	ExpandedTreeCount Count `json:"expanded_tree_count"`

	// The maximum number of entries an a tree.
	MaxTreeEntries Count `json:"max_tree_entries"`

	// The total number of blobs.
	ExpandedBlobCount Count `json:"expanded_blob_count"`

	// The total size of all blobs.
	ExpandedBlobSize Count `json:"expanded_blob_size"`

	// The total number of symbolic links.
	ExpandedLinkCount Count `json:"expanded_link_count"`

	// The total number of submodules referenced.
	ExpandedSubmoduleCount Count `json:"expanded_submodule_count"`
}

func (s *TreeSize) addDescendent(filename string, s2 TreeSize) {
	s.MaxPathDepth.AdjustMax(s2.MaxPathDepth)
	if s2.MaxPathLength > 0 {
		s.MaxPathLength.AdjustMax((Count(len(filename)) + 1).Plus(s2.MaxPathLength))
	} else {
		s.MaxPathLength.AdjustMax(Count(len(filename)))
	}
	s.ExpandedTreeCount.Increment(s2.ExpandedTreeCount)
	s.MaxTreeEntries.AdjustMax(s2.MaxTreeEntries)
	s.ExpandedBlobCount.Increment(s2.ExpandedBlobCount)
	s.ExpandedBlobSize.Increment(s2.ExpandedBlobSize)
	s.ExpandedLinkCount.Increment(s2.ExpandedLinkCount)
	s.ExpandedSubmoduleCount.Increment(s2.ExpandedSubmoduleCount)
}

func (s *TreeSize) adjustMaxima(s2 TreeSize) {
	s.MaxPathDepth.AdjustMax(s2.MaxPathDepth)
	s.MaxPathLength.AdjustMax(s2.MaxPathLength)
	s.ExpandedTreeCount.AdjustMax(s2.ExpandedTreeCount)
	s.MaxTreeEntries.AdjustMax(s2.MaxTreeEntries)
	s.ExpandedBlobCount.AdjustMax(s2.ExpandedBlobCount)
	s.ExpandedBlobSize.AdjustMax(s2.ExpandedBlobSize)
	s.ExpandedLinkCount.AdjustMax(s2.ExpandedLinkCount)
	s.ExpandedSubmoduleCount.AdjustMax(s2.ExpandedSubmoduleCount)
}

// Record that the object has a blob of the specified `size` as a
// direct descendant.
func (s *TreeSize) addBlob(filename string, size BlobSize) {
	s.MaxPathDepth.AdjustMax(1)
	s.MaxPathLength.AdjustMax(Count(len(filename)))
	s.ExpandedBlobSize.Increment(size.Size)
	s.ExpandedBlobCount.Increment(1)
}

// Record that the object has a link as a direct descendant.
func (s *TreeSize) addLink(filename string) {
	s.MaxPathDepth.AdjustMax(1)
	s.MaxPathLength.AdjustMax(Count(len(filename)))
	s.ExpandedLinkCount.Increment(1)
}

// Record that the object has a submodule as a direct descendant.
func (s *TreeSize) addSubmodule(filename string) {
	s.MaxPathDepth.AdjustMax(1)
	s.MaxPathLength.AdjustMax(Count(len(filename)))
	s.ExpandedSubmoduleCount.Increment(1)
}

func (s TreeSize) String() string {
	return fmt.Sprintf(
		"max_path_depth=%d, max_path_length=%d, "+
			"expanded_tree_count=%d, max_tree_entries=%d, "+
			"expanded_blob_count=%d, expanded_blob_size=%d, "+
			"expanded_link_count=%d, expanded_submodule_count=%d",
		s.MaxPathDepth, s.MaxPathLength,
		s.ExpandedTreeCount, s.MaxTreeEntries,
		s.ExpandedBlobCount, s.ExpandedBlobSize,
		s.ExpandedLinkCount, s.ExpandedSubmoduleCount,
	)
}

type CommitSize struct {
	// The height of the ancestor graph, including this commit.
	MaxAncestorDepth Count `json:"max_ancestor_depth"`
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

type HistorySize struct {
	// The total number of unique commits analyzed.
	UniqueCommitCount Count `json:"unique_commit_count"`

	// The total size of all commits analyzed.
	UniqueCommitSize Count `json:"unique_commit_size"`

	// The maximum size of any analyzed commit.
	MaxCommitSize Count `json:"max_commit_size"`

	// The maximum ancestor depth of any analyzed commit.
	MaxHistoryDepth Count `json:"max_history_depth"`

	// The maximum number of direct parents of any analyzed commit.
	MaxParentCount Count `json:"max_parent_count"`

	// The total number of unique trees analyzed.
	UniqueTreeCount Count `json:"unique_tree_count"`

	// The total size of all trees analyzed.
	UniqueTreeSize Count `json:"unique_tree_size"`

	// The total number of tree entries in all unique trees analyzed.
	UniqueTreeEntries Count `json:"unique_tree_entries"`

	// The total number of unique blobs analyzed.
	UniqueBlobCount Count `json:"unique_blob_count"`

	// The total size of all of the unique blobs analyzed.
	UniqueBlobSize Count `json:"unique_blob_size"`

	// The total number of unique tag objects analyzed.
	UniqueTagCount Count `json:"unique_tag_count"`

	// The maximum TreeSize in the analyzed history (where each
	// attribute is maximized separately).
	TreeSize
}

func (s *HistorySize) recordBlob(blobSize BlobSize) {
	s.UniqueBlobCount.Increment(1)
	s.UniqueBlobSize.Increment(blobSize.Size)
}

func (s *HistorySize) recordTree(treeSize TreeSize, size Count, treeEntries Count) {
	s.UniqueTreeCount.Increment(1)
	s.UniqueTreeSize.Increment(size)
	s.UniqueTreeEntries.Increment(treeEntries)
	s.TreeSize.adjustMaxima(treeSize)
}

func (s *HistorySize) recordCommit(commitSize CommitSize, size Count, parentCount Count) {
	s.UniqueCommitCount.Increment(1)
	s.UniqueCommitSize.Increment(size)
	s.MaxCommitSize.AdjustMax(size)
	s.MaxHistoryDepth.AdjustMax(commitSize.MaxAncestorDepth)
	s.MaxParentCount.AdjustMax(parentCount)
}

func (s HistorySize) String() string {
	return fmt.Sprintf(
		"unique_commit_count=%d, unique_commit_count = %d, max_commit_size = %d, "+
			"max_history_depth=%d, max_parent_count=%d, "+
			"unique_tree_count=%d, unique_tree_entries=%d, unique_blob_count=%d, "+
			"unique_blob_size=%d, unique_tag_count=%d, %s",
		s.UniqueCommitCount, s.UniqueCommitSize, s.MaxCommitSize,
		s.MaxHistoryDepth, s.MaxParentCount,
		s.UniqueTreeCount, s.UniqueTreeEntries, s.UniqueBlobCount,
		s.UniqueBlobSize, s.UniqueTagCount, s.TreeSize,
	)
}
