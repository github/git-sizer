package sizes

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

// A count of something, capped at math.MaxUint64.
type Count uint64

// The type of an object ("blob", "tree", "commit", "tag", "missing").
type Type string

// Return the sum of two Counts, capped at math.MaxUint64.
func addCapped(n1, n2 Count) Count {
	n := n1 + n2
	if n < n1 {
		// Overflow
		return math.MaxUint64
	}
	return n
}

// Increment `*n1` by `n2`, capped at math.MaxUint64.
func (n1 *Count) Increment(n2 Count) {
	*n1 = addCapped(*n1, n2)
}

// Adjust `*n1` to be `max(*n1, n2)`.
func (n1 *Count) AdjustMax(n2 Count) {
	if n2 > *n1 {
		*n1 = n2
	}
}

type Repository struct {
	path string

	batchCommand      *exec.Cmd
	batchStdin        io.WriteCloser
	batchStdoutWriter io.ReadCloser
	batchStdout       *bufio.Reader

	checkCommand      *exec.Cmd
	checkStdin        io.WriteCloser
	checkStdoutWriter io.ReadCloser
	checkStdout       *bufio.Reader
}

func NewRepository(path string) (*Repository, error) {
	batchCommand := exec.Command("git", "-C", path, "cat-file", "--batch")
	batchStdin, err := batchCommand.StdinPipe()
	if err != nil {
		return nil, err
	}
	batchStdout, err := batchCommand.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = batchCommand.Start()
	if err != nil {
		return nil, err
	}

	checkCommand := exec.Command("git", "-C", path, "cat-file", "--batch-check")
	checkStdin, err := checkCommand.StdinPipe()
	if err != nil {
		return nil, err
	}
	checkStdout, err := checkCommand.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = checkCommand.Start()
	if err != nil {
		return nil, err
	}

	return &Repository{
		path: path,

		batchCommand:      batchCommand,
		batchStdin:        batchStdin,
		batchStdoutWriter: batchStdout,
		batchStdout:       bufio.NewReader(batchStdout),

		checkCommand:      checkCommand,
		checkStdin:        checkStdin,
		checkStdoutWriter: checkStdout,
		checkStdout:       bufio.NewReader(checkStdout),
	}, nil
}

type Oid [20]byte

func NewOid(s string) (Oid, error) {
	oidBytes, err := hex.DecodeString(s)
	if err != nil {
		return Oid{}, err
	}
	if len(oidBytes) != 20 {
		return Oid{}, errors.New("hex oid has the wrong length")
	}
	var oid Oid
	copy(oid[0:20], oidBytes)
	return oid, nil
}

func (oid Oid) String() string {
	return hex.EncodeToString(oid[:])
}

// Parse a `cat-file --batch[-check]` output header line (including
// the trailing LF). `spec` is used in error messages.
func (repo *Repository) parseHeader(spec string, header string) (Oid, Type, Count, error) {
	header = header[:len(header)-1]
	words := strings.Split(header, " ")
	if words[len(words)-1] == "missing" {
		return Oid{}, "missing", 0, errors.New(fmt.Sprintf("missing object %s", spec))
	}

	oid, err := NewOid(words[0])
	if err != nil {
		return Oid{}, "missing", 0, err
	}

	size, err := strconv.ParseUint(words[2], 10, 0)
	if err != nil {
		return Oid{}, "missing", 0, err
	}
	return oid, Type(words[1]), Count(size), nil
}

func (repo *Repository) ReadHeader(spec string) (Oid, Type, Count, error) {
	fmt.Fprintf(repo.checkStdin, "%s\n", spec)
	header, err := repo.checkStdout.ReadString('\n')
	if err != nil {
		return Oid{}, "missing", 0, err
	}
	return repo.parseHeader(spec, header)
}

func (repo *Repository) readObject(spec string) (Oid, Type, []byte, error) {
	fmt.Fprintf(repo.batchStdin, "%s\n", spec)
	header, err := repo.batchStdout.ReadString('\n')
	if err != nil {
		return Oid{}, "missing", []byte{}, err
	}
	oid, objectType, size, err := repo.parseHeader(spec, header)
	if err != nil {
		return Oid{}, "missing", []byte{}, err
	}
	// +1 for LF:
	data := make([]byte, size+1)
	rest := data
	for len(rest) > 0 {
		n, err := repo.batchStdout.Read(rest)
		if err != nil {
			return Oid{}, "missing", []byte{}, err
		}
		rest = rest[n:]
	}
	// -1 to remove LF:
	return oid, objectType, data[:len(data)-1], nil
}

type Commit struct {
	Size    Count
	Parents []Oid
	Tree    Oid
}

func (repo *Repository) ReadCommit(oid Oid) (*Commit, error) {
	oid, objectType, data, err := repo.readObject(oid.String())
	if err != nil {
		return nil, err
	}
	if objectType != "commit" {
		return nil, fmt.Errorf("expected commit; found %s for object %s", objectType, oid)
	}
	headerEnd := bytes.Index(data, []byte("\n\n"))
	if headerEnd == -1 {
		return nil, fmt.Errorf("commit %s has no header separator", oid)
	}
	header := string(data[:headerEnd+1])
	var parents []Oid
	var tree Oid
	var treeFound bool
	for len(header) != 0 {
		keyEnd := strings.IndexByte(header, ' ')
		if keyEnd == -1 {
			return nil, fmt.Errorf("malformed header in commit %s", oid)
		}
		key := header[:keyEnd]
		header = header[keyEnd+1:]
		valueEnd := strings.IndexByte(header, '\n')
		if valueEnd == -1 {
			return nil, fmt.Errorf("malformed header in commit %s", oid)
		}
		value := header[:valueEnd]
		header = header[valueEnd+1:]
		switch key {
		case "parent":
			parent, err := NewOid(value)
			if err != nil {
				return nil, fmt.Errorf("malformed parent header in commit %s", oid)
			}
			parents = append(parents, parent)
		case "tree":
			if treeFound {
				return nil, fmt.Errorf("multiple trees found in commit %s", oid)
			}
			tree, err = NewOid(value)
			if err != nil {
				return nil, fmt.Errorf("malformed tree header in commit %s", oid)
			}
			treeFound = true
		}
	}
	return &Commit{
		Size:    Count(len(data)),
		Parents: parents,
		Tree:    tree,
	}, nil
}

type Tree struct {
	data []byte
}

func (repo *Repository) ReadTree(oid Oid) (*Tree, error) {
	oid, objectType, data, err := repo.readObject(oid.String())
	if err != nil {
		return nil, err
	}
	if objectType != "tree" {
		return nil, errors.New(fmt.Sprintf("expected tree; found %s for object %s", objectType, oid))
	}
	return &Tree{data}, nil
}

type TreeEntry struct {
	Name     string
	Oid      Oid
	Type     Type
	Filemode uint
}

type TreeIter struct {
	// The as-yet-unread part of the tree's data.
	data []byte
}

func (tree *Tree) Iter() *TreeIter {
	return &TreeIter{
		data: tree.data,
	}
}

func (iter *TreeIter) NextEntry(entry *TreeEntry) (bool, error) {
	if len(iter.data) == 0 {
		return false, nil
	}

	spAt := bytes.IndexByte(iter.data, ' ')
	if spAt < 0 {
		return false, errors.New("failed to find SP after mode")
	}
	mode, err := strconv.ParseUint(string(iter.data[:spAt]), 8, 32)
	if err != nil {
		return false, err
	}
	entry.Filemode = uint(mode)

	iter.data = iter.data[spAt+1:]
	nulAt := bytes.IndexByte(iter.data, 0)
	if nulAt < 0 {
		return false, errors.New("failed to find NUL after filename")
	}

	entry.Name = string(iter.data[:nulAt])

	iter.data = iter.data[nulAt+1:]
	if len(iter.data) < 20 {
		return false, errors.New("tree entry ends unexpectedly")
	}

	copy(entry.Oid[0:20], iter.data[0:20])
	iter.data = iter.data[20:]

	return true, nil
}

type Size interface {
	fmt.Stringer
}

type BlobSize Count

func (s BlobSize) String() string {
	return fmt.Sprintf("blob_size=%d", Count(s))
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
		s.MaxPathLength.AdjustMax(addCapped(Count(len(filename))+1, s2.MaxPathLength))
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

// Record that the object has a blob of the specified `size` as a
// direct descendant.
func (s *TreeSize) addBlob(filename string, size BlobSize) {
	s.MaxPathDepth.AdjustMax(1)
	s.MaxPathLength.AdjustMax(Count(len(filename)))
	s.ExpandedBlobSize.Increment(Count(size))
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

func (cache *SizeCache) ObjectSize(spec string) (Oid, Type, Size, error) {
	oid, objectType, objectSize, err := cache.repo.ReadHeader(spec)
	if err != nil {
		return Oid{}, "missing", nil, err
	}

	switch objectType {
	case "blob":
		blobSize := BlobSize(objectSize)
		cache.blobSizes[oid] = blobSize
		return oid, "blob", blobSize, nil
	case "tree":
		treeSize, err := cache.TreeSize(oid)
		return oid, "tree", treeSize, err
	case "commit":
		commitSize, err := cache.CommitSize(oid)
		return oid, "commit", commitSize, err
	case "tag":
		return oid, "tag", nil, fmt.Errorf("object %v has unexpected type '%s'", oid, objectType)
	default:
		panic(fmt.Sprintf("object %v has unknown type", oid))
	}
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
			return 0, err
		}
		if objectType != "blob" {
			return 0, fmt.Errorf("object %s is a %s, not a blob", oid, objectType)
		}
		size = BlobSize(objectSize)
		cache.blobSizes[oid] = size
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

			s, err := cache.queueTree(oid)
			if err == nil {
				cache.treeSizes[oid] = s
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

			s, err := cache.queueCommit(oid)
			if err == nil {
				cache.commitSizes[oid] = s
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
func (cache *SizeCache) queueCommit(oid Oid) (CommitSize, error) {
	var err error

	commit, err := cache.repo.ReadCommit(oid)
	if err != nil {
		return CommitSize{}, err
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
		return CommitSize{}, NotYetKnown
	}

	// Now add one to the ancestor depth to account for this commit
	// itself:
	size.MaxAncestorDepth.Increment(1)
	return size, nil
}

// Compute and return the size of the tree with the specified `oid` if
// we already know the size of its constituents. If the constituents'
// sizes are not yet known but believed to be computable, add any
// unknown constituents to `treesToDo` and return an `NotYetKnown`
// error. If another error occurred while looking up an object, return
// that error. `oid` is not already in the cache.
func (cache *SizeCache) queueTree(oid Oid) (TreeSize, error) {
	var err error

	tree, err := cache.repo.ReadTree(oid)
	if err != nil {
		return TreeSize{}, err
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
			return TreeSize{}, err
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
					return TreeSize{}, err
				}
				size.addBlob(entry.Name, blobSize)
			}
		}
	}

	if !ok {
		return TreeSize{}, NotYetKnown
	}

	// Now add one to the depth and to the tree count to account for
	// this tree itself:
	size.MaxPathDepth.Increment(1)
	size.MaxTreeEntries.AdjustMax(entryCount)
	return size, nil
}
