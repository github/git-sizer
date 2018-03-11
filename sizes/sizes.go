package sizes

import (
	"fmt"

	"github.com/github/git-sizer/counts"
	"github.com/github/git-sizer/git"
)

type Size interface {
	fmt.Stringer
}

type BlobSize struct {
	Size counts.Count32
}

type TreeSize struct {
	// The maximum depth of trees and blobs starting at this object
	// (not including this object).
	MaxPathDepth counts.Count32 `json:"max_path_depth"`

	// The maximum length of any path relative to this object, in
	// characters.
	MaxPathLength counts.Count32 `json:"max_path_length"`

	// The total number of trees, including duplicates.
	ExpandedTreeCount counts.Count32 `json:"expanded_tree_count"`

	// The total number of blobs, including duplicates.
	ExpandedBlobCount counts.Count32 `json:"expanded_blob_count"`

	// The total size of all blobs, including duplicates.
	ExpandedBlobSize counts.Count64 `json:"expanded_blob_size"`

	// The total number of symbolic links, including duplicates.
	ExpandedLinkCount counts.Count32 `json:"expanded_link_count"`

	// The total number of submodules referenced, including duplicates.
	ExpandedSubmoduleCount counts.Count32 `json:"expanded_submodule_count"`
}

func (s *TreeSize) addDescendent(filename string, s2 TreeSize) {
	s.MaxPathDepth.AdjustMaxIfNecessary(s2.MaxPathDepth.Plus(1))
	if s2.MaxPathLength > 0 {
		s.MaxPathLength.AdjustMaxIfNecessary(
			(counts.NewCount32(uint64(len(filename))) + 1).Plus(s2.MaxPathLength),
		)
	} else {
		s.MaxPathLength.AdjustMaxIfNecessary(counts.NewCount32(uint64(len(filename))))
	}
	s.ExpandedTreeCount.Increment(s2.ExpandedTreeCount)
	s.ExpandedBlobCount.Increment(s2.ExpandedBlobCount)
	s.ExpandedBlobSize.Increment(s2.ExpandedBlobSize)
	s.ExpandedLinkCount.Increment(s2.ExpandedLinkCount)
	s.ExpandedSubmoduleCount.Increment(s2.ExpandedSubmoduleCount)
}

// Record that the object has a blob of the specified `size` as a
// direct descendant.
func (s *TreeSize) addBlob(filename string, size BlobSize) {
	s.MaxPathDepth.AdjustMaxIfNecessary(1)
	s.MaxPathLength.AdjustMaxIfNecessary(counts.NewCount32(uint64(len(filename))))
	s.ExpandedBlobSize.Increment(counts.Count64(size.Size))
	s.ExpandedBlobCount.Increment(1)
}

// Record that the object has a link as a direct descendant.
func (s *TreeSize) addLink(filename string) {
	s.MaxPathDepth.AdjustMaxIfNecessary(1)
	s.MaxPathLength.AdjustMaxIfNecessary(counts.NewCount32(uint64(len(filename))))
	s.ExpandedLinkCount.Increment(1)
}

// Record that the object has a submodule as a direct descendant.
func (s *TreeSize) addSubmodule(filename string) {
	s.MaxPathDepth.AdjustMaxIfNecessary(1)
	s.MaxPathLength.AdjustMaxIfNecessary(counts.NewCount32(uint64(len(filename))))
	s.ExpandedSubmoduleCount.Increment(1)
}

type CommitSize struct {
	// The height of the ancestor graph, including this commit.
	MaxAncestorDepth counts.Count32 `json:"max_ancestor_depth"`
}

func (s *CommitSize) addParent(s2 CommitSize) {
	s.MaxAncestorDepth.AdjustMaxIfNecessary(s2.MaxAncestorDepth)
}

func (s *CommitSize) addTree(s2 TreeSize) {
}

type TagSize struct {
	// The number of tags that have to be traversed (including this
	// one) to get to an object.
	TagDepth counts.Count32
}

type HistorySize struct {
	// The total number of unique commits analyzed.
	UniqueCommitCount counts.Count32 `json:"unique_commit_count"`

	// The total size of all commits analyzed.
	UniqueCommitSize counts.Count64 `json:"unique_commit_size"`

	// The maximum size of any analyzed commit.
	MaxCommitSize counts.Count32 `json:"max_commit_size"`

	// The commit with the maximum size.
	MaxCommitSizeCommit *Path `json:"max_commit,omitempty"`

	// The maximum ancestor depth of any analyzed commit.
	MaxHistoryDepth counts.Count32 `json:"max_history_depth"`

	// The maximum number of direct parents of any analyzed commit.
	MaxParentCount counts.Count32 `json:"max_parent_count"`

	// The commit with the maximum number of direct parents.
	MaxParentCountCommit *Path `json:"max_parent_count_commit,omitempty"`

	// The total number of unique trees analyzed.
	UniqueTreeCount counts.Count32 `json:"unique_tree_count"`

	// The total size of all trees analyzed.
	UniqueTreeSize counts.Count64 `json:"unique_tree_size"`

	// The total number of tree entries in all unique trees analyzed.
	UniqueTreeEntries counts.Count64 `json:"unique_tree_entries"`

	// The maximum number of entries an a tree.
	MaxTreeEntries counts.Count32 `json:"max_tree_entries"`

	// The tree with the maximum number of entries.
	MaxTreeEntriesTree *Path `json:"max_tree_entries_tree,omitempty"`

	// The total number of unique blobs analyzed.
	UniqueBlobCount counts.Count32 `json:"unique_blob_count"`

	// The total size of all of the unique blobs analyzed.
	UniqueBlobSize counts.Count64 `json:"unique_blob_size"`

	// The maximum size of any analyzed blob.
	MaxBlobSize counts.Count32 `json:"max_blob_size"`

	// The biggest blob found.
	MaxBlobSizeBlob *Path `json:"max_blob_size_blob,omitempty"`

	// The total number of unique tag objects analyzed.
	UniqueTagCount counts.Count32 `json:"unique_tag_count"`

	// The maximum number of tags in a chain.
	MaxTagDepth counts.Count32 `json:"max_tag_depth"`

	// The tag with the maximum tag depth.
	MaxTagDepthTag *Path `json:"max_tag_depth_tag,omitempty"`

	// The number of references analyzed. Note that we don't eliminate
	// duplicates if the user passes the same reference more than
	// once.
	ReferenceCount counts.Count32 `json:"reference_count"`

	// The maximum TreeSize in the analyzed history (where each
	// attribute is maximized separately).

	// The maximum depth of trees and blobs starting at this object
	// (not including this object).
	MaxPathDepth counts.Count32 `json:"max_path_depth"`

	// The tree with the maximum path depth.
	MaxPathDepthTree *Path `json:"max_path_depth_tree,omitempty"`

	// The maximum length of any path relative to this object, in
	// characters.
	MaxPathLength counts.Count32 `json:"max_path_length"`

	// The tree with the maximum path length.
	MaxPathLengthTree *Path `json:"max_path_length_tree,omitempty"`

	// The total number of trees, including duplicates.
	MaxExpandedTreeCount counts.Count32 `json:"max_expanded_tree_count"`

	// The tree with the maximum expanded tree count.
	MaxExpandedTreeCountTree *Path `json:"max_expanded_tree_count_tree,omitempty"`

	// The total number of blobs, including duplicates.
	MaxExpandedBlobCount counts.Count32 `json:"max_expanded_blob_count"`

	// The tree with the maximum expanded blob count.
	MaxExpandedBlobCountTree *Path `json:"max_expanded_blob_count_tree,omitempty"`

	// The total size of all blobs, including duplicates.
	MaxExpandedBlobSize counts.Count64 `json:"max_expanded_blob_size"`

	// The tree with the maximum expanded blob size.
	MaxExpandedBlobSizeTree *Path `json:"max_expanded_blob_size_tree,omitempty"`

	// The total number of symbolic links, including duplicates.
	MaxExpandedLinkCount counts.Count32 `json:"max_expanded_link_count"`

	// The tree with the maximum expanded link count.
	MaxExpandedLinkCountTree *Path `json:"max_expanded_link_count_tree,omitempty"`

	// The total number of submodules referenced, including duplicates.
	MaxExpandedSubmoduleCount counts.Count32 `json:"max_expanded_submodule_count"`

	// The tree with the maximum expanded submodule count.
	MaxExpandedSubmoduleCountTree *Path `json:"max_expanded_submodule_count_tree,omitempty"`
}

// Convenience function: forget `*path` if it is non-nil and overwrite
// it with a `*Path` for the object corresponding to `(oid,
// objectType)`. This function can be used if a new largest item was
// found.
func setPath(
	pr PathResolver,
	path **Path,
	oid git.OID, objectType string) {
	if *path != nil {
		pr.ForgetPath(*path)
	}
	*path = pr.RequestPath(oid, objectType)
}

func (s *HistorySize) recordBlob(g *Graph, oid git.OID, blobSize BlobSize) {
	s.UniqueBlobCount.Increment(1)
	s.UniqueBlobSize.Increment(counts.Count64(blobSize.Size))
	if s.MaxBlobSize.AdjustMaxIfNecessary(blobSize.Size) {
		setPath(g.pathResolver, &s.MaxBlobSizeBlob, oid, "blob")
	}
}

func (s *HistorySize) recordTree(
	g *Graph, oid git.OID, treeSize TreeSize, size counts.Count32, treeEntries counts.Count32,
) {
	s.UniqueTreeCount.Increment(1)
	s.UniqueTreeSize.Increment(counts.Count64(size))
	s.UniqueTreeEntries.Increment(counts.Count64(treeEntries))
	if s.MaxTreeEntries.AdjustMaxIfNecessary(treeEntries) {
		setPath(g.pathResolver, &s.MaxTreeEntriesTree, oid, "tree")
	}

	if s.MaxPathDepth.AdjustMaxIfNecessary(treeSize.MaxPathDepth) {
		setPath(g.pathResolver, &s.MaxPathDepthTree, oid, "tree")
	}
	if s.MaxPathLength.AdjustMaxIfNecessary(treeSize.MaxPathLength) {
		setPath(g.pathResolver, &s.MaxPathLengthTree, oid, "tree")
	}
	if s.MaxExpandedTreeCount.AdjustMaxIfNecessary(treeSize.ExpandedTreeCount) {
		setPath(g.pathResolver, &s.MaxExpandedTreeCountTree, oid, "tree")
	}
	if s.MaxExpandedBlobCount.AdjustMaxIfNecessary(treeSize.ExpandedBlobCount) {
		setPath(g.pathResolver, &s.MaxExpandedBlobCountTree, oid, "tree")
	}
	if s.MaxExpandedBlobSize.AdjustMaxIfNecessary(treeSize.ExpandedBlobSize) {
		setPath(g.pathResolver, &s.MaxExpandedBlobSizeTree, oid, "tree")
	}
	if s.MaxExpandedLinkCount.AdjustMaxIfNecessary(treeSize.ExpandedLinkCount) {
		setPath(g.pathResolver, &s.MaxExpandedLinkCountTree, oid, "tree")
	}
	if s.MaxExpandedSubmoduleCount.AdjustMaxIfNecessary(treeSize.ExpandedSubmoduleCount) {
		setPath(g.pathResolver, &s.MaxExpandedSubmoduleCountTree, oid, "tree")
	}
}

func (s *HistorySize) recordCommit(
	g *Graph, oid git.OID, commitSize CommitSize,
	size counts.Count32, parentCount counts.Count32,
) {
	s.UniqueCommitCount.Increment(1)
	s.UniqueCommitSize.Increment(counts.Count64(size))
	if s.MaxCommitSize.AdjustMaxIfPossible(size) {
		setPath(g.pathResolver, &s.MaxCommitSizeCommit, oid, "commit")
	}
	s.MaxHistoryDepth.AdjustMaxIfPossible(commitSize.MaxAncestorDepth)
	if s.MaxParentCount.AdjustMaxIfPossible(parentCount) {
		setPath(g.pathResolver, &s.MaxParentCountCommit, oid, "commit")
	}
}

func (s *HistorySize) recordTag(g *Graph, oid git.OID, tagSize TagSize, size counts.Count32) {
	s.UniqueTagCount.Increment(1)
	if s.MaxTagDepth.AdjustMaxIfNecessary(tagSize.TagDepth) {
		setPath(g.pathResolver, &s.MaxTagDepthTag, oid, "tag")
	}
}

func (s *HistorySize) recordReference(g *Graph, ref git.Reference) {
	s.ReferenceCount.Increment(1)
}
