package sizes

import (
	"errors"
	"fmt"
	"io"
	"math"
)

// A count of something, capped at math.MaxUint64.
type Count uint64

// Return the sum of two Counts, capped at math.MaxUint64.
func (n1 Count) Plus(n2 Count) Count {
	n := n1 + n2
	if n < n1 {
		// Overflow
		return math.MaxUint64
	}
	return n
}

// Increment `*n1` by `n2`, capped at math.MaxUint64.
func (n1 *Count) Increment(n2 Count) {
	*n1 = n1.Plus(n2)
}

// Adjust `*n1` to be `max(*n1, n2)`.
func (n1 *Count) AdjustMax(n2 Count) {
	if n2 > *n1 {
		*n1 = n2
	}
}

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
		"max_path_depth=%d, max_path_length=%d, expanded_tree_count=%d, max_tree_entries=%d, expanded_blob_count=%d, expanded_blob_size=%d, expanded_link_count=%d, expanded_submodule_count=%d",
		s.MaxPathDepth, s.MaxPathLength, s.ExpandedTreeCount, s.MaxTreeEntries, s.ExpandedBlobCount, s.ExpandedBlobSize, s.ExpandedLinkCount, s.ExpandedSubmoduleCount,
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

type ToDoList struct {
	list []Oid
}

func (t *ToDoList) Length() int {
	return len(t.list)
}

func (t *ToDoList) Push(oid Oid) {
	t.list = append(t.list, oid)
}

func (t *ToDoList) Peek() Oid {
	return t.list[len(t.list)-1]
}

func (t *ToDoList) Drop() {
	t.list = t.list[0 : len(t.list)-1]
}

func (t *ToDoList) Dump(w io.Writer) {
	fmt.Fprintf(w, "todo list has %d items\n", t.Length())
	for i, idString := range t.list {
		fmt.Fprintf(w, "%8d %s\n", i, idString)
	}
	fmt.Fprintf(w, "\n")
}

var NotYetKnown = errors.New("the size of an object is not yet known")

type SizeCache struct {
	repo *Repository

	// The (recursive) size of trees whose sizes have been computed so
	// far.
	treeSizes map[Oid]TreeSize

	// The size of blobs whose sizes have been looked up so far.
	blobSizes map[Oid]BlobSize

	// The size of commits whose sizes have been looked up so far.
	commitSizes map[Oid]CommitSize

	// Statistics about the overall history size:
	HistorySize HistorySize

	// The OIDs of commits and trees whose sizes are in the process of
	// being computed. This is, roughly, the call stack. As long as
	// there are no SHA-1 collisions, the sizes of these lists are
	// bounded:
	//
	// * commitsToDo is at most the total number of direct parents
	//   along a single ancestry path through history.
	//
	// * treesToDo is at most the total number of direct non-blob
	//   referents in all unique objects along a single lineage of
	//   descendants of the starting point.
	commitsToDo ToDoList
	treesToDo   ToDoList
}

func NewSizeCache(repo *Repository) (*SizeCache, error) {
	cache := &SizeCache{
		repo:        repo,
		treeSizes:   make(map[Oid]TreeSize),
		blobSizes:   make(map[Oid]BlobSize),
		commitSizes: make(map[Oid]CommitSize),
	}
	return cache, nil
}

func (cache *SizeCache) TypedObjectSize(
	spec string, oid Oid, objectType Type, objectSize Count,
) (Size, error) {
	switch objectType {
	case "blob":
		blobSize := BlobSize{objectSize}
		cache.recordBlob(oid, blobSize)
		return blobSize, nil
	case "tree":
		treeSize, err := cache.TreeSize(oid)
		return treeSize, err
	case "commit":
		commitSize, err := cache.CommitSize(oid)
		return commitSize, err
	case "tag":
		// FIXME
		return nil, nil
	default:
		panic(fmt.Sprintf("object %v has unknown type", oid))
	}
}

func (cache *SizeCache) ObjectSize(spec string) (Oid, Type, Size, error) {
	oid, objectType, objectSize, err := cache.repo.ReadHeader(spec)
	if err != nil {
		return Oid{}, "missing", nil, err
	}

	size, err := cache.TypedObjectSize(spec, oid, objectType, objectSize)
	return oid, objectType, size, err
}

func (cache *SizeCache) ReferenceSize(ref Reference) (Size, error) {
	return cache.TypedObjectSize(ref.Refname, ref.Oid, ref.ObjectType, ref.ObjectSize)
}

func (cache *SizeCache) OidObjectSize(oid Oid) (Type, Size, error) {
	_, objectType, size, error := cache.ObjectSize(oid.String())
	return objectType, size, error
}

func (cache *SizeCache) BlobSize(oid Oid) (BlobSize, error) {
	size, ok := cache.blobSizes[oid]
	if !ok {
		_, objectType, objectSize, err := cache.repo.ReadHeader(oid.String())
		if err != nil {
			return BlobSize{}, err
		}
		if objectType != "blob" {
			return BlobSize{}, fmt.Errorf("object %s is a %s, not a blob", oid, objectType)
		}
		size = BlobSize{objectSize}
		cache.recordBlob(oid, size)
	}
	return size, nil
}

func (cache *SizeCache) TreeSize(oid Oid) (TreeSize, error) {
	s, ok := cache.treeSizes[oid]
	if ok {
		return s, nil
	}

	cache.treesToDo.Push(oid)
	err := cache.fill()
	if err != nil {
		return TreeSize{}, err
	}

	// Now the size should be in the cache:
	s, ok = cache.treeSizes[oid]
	if ok {
		return s, nil
	}
	panic("queueTree() didn't fill tree")
}

func (cache *SizeCache) CommitSize(oid Oid) (CommitSize, error) {
	s, ok := cache.commitSizes[oid]
	if ok {
		return s, nil
	}

	cache.commitsToDo.Push(oid)
	err := cache.fill()
	if err != nil {
		return CommitSize{}, err
	}

	// Now the size should be in the cache:
	s, ok = cache.commitSizes[oid]
	if ok {
		return s, nil
	}
	panic("fill() didn't fill commit")
}

func (cache *SizeCache) recordCommit(oid Oid, commitSize CommitSize, size Count, parentCount Count) {
	cache.commitSizes[oid] = commitSize
	cache.HistorySize.recordCommit(commitSize, size, parentCount)
}

func (cache *SizeCache) recordTree(oid Oid, treeSize TreeSize, size Count, treeEntries Count) {
	cache.treeSizes[oid] = treeSize
	cache.HistorySize.recordTree(treeSize, size, treeEntries)
}

func (cache *SizeCache) recordBlob(oid Oid, blobSize BlobSize) {
	cache.blobSizes[oid] = blobSize
	cache.HistorySize.recordBlob(blobSize)
}

// Compute the sizes of any trees listed in `cache.commitsToDo` or
// `cache.treesToDo`. This might involve computing the sizes of
// referred-to objects. Do this without recursion to avoid unlimited
// stack growth.
func (cache *SizeCache) fill() error {
	for {
		if cache.treesToDo.Length() != 0 {
			oid := cache.treesToDo.Peek()

			// See if the object's size has been computed since it was
			// enqueued. This can happen if it is used in multiple places
			// in the ancestry graph.
			_, ok := cache.treeSizes[oid]
			if ok {
				cache.treesToDo.Drop()
				continue
			}

			treeSize, size, treeEntries, err := cache.queueTree(oid)
			if err == nil {
				cache.recordTree(oid, treeSize, size, treeEntries)
				cache.treesToDo.Drop()
			} else if err == NotYetKnown {
				// Let loop continue (the tree's constituents were added
				// to `treesToDo` by `queueTree()`).
			} else {
				return err
			}
			continue
		}

		if cache.commitsToDo.Length() != 0 {
			oid := cache.commitsToDo.Peek()

			// See if the object's size has been computed since it was
			// enqueued. This can happen if it is used in multiple places
			// in the ancestry graph.
			_, ok := cache.commitSizes[oid]
			if ok {
				cache.commitsToDo.Drop()
				continue
			}

			commitSize, size, parentCount, err := cache.queueCommit(oid)
			if err == nil {
				cache.recordCommit(oid, commitSize, size, parentCount)
				cache.commitsToDo.Drop()
			} else if err == NotYetKnown {
				// Let loop continue (the commits's constituents were
				// added to `commitsToDo` and `treesToDo` by
				// `queueCommit()`).
			} else {
				return err
			}
			continue
		}

		// There is nothing left to do:
		return nil
	}
}

// Compute and return the size of the commit with the specified `oid`
// if we already know the size of its constituents. If the
// constituents' sizes are not yet known but believed to be
// computable, add any unknown constituents to `commitsToDo` and
// `treesToDo` and return an `NotYetKnown` error. If another error
// occurred while looking up an object, return that error. `oid` is
// not already in the cache.
func (cache *SizeCache) queueCommit(oid Oid) (CommitSize, Count, Count, error) {
	var err error

	commit, err := cache.repo.ReadCommit(oid)
	if err != nil {
		return CommitSize{}, 0, 0, err
	}

	ok := true

	size := CommitSize{}

	// First accumulate all of the sizes for all parents:
	for _, parent := range commit.Parents {
		parentSize, parentOK := cache.commitSizes[parent]
		if parentOK {
			if ok {
				size.addParent(parentSize)
			}
		} else {
			ok = false
			// Schedule this one to be computed:
			cache.commitsToDo.Push(parent)
		}
	}

	// Now gather information about the tree:
	treeSize, treeOk := cache.treeSizes[commit.Tree]
	if treeOk {
		if ok {
			size.addTree(treeSize)
		}
	} else {
		ok = false
		cache.treesToDo.Push(commit.Tree)
	}

	if !ok {
		return CommitSize{}, 0, 0, NotYetKnown
	}

	// Now add one to the ancestor depth to account for this commit
	// itself:
	size.MaxAncestorDepth.Increment(1)
	return size, commit.Size, Count(len(commit.Parents)), nil
}

// Compute and return the size of the tree with the specified `oid` if
// we already know the size of its constituents. If the constituents'
// sizes are not yet known but believed to be computable, add any
// unknown constituents to `treesToDo` and return an `NotYetKnown`
// error. If another error occurred while looking up an object, return
// that error. `oid` is not already in the cache.
func (cache *SizeCache) queueTree(oid Oid) (TreeSize, Count, Count, error) {
	var err error

	tree, err := cache.repo.ReadTree(oid)
	if err != nil {
		return TreeSize{}, 0, 0, err
	}

	ok := true

	entryCount := Count(0)

	// First accumulate all of the sizes (including maximum depth) for
	// all descendants:
	size := TreeSize{
		ExpandedTreeCount: 1,
	}

	var entry TreeEntry

	iter := tree.Iter()

	for {
		entryOk, err := iter.NextEntry(&entry)
		if err != nil {
			return TreeSize{}, 0, 0, err
		}
		if !entryOk {
			break
		}
		entryCount += 1

		switch {
		case entry.Filemode&0170000 == 0040000:
			// Tree
			subsize, subok := cache.treeSizes[entry.Oid]
			if subok {
				if ok {
					size.addDescendent(entry.Name, subsize)
				}
			} else {
				ok = false
				// Schedule this one to be computed:
				cache.treesToDo.Push(entry.Oid)
			}

		case entry.Filemode&0170000 == 0160000:
			// Commit
			if ok {
				size.addSubmodule(entry.Name)
			}

		case entry.Filemode&0170000 == 0120000:
			// Symlink
			if ok {
				size.addLink(entry.Name)
			}

		default:
			// Blob
			blobSize, blobOk := cache.blobSizes[entry.Oid]
			if blobOk {
				if ok {
					size.addBlob(entry.Name, blobSize)
				}
			} else {
				blobSize, err := cache.BlobSize(entry.Oid)
				if err != nil {
					return TreeSize{}, 0, 0, err
				}
				size.addBlob(entry.Name, blobSize)
			}
		}
	}

	if !ok {
		return TreeSize{}, 0, 0, NotYetKnown
	}

	// Now add one to the depth and to the tree count to account for
	// this tree itself:
	size.MaxPathDepth.Increment(1)
	size.MaxTreeEntries.AdjustMax(entryCount)
	return size, Count(len(tree.data)), entryCount, nil
}
