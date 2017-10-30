package sizes

import (
	"errors"
	"fmt"
)

var NotYetKnown = errors.New("the size of an object is not yet known")

type SizeScanner struct {
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
	// * treesToDo is at most the total number of direct non-blob
	//   referents in all unique objects along a single lineage of
	//   descendants of the starting point.
	//
	// * commitsToDo is at most the total number of direct parents
	//   along a single ancestry path through history.
	treesToDo   ToDoList
	commitsToDo ToDoList
}

func NewSizeScanner(repo *Repository) (*SizeScanner, error) {
	scanner := &SizeScanner{
		repo:        repo,
		treeSizes:   make(map[Oid]TreeSize),
		blobSizes:   make(map[Oid]BlobSize),
		commitSizes: make(map[Oid]CommitSize),
	}
	return scanner, nil
}

func (scanner *SizeScanner) TypedObjectSize(
	spec string, oid Oid, objectType Type, objectSize Count,
) (Size, error) {
	switch objectType {
	case "blob":
		blobSize := BlobSize{objectSize}
		scanner.recordBlob(oid, blobSize)
		return blobSize, nil
	case "tree":
		treeSize, err := scanner.TreeSize(oid)
		return treeSize, err
	case "commit":
		commitSize, err := scanner.CommitSize(oid)
		return commitSize, err
	case "tag":
		// FIXME
		return nil, nil
	default:
		panic(fmt.Sprintf("object %v has unknown type", oid))
	}
}

func (scanner *SizeScanner) ObjectSize(spec string) (Oid, Type, Size, error) {
	oid, objectType, objectSize, err := scanner.repo.ReadHeader(spec)
	if err != nil {
		return Oid{}, "missing", nil, err
	}

	size, err := scanner.TypedObjectSize(spec, oid, objectType, objectSize)
	return oid, objectType, size, err
}

func (scanner *SizeScanner) ReferenceSize(ref Reference) (Size, error) {
	return scanner.TypedObjectSize(ref.Refname, ref.Oid, ref.ObjectType, ref.ObjectSize)
}

func (scanner *SizeScanner) OidObjectSize(oid Oid) (Type, Size, error) {
	_, objectType, size, error := scanner.ObjectSize(oid.String())
	return objectType, size, error
}

func (scanner *SizeScanner) BlobSize(oid Oid) (BlobSize, error) {
	size, ok := scanner.blobSizes[oid]
	if !ok {
		_, objectType, objectSize, err := scanner.repo.ReadHeader(oid.String())
		if err != nil {
			return BlobSize{}, err
		}
		if objectType != "blob" {
			return BlobSize{}, fmt.Errorf("object %s is a %s, not a blob", oid, objectType)
		}
		size = BlobSize{objectSize}
		scanner.recordBlob(oid, size)
	}
	return size, nil
}

func (scanner *SizeScanner) TreeSize(oid Oid) (TreeSize, error) {
	s, ok := scanner.treeSizes[oid]
	if ok {
		return s, nil
	}

	scanner.treesToDo.Push(oid)
	err := scanner.fill()
	if err != nil {
		return TreeSize{}, err
	}

	// Now the size should be in the cache:
	s, ok = scanner.treeSizes[oid]
	if ok {
		return s, nil
	}
	panic("queueTree() didn't fill tree")
}

func (scanner *SizeScanner) CommitSize(oid Oid) (CommitSize, error) {
	s, ok := scanner.commitSizes[oid]
	if ok {
		return s, nil
	}

	scanner.commitsToDo.Push(oid)
	err := scanner.fill()
	if err != nil {
		return CommitSize{}, err
	}

	// Now the size should be in the cache:
	s, ok = scanner.commitSizes[oid]
	if ok {
		return s, nil
	}
	panic("fill() didn't fill commit")
}

func (scanner *SizeScanner) recordBlob(oid Oid, blobSize BlobSize) {
	scanner.blobSizes[oid] = blobSize
	scanner.HistorySize.recordBlob(blobSize)
}

func (scanner *SizeScanner) recordTree(oid Oid, treeSize TreeSize, size Count, treeEntries Count) {
	scanner.treeSizes[oid] = treeSize
	scanner.HistorySize.recordTree(treeSize, size, treeEntries)
}

func (scanner *SizeScanner) recordCommit(oid Oid, commitSize CommitSize, size Count, parentCount Count) {
	scanner.commitSizes[oid] = commitSize
	scanner.HistorySize.recordCommit(commitSize, size, parentCount)
}

// Compute the sizes of any trees listed in `scanner.commitsToDo` or
// `scanner.treesToDo`. This might involve computing the sizes of
// referred-to objects. Do this without recursion to avoid unlimited
// stack growth.
func (scanner *SizeScanner) fill() error {
	for {
		if scanner.treesToDo.Length() != 0 {
			oid := scanner.treesToDo.Peek()

			// See if the object's size has been computed since it was
			// enqueued. This can happen if it is used in multiple places
			// in the ancestry graph.
			_, ok := scanner.treeSizes[oid]
			if ok {
				scanner.treesToDo.Drop()
				continue
			}

			treeSize, size, treeEntries, err := scanner.queueTree(oid)
			if err == nil {
				scanner.recordTree(oid, treeSize, size, treeEntries)
				scanner.treesToDo.Drop()
			} else if err == NotYetKnown {
				// Let loop continue (the tree's constituents were added
				// to `treesToDo` by `queueTree()`).
			} else {
				return err
			}
			continue
		}

		if scanner.commitsToDo.Length() != 0 {
			oid := scanner.commitsToDo.Peek()

			// See if the object's size has been computed since it was
			// enqueued. This can happen if it is used in multiple places
			// in the ancestry graph.
			_, ok := scanner.commitSizes[oid]
			if ok {
				scanner.commitsToDo.Drop()
				continue
			}

			commitSize, size, parentCount, err := scanner.queueCommit(oid)
			if err == nil {
				scanner.recordCommit(oid, commitSize, size, parentCount)
				scanner.commitsToDo.Drop()
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

// Compute and return the size of the tree with the specified `oid` if
// we already know the size of its constituents. If the constituents'
// sizes are not yet known but believed to be computable, add any
// unknown constituents to `treesToDo` and return an `NotYetKnown`
// error. If another error occurred while looking up an object, return
// that error. `oid` is not already in the cache.
func (scanner *SizeScanner) queueTree(oid Oid) (TreeSize, Count, Count, error) {
	var err error

	tree, err := scanner.repo.ReadTree(oid)
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
			subsize, subok := scanner.treeSizes[entry.Oid]
			if subok {
				if ok {
					size.addDescendent(entry.Name, subsize)
				}
			} else {
				ok = false
				// Schedule this one to be computed:
				scanner.treesToDo.Push(entry.Oid)
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
			blobSize, blobOk := scanner.blobSizes[entry.Oid]
			if blobOk {
				if ok {
					size.addBlob(entry.Name, blobSize)
				}
			} else {
				blobSize, err := scanner.BlobSize(entry.Oid)
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
	return size, Count(len(tree.data)), entryCount, nil
}

// Compute and return the size of the commit with the specified `oid`
// if we already know the size of its constituents. If the
// constituents' sizes are not yet known but believed to be
// computable, add any unknown constituents to `commitsToDo` and
// `treesToDo` and return an `NotYetKnown` error. If another error
// occurred while looking up an object, return that error. `oid` is
// not already in the cache.
func (scanner *SizeScanner) queueCommit(oid Oid) (CommitSize, Count, Count, error) {
	var err error

	commit, err := scanner.repo.ReadCommit(oid)
	if err != nil {
		return CommitSize{}, 0, 0, err
	}

	ok := true

	size := CommitSize{}

	// First accumulate all of the sizes for all parents:
	for _, parent := range commit.Parents {
		parentSize, parentOK := scanner.commitSizes[parent]
		if parentOK {
			if ok {
				size.addParent(parentSize)
			}
		} else {
			ok = false
			// Schedule this one to be computed:
			scanner.commitsToDo.Push(parent)
		}
	}

	// Now gather information about the tree:
	treeSize, treeOk := scanner.treeSizes[commit.Tree]
	if treeOk {
		if ok {
			size.addTree(treeSize)
		}
	} else {
		ok = false
		scanner.treesToDo.Push(commit.Tree)
	}

	if !ok {
		return CommitSize{}, 0, 0, NotYetKnown
	}

	// Now add one to the ancestor depth to account for this commit
	// itself:
	size.MaxAncestorDepth.Increment(1)
	return size, commit.Size, Count(len(commit.Parents)), nil
}
