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

	// The size of tags whose sizes have been looked up so far.
	tagSizes map[Oid]TagSize

	// Statistics about the overall history size:
	HistorySize HistorySize

	// The OIDs of commits and trees whose sizes are in the process of
	// being computed. This is, roughly, the call stack. As long as
	// there are no SHA-1 collisions, the sizes of these lists are
	// bounded:
	//
	// * The total number of trees is at most the total number of
	//   direct non-blob referents in all unique objects along a
	//   single lineage of descendants of the starting point.
	//
	// * The total number of commits is at most the total number of
	//   direct parents along a single ancestry path through history.
	//
	// * The total number of tags is at most the total number of a
	//   tags that refer to each other in a chain.
	toDo ToDoList
}

func NewSizeScanner(repo *Repository) (*SizeScanner, error) {
	scanner := &SizeScanner{
		repo:        repo,
		treeSizes:   make(map[Oid]TreeSize),
		blobSizes:   make(map[Oid]BlobSize),
		commitSizes: make(map[Oid]CommitSize),
		tagSizes:    make(map[Oid]TagSize),
	}
	return scanner, nil
}

func (scanner *SizeScanner) TypedObjectSize(
	spec string, oid Oid, objectType ObjectType, objectSize Count32,
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
		tagSize, err := scanner.TagSize(oid)
		return tagSize, err
	default:
		panic(fmt.Sprintf("object %v has unknown type", oid))
	}
}

func (scanner *SizeScanner) ObjectSize(spec string) (Oid, ObjectType, Size, error) {
	oid, objectType, objectSize, err := scanner.repo.ReadHeader(spec)
	if err != nil {
		return Oid{}, "missing", nil, err
	}

	size, err := scanner.TypedObjectSize(spec, oid, objectType, objectSize)
	return oid, objectType, size, err
}

func (scanner *SizeScanner) ReferenceSize(ref Reference) (Size, error) {
	size, err := scanner.TypedObjectSize(ref.Refname, ref.Oid, ref.ObjectType, ref.ObjectSize)
	if err != nil {
		return nil, err
	}
	scanner.recordReference(ref)
	return size, err
}

func (scanner *SizeScanner) OidObjectSize(oid Oid) (ObjectType, Size, error) {
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

	scanner.toDo.Push(&pendingTree{oid})
	err := scanner.fill()
	if err != nil {
		return TreeSize{}, err
	}

	// Now the size should be in the cache:
	s, ok = scanner.treeSizes[oid]
	if !ok {
		panic("queueTree() didn't fill tree")
	}
	return s, nil
}

func (scanner *SizeScanner) CommitSize(oid Oid) (CommitSize, error) {
	s, ok := scanner.commitSizes[oid]
	if ok {
		return s, nil
	}

	scanner.toDo.Push(&pendingCommit{oid})
	err := scanner.fill()
	if err != nil {
		return CommitSize{}, err
	}

	// Now the size should be in the cache:
	s, ok = scanner.commitSizes[oid]
	if !ok {
		panic("fill() didn't fill commit")
	}
	return s, nil
}

func (scanner *SizeScanner) TagSize(oid Oid) (TagSize, error) {
	s, ok := scanner.tagSizes[oid]
	if ok {
		return s, nil
	}

	scanner.toDo.Push(&pendingTag{oid})
	err := scanner.fill()
	if err != nil {
		return TagSize{}, err
	}

	// Now the size should be in the cache:
	s, ok = scanner.tagSizes[oid]
	if !ok {
		panic("fill() didn't fill tag")
	}
	return s, nil
}

func (scanner *SizeScanner) recordReference(ref Reference) {
	scanner.HistorySize.recordReference(ref)
}

func (scanner *SizeScanner) recordBlob(oid Oid, blobSize BlobSize) {
	scanner.blobSizes[oid] = blobSize
	scanner.HistorySize.recordBlob(blobSize)
}

func (scanner *SizeScanner) recordTree(oid Oid, treeSize TreeSize, size Count32, treeEntries Count32) {
	scanner.treeSizes[oid] = treeSize
	scanner.HistorySize.recordTree(treeSize, size, treeEntries)
}

func (scanner *SizeScanner) recordCommit(oid Oid, commitSize CommitSize, size Count32, parentCount Count32) {
	scanner.commitSizes[oid] = commitSize
	scanner.HistorySize.recordCommit(commitSize, size, parentCount)
}

func (scanner *SizeScanner) recordTag(oid Oid, tagSize TagSize, size Count32) {
	scanner.tagSizes[oid] = tagSize
	scanner.HistorySize.recordTag(tagSize, size)
}

type pending interface {
	run(*SizeScanner) error
}

type pendingTree struct {
	oid Oid
}

func (p *pendingTree) run(scanner *SizeScanner) error {
	// See if the object's size has been computed since it was
	// enqueued. This can happen if it is used in multiple places in
	// the ancestry graph.
	_, ok := scanner.treeSizes[p.oid]
	if ok {
		scanner.toDo.Drop()
		return nil
	}

	treeSize, size, treeEntries, err := scanner.queueTree(p.oid)
	if err == nil {
		scanner.recordTree(p.oid, treeSize, size, treeEntries)
		scanner.toDo.Drop()
	} else if err == NotYetKnown {
		// Let loop continue (the tree's constituents were added
		// to `treesToDo` by `queueTree()`).
	} else {
		return err
	}
	return nil
}

type pendingCommit struct {
	oid Oid
}

func (p *pendingCommit) run(scanner *SizeScanner) error {
	// See if the object's size has been computed since it was
	// enqueued. This can happen if it is used in multiple places
	// in the ancestry graph.
	_, ok := scanner.commitSizes[p.oid]
	if ok {
		scanner.toDo.Drop()
		return nil
	}

	commitSize, size, parentCount, err := scanner.queueCommit(p.oid)
	if err == nil {
		scanner.recordCommit(p.oid, commitSize, size, parentCount)
		scanner.toDo.Drop()
	} else if err == NotYetKnown {
		// Let loop continue (the commits's constituents were
		// added to `commitsToDo` and `treesToDo` by
		// `queueCommit()`).
	} else {
		return err
	}
	return nil
}

type pendingTag struct {
	oid Oid
}

func (p *pendingTag) run(scanner *SizeScanner) error {
	// See if the object's size has been computed since it was
	// enqueued. This can happen if it is used in multiple places
	// in the ancestry graph.
	_, ok := scanner.tagSizes[p.oid]
	if ok {
		scanner.toDo.Drop()
		return nil
	}

	tagSize, size, err := scanner.queueTag(p.oid)
	if err == nil {
		scanner.recordTag(p.oid, tagSize, size)
		scanner.toDo.Drop()
	} else if err == NotYetKnown {
		// Let loop continue (the tag's referent was added to
		// a todo list by `queueTag()`).
	} else {
		return err
	}
	return nil
}

// Compute the sizes of any trees listed in `scanner.commitsToDo` or
// `scanner.treesToDo`. This might involve computing the sizes of
// referred-to objects. Do this without recursion to avoid unlimited
// stack growth.
func (scanner *SizeScanner) fill() error {
	for scanner.toDo.Length() != 0 {
		p := scanner.toDo.Peek()

		err := p.run(scanner)
		if err != nil {
			return err
		}
	}

	// There is nothing left to do:
	return nil
}

// Compute and return the size of the tree with the specified `oid` if
// we already know the size of its constituents. If the constituents'
// sizes are not yet known but believed to be computable, add any
// unknown constituents to `treesToDo` and return an `NotYetKnown`
// error. If another error occurred while looking up an object, return
// that error. `oid` is not already in the cache.
func (scanner *SizeScanner) queueTree(oid Oid) (TreeSize, Count32, Count32, error) {
	var err error

	tree, err := scanner.repo.ReadTree(oid)
	if err != nil {
		return TreeSize{}, 0, 0, err
	}

	ok := true

	var entryCount Count32

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
		entryCount.Increment(1)

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
				scanner.toDo.Push(&pendingTree{entry.Oid})
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
	return size, NewCount32(uint64(len(tree.data))), entryCount, nil
}

// Compute and return the size of the commit with the specified `oid`
// if we already know the size of its constituents. If the
// constituents' sizes are not yet known but believed to be
// computable, add any unknown constituents to `commitsToDo` and
// `treesToDo` and return an `NotYetKnown` error. If another error
// occurred while looking up an object, return that error. `oid` is
// not already in the cache.
func (scanner *SizeScanner) queueCommit(oid Oid) (CommitSize, Count32, Count32, error) {
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
			scanner.toDo.Push(&pendingCommit{parent})
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
		scanner.toDo.Push(&pendingTree{commit.Tree})
	}

	if !ok {
		return CommitSize{}, 0, 0, NotYetKnown
	}

	// Now add one to the ancestor depth to account for this commit
	// itself:
	size.MaxAncestorDepth.Increment(1)
	return size, commit.Size, NewCount32(uint64(len(commit.Parents))), nil
}

// Compute and return the size of the annotated tag with the specified
// `oid` if we already know the size of its referent. If the
// referent's size is not yet known but believed to be computable, add
// it to the appropriate todo list and return an `NotYetKnown` error.
// If another error occurred while looking up an object, return that
// error. `oid` is not already in the cache.
func (scanner *SizeScanner) queueTag(oid Oid) (TagSize, Count32, error) {
	var err error

	tag, err := scanner.repo.ReadTag(oid)
	if err != nil {
		return TagSize{}, 0, err
	}

	size := TagSize{TagDepth: 1}
	ok := true
	switch tag.ReferentType {
	case "tag":
		referentSize, referentOK := scanner.tagSizes[tag.Referent]
		if referentOK {
			size.TagDepth.Increment(referentSize.TagDepth)
		} else {
			ok = false
			// Schedule this one to be computed:
			scanner.toDo.Push(&pendingTag{tag.Referent})
		}
	case "commit":
		_, referentOK := scanner.commitSizes[tag.Referent]
		if !referentOK {
			ok = false
			// Schedule this one to be computed:
			scanner.toDo.Push(&pendingCommit{tag.Referent})
		}
	case "tree":
		_, referentOK := scanner.treeSizes[tag.Referent]
		if !referentOK {
			ok = false
			// Schedule this one to be computed:
			scanner.toDo.Push(&pendingTree{tag.Referent})
		}
	case "blob":
		_, referentOK := scanner.commitSizes[tag.Referent]
		if !referentOK {
			_, err := scanner.BlobSize(tag.Referent)
			if err != nil {
				return TagSize{}, 0, err
			}
		}
	default:
	}

	if !ok {
		return TagSize{}, 0, NotYetKnown
	}

	// Now add one to the tag depth to account for this tag itself:
	return size, tag.Size, nil
}
