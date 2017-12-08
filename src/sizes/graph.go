package sizes

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sync"
)

func ScanRepositoryUsingGraph(repo *Repository, filter ReferenceFilter) (HistorySize, error) {
	graph := NewGraph()

	refIter, err := repo.NewReferenceIter()
	if err != nil {
		return HistorySize{}, err
	}
	defer refIter.Close()

	iter, in, err := repo.NewObjectIter("--stdin", "--topo-order")
	if err != nil {
		return HistorySize{}, err
	}
	defer iter.Close()

	errChan := make(chan error, 1)
	// Feed the references that we want into the stdin of the object
	// iterator:
	go func() {
		defer in.Close()
		bufin := bufio.NewWriter(in)
		defer bufin.Flush()

		for {
			ref, ok, err := refIter.Next()
			if err != nil {
				errChan <- err
				return
			}
			if !ok {
				break
			}
			if !filter(ref) {
				continue
			}
			graph.RegisterReference(ref)
			_, err = bufin.WriteString(ref.Oid.String())
			if err != nil {
				errChan <- err
				return
			}
			err = bufin.WriteByte('\n')
			if err != nil {
				errChan <- err
				return
			}
		}
		errChan <- nil
	}()

	type ObjectHeader struct {
		oid        Oid
		objectSize Count32
	}

	// We process the blobs right away, but record these other types
	// of objects for later processing:
	var trees, commits, tags []ObjectHeader

	for {
		oid, objectType, objectSize, err := iter.Next()
		if err != nil {
			if err != io.EOF {
				return HistorySize{}, err
			}
			break
		}
		switch objectType {
		case "blob":
			graph.RegisterBlob(oid, objectSize)
		case "tree":
			trees = append(trees, ObjectHeader{oid, objectSize})
		case "commit":
			commits = append(commits, ObjectHeader{oid, objectSize})
		case "tag":
			tags = append(tags, ObjectHeader{oid, objectSize})
		default:
			err = fmt.Errorf("unexpected object type: %s", objectType)
		}
	}

	err = <-errChan
	if err != nil {
		return HistorySize{}, err
	}

	objectIter, objectIn, err := repo.NewBatchObjectIter()
	if err != nil {
		return HistorySize{}, err
	}
	defer objectIter.Close()

	go func() {
		defer objectIn.Close()
		bufin := bufio.NewWriter(objectIn)
		defer bufin.Flush()

		for _, obj := range trees {
			_, err := bufin.WriteString(obj.oid.String())
			if err != nil {
				errChan <- err
				return
			}
			err = bufin.WriteByte('\n')
			if err != nil {
				errChan <- err
				return
			}
		}

		for i := len(commits); i > 0; i-- {
			obj := commits[i-1]
			_, err := bufin.WriteString(obj.oid.String())
			if err != nil {
				errChan <- err
				return
			}
			err = bufin.WriteByte('\n')
			if err != nil {
				errChan <- err
				return
			}
		}

		for _, obj := range tags {
			_, err := bufin.WriteString(obj.oid.String())
			if err != nil {
				errChan <- err
				return
			}
			err = bufin.WriteByte('\n')
			if err != nil {
				errChan <- err
				return
			}
		}

		errChan <- nil
	}()

	for _ = range trees {
		oid, objectType, _, data, err := objectIter.Next()
		if err != nil {
			if err != io.EOF {
				return HistorySize{}, err
			}
			return HistorySize{}, errors.New("fewer trees read than expected")
		}
		if objectType != "tree" {
			return HistorySize{}, fmt.Errorf("expected tree; read %#v", objectType)
		}
		tree, err := ParseTree(oid, data)
		if err != nil {
			return HistorySize{}, err
		}
		err = graph.RegisterTree(oid, tree)
		if err != nil {
			return HistorySize{}, err
		}
	}

	for range commits {
		oid, objectType, _, data, err := objectIter.Next()
		if err != nil {
			if err != io.EOF {
				return HistorySize{}, err
			}
			return HistorySize{}, errors.New("fewer commits read than expected")
		}
		if objectType != "commit" {
			return HistorySize{}, fmt.Errorf("expected commit; read %#v", objectType)
		}
		commit, err := ParseCommit(oid, data)
		if err != nil {
			return HistorySize{}, err
		}
		graph.RegisterCommit(oid, commit)
	}

	for range tags {
		oid, objectType, _, data, err := objectIter.Next()
		if err != nil {
			if err != io.EOF {
				return HistorySize{}, err
			}
			return HistorySize{}, errors.New("fewer tags read than expected")
		}
		if objectType != "tag" {
			return HistorySize{}, fmt.Errorf("expected tag; read %#v", objectType)
		}
		tag, err := ParseTag(oid, data)
		if err != nil {
			return HistorySize{}, err
		}
		graph.RegisterTag(oid, tag)
	}

	err = <-errChan
	if err != nil {
		return HistorySize{}, err
	}

	return graph.HistorySize(), nil
}

// An object graph that is being built up.
type Graph struct {
	repo *Repository

	blobLock  sync.Mutex
	blobSizes map[Oid]BlobSize

	treeLock    sync.Mutex
	treeRecords map[Oid]*treeRecord
	treeSizes   map[Oid]TreeSize

	commitLock  sync.Mutex
	commitSizes map[Oid]CommitSize

	tagLock    sync.Mutex
	tagRecords map[Oid]*tagRecord
	tagSizes   map[Oid]TagSize

	// Statistics about the overall history size:
	historyLock sync.Mutex
	historySize HistorySize
}

func NewGraph() *Graph {
	return &Graph{
		blobSizes: make(map[Oid]BlobSize),

		treeRecords: make(map[Oid]*treeRecord),
		treeSizes:   make(map[Oid]TreeSize),

		commitSizes: make(map[Oid]CommitSize),

		tagRecords: make(map[Oid]*tagRecord),
		tagSizes:   make(map[Oid]TagSize),
	}
}

func (g *Graph) RegisterReference(ref Reference) {
	g.historyLock.Lock()
	g.historySize.recordReference(g, ref)
	g.historyLock.Unlock()
}

func (g *Graph) HistorySize() HistorySize {
	g.treeLock.Lock()
	defer g.treeLock.Unlock()
	g.tagLock.Lock()
	defer g.tagLock.Unlock()
	g.historyLock.Lock()
	defer g.historyLock.Unlock()
	if len(g.treeRecords) != 0 {
		panic(fmt.Sprintf("%d tree records remain!", len(g.treeRecords)))
	}
	if len(g.tagRecords) != 0 {
		panic(fmt.Sprintf("%d tag records remain!", len(g.tagRecords)))
	}
	return g.historySize
}

// Record that the specified `oid` is a blob with the specified size.
func (g *Graph) RegisterBlob(oid Oid, objectSize Count32) {
	size := BlobSize{Size: objectSize}
	// There are no listeners. Since this is a blob, we know all that
	// we need to know about it. So skip the record and just fill in
	// the size.
	g.blobLock.Lock()
	g.blobSizes[oid] = size
	g.blobLock.Unlock()

	g.historyLock.Lock()
	g.historySize.recordBlob(g, oid, size)
	g.historyLock.Unlock()
}

// The `Require*Size` functions behave as follows:
//
// * If the size of the object with name `oid` is already known. In
//   this case, return true as the second value.
//
// * If the size of the object is not yet known, then register the
//   listener to be informed some time in the future when the size is
//   known. In this case, return false as the second value.

func (g *Graph) GetBlobSize(oid Oid) BlobSize {
	// See if we already know the size:
	size, ok := g.blobSizes[oid]
	if !ok {
		panic("blob size not known")
	}
	return size
}

func (g *Graph) RequireTreeSize(oid Oid, listener func(TreeSize)) (TreeSize, bool) {
	g.treeLock.Lock()

	size, ok := g.treeSizes[oid]
	if ok {
		g.treeLock.Unlock()

		return size, true
	}

	record, ok := g.treeRecords[oid]
	if !ok {
		record = newTreeRecord(oid)
		g.treeRecords[oid] = record
	}
	record.addListener(listener)

	g.treeLock.Unlock()

	return TreeSize{}, false
}

func (g *Graph) GetTreeSize(oid Oid) TreeSize {
	g.treeLock.Lock()

	size, ok := g.treeSizes[oid]
	if !ok {
		panic("tree size not available!")
	}
	g.treeLock.Unlock()
	return size
}

// Record that the specified `oid` is the specified `tree`.
func (g *Graph) RegisterTree(oid Oid, tree *Tree) error {
	g.treeLock.Lock()

	if _, ok := g.treeSizes[oid]; ok {
		panic(fmt.Sprintf("tree %s registered twice!", oid))
	}

	// See if we already have a record for this tree:
	record, ok := g.treeRecords[oid]
	if !ok {
		record = newTreeRecord(oid)
		g.treeRecords[oid] = record
	}

	g.treeLock.Unlock()

	// Let the record take care of the rest:
	return record.initialize(g, tree)
}

func (g *Graph) finalizeTreeSize(oid Oid, size TreeSize, objectSize Count32, treeEntries Count32) {
	g.treeLock.Lock()
	g.treeSizes[oid] = size
	delete(g.treeRecords, oid)
	g.treeLock.Unlock()

	g.historyLock.Lock()
	g.historySize.recordTree(g, oid, size, objectSize, treeEntries)
	g.historyLock.Unlock()
}

type treeRecord struct {
	oid Oid

	// Limit to only one mutator at a time.
	lock sync.Mutex

	// The size of this object, in bytes. Initialized iff pending !=
	// -1.
	objectSize Count32

	// The number of entries directly in this tree. Initialized iff
	// pending != -1.
	entryCount Count32

	// The size of the items we know so far:
	size TreeSize

	// The number of dependencies whose callbacks haven't yet been
	// invoked. Before the tree itself has been read, this is set to
	// -1. After it has been read, it counts down the number of
	// dependencies that are still unknown. When this number goes to
	// zero, then `size` is the final answer.
	pending int

	// The listeners waiting to learn our size.
	listeners []func(TreeSize)
}

func newTreeRecord(oid Oid) *treeRecord {
	return &treeRecord{
		oid:     oid,
		size:    TreeSize{ExpandedTreeCount: 1},
		pending: -1,
	}
}

// Initialize `r` (which is empty) based on `tree`.
func (r *treeRecord) initialize(g *Graph, tree *Tree) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.objectSize = NewCount32(uint64(len(tree.data)))
	r.pending = 0

	iter := tree.Iter()
	for {
		entry, ok, err := iter.NextEntry()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		name := entry.Name

		switch {
		case entry.Filemode&0170000 == 0040000:
			// Tree
			listener := func(size TreeSize) {
				r.lock.Lock()
				defer r.lock.Unlock()

				r.size.addDescendent(name, size)
				r.pending--
				r.maybeFinalize(g)
			}
			treeSize, ok := g.RequireTreeSize(entry.Oid, listener)
			if ok {
				r.size.addDescendent(name, treeSize)
			} else {
				r.pending++
			}
			r.entryCount.Increment(1)

		case entry.Filemode&0170000 == 0160000:
			// Commit (i.e., submodule)
			r.size.addSubmodule(name)
			r.entryCount.Increment(1)

		case entry.Filemode&0170000 == 0120000:
			// Symlink
			r.size.addLink(name)
			r.entryCount.Increment(1)

		default:
			// Blob
			blobSize := g.GetBlobSize(entry.Oid)
			r.size.addBlob(name, blobSize)
			r.entryCount.Increment(1)
		}
	}

	r.maybeFinalize(g)

	return nil
}

func (r *treeRecord) maybeFinalize(g *Graph) {
	if r.pending == 0 {
		// Add one for this tree itself:
		r.size.MaxPathDepth.Increment(1)

		g.finalizeTreeSize(r.oid, r.size, r.objectSize, r.entryCount)
		for _, listener := range r.listeners {
			listener(r.size)
		}
	}
}

// Must be called either before `r` is published or while it is
// locked.
func (r *treeRecord) addListener(listener func(TreeSize)) {
	r.listeners = append(r.listeners, listener)
}

func (g *Graph) GetCommitSize(oid Oid) CommitSize {
	g.commitLock.Lock()

	size, ok := g.commitSizes[oid]
	if !ok {
		panic("commit is not available")
	}
	g.commitLock.Unlock()

	return size
}

// Record that the specified `oid` is the specified `commit`.
func (g *Graph) RegisterCommit(oid Oid, commit *Commit) {
	g.commitLock.Lock()
	if _, ok := g.commitSizes[oid]; ok {
		panic(fmt.Sprintf("commit %s registered twice!", oid))
	}
	g.commitLock.Unlock()

	// The number of direct parents of this commit.
	parentCount := NewCount32(uint64(len(commit.Parents)))

	// The size of the items we know so far:
	size := CommitSize{}

	// The tree:
	treeSize := g.GetTreeSize(commit.Tree)
	size.addTree(treeSize)

	for _, parent := range commit.Parents {
		parentSize := g.GetCommitSize(parent)
		size.addParent(parentSize)
	}

	// Add 1 for this commit itself:
	size.MaxAncestorDepth.Increment(1)

	g.commitLock.Lock()
	g.commitSizes[oid] = size
	g.commitLock.Unlock()

	g.historyLock.Lock()
	g.historySize.recordCommit(g, oid, size, commit.Size, parentCount)
	g.historyLock.Unlock()
}

func (g *Graph) RequireTagSize(oid Oid, listener func(TagSize)) (TagSize, bool) {
	g.tagLock.Lock()

	size, ok := g.tagSizes[oid]
	if ok {
		g.tagLock.Unlock()

		return size, true
	}

	record, ok := g.tagRecords[oid]
	if !ok {
		record = newTagRecord(oid)
		g.tagRecords[oid] = record
	}
	record.addListener(listener)

	g.tagLock.Unlock()

	return TagSize{}, false
}

// Record that the specified `oid` is the specified `tag`.
func (g *Graph) RegisterTag(oid Oid, tag *Tag) {
	g.tagLock.Lock()

	if _, ok := g.tagSizes[oid]; ok {
		panic(fmt.Sprintf("tag %s registered twice!", oid))
	}

	// See if we already have a record for this tag:
	record, ok := g.tagRecords[oid]
	if !ok {
		record = newTagRecord(oid)
		g.tagRecords[oid] = record
	}

	g.tagLock.Unlock()

	// Let the record take care of the rest:
	record.initialize(g, tag)
}

func (g *Graph) finalizeTagSize(oid Oid, size TagSize, objectSize Count32) {
	g.tagLock.Lock()
	g.tagSizes[oid] = size
	delete(g.tagRecords, oid)
	g.tagLock.Unlock()

	g.historyLock.Lock()
	g.historySize.recordTag(g, oid, size, objectSize)
	g.historyLock.Unlock()
}

type tagRecord struct {
	oid Oid

	// Limit to only one mutator at a time.
	lock sync.Mutex

	// The size of this commit object in bytes.
	objectSize Count32

	// The size of the items we know so far:
	size TagSize

	// See treeRecord.pending. This value can be at most 1 for a Tag.
	pending int8

	// See treeRecord.listeners.
	listeners []func(TagSize)
}

func newTagRecord(oid Oid) *tagRecord {
	return &tagRecord{
		oid:     oid,
		pending: -1,
	}
}

// Initialize `r` (which is empty) based on `tag`.
func (r *tagRecord) initialize(g *Graph, tag *Tag) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.objectSize = tag.Size
	r.pending = 0
	r.size.TagDepth = 1

	// The only thing that a tag cares about its ancestors is how many
	// tags have to be traversed to get to a real object. So we only
	// need to listen to the referent if it is another tag.
	switch tag.ReferentType {
	case "tag":
		listener := func(size TagSize) {
			r.lock.Lock()
			defer r.lock.Unlock()

			r.size.TagDepth.Increment(size.TagDepth)
			r.pending--
			r.maybeFinalize(g)
		}
		tagSize, ok := g.RequireTagSize(tag.Referent, listener)
		if ok {
			r.size.TagDepth.Increment(tagSize.TagDepth)
		} else {
			r.pending++
		}
	case "commit":
	case "tree":
	case "blob":
	default:
	}

	if r.pending == 0 {
		g.finalizeTagSize(r.oid, r.size, r.objectSize)
	}
}

func (r *tagRecord) maybeFinalize(g *Graph) {
	if r.pending == 0 {
		g.finalizeTagSize(r.oid, r.size, r.objectSize)
		for _, listener := range r.listeners {
			listener(r.size)
		}
	}
}

// Must be called either before `r` is published or while it is
// locked.
func (r *tagRecord) addListener(listener func(TagSize)) {
	r.listeners = append(r.listeners, listener)
}
