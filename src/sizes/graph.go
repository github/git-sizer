package sizes

import (
	"bufio"
	"fmt"
	"io"
	"sync"
)

func ScanRepositoryUsingGraph(repo *Repository, filter ReferenceFilter) (HistorySize, error) {
	// FIXME: Process references and use reference filter.

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
			err = graph.RegisterBlob(oid, objectSize)
		case "tree":
			var tree *Tree
			tree, err = repo.ReadTree(oid)
			if err == nil {
				err = graph.RegisterTree(oid, tree)
			}
		case "commit":
			var commit *Commit
			commit, err = repo.ReadCommit(oid)
			if err == nil {
				err = graph.RegisterCommit(oid, commit)
			}
		case "tag":
			var tag *Tag
			tag, err = repo.ReadTag(oid)
			if err == nil {
				err = graph.RegisterTag(oid, tag)
			}
		default:
			err = fmt.Errorf("unexpected object type: %s", objectType)
		}
		if err != nil {
			return HistorySize{}, err
		}
	}

	err = <-errChan
	if err != nil {
		return HistorySize{}, err
	}

	return graph.HistorySize, nil
}

// An object graph that is being built up.
type Graph struct {
	repo *Repository

	blobLock    sync.Mutex
	blobRecords map[Oid]*blobRecord
	blobSizes   map[Oid]BlobSize

	treeLock    sync.Mutex
	treeRecords map[Oid]*treeRecord
	treeSizes   map[Oid]TreeSize

	commitLock    sync.Mutex
	commitRecords map[Oid]*commitRecord
	commitSizes   map[Oid]CommitSize

	tagLock    sync.Mutex
	tagRecords map[Oid]*tagRecord
	tagSizes   map[Oid]TagSize

	// Statistics about the overall history size:
	historyLock sync.Mutex
	HistorySize HistorySize
}

func NewGraph() *Graph {
	return &Graph{
		blobRecords: make(map[Oid]*blobRecord),
		blobSizes:   make(map[Oid]BlobSize),

		treeRecords: make(map[Oid]*treeRecord),
		treeSizes:   make(map[Oid]TreeSize),

		commitRecords: make(map[Oid]*commitRecord),
		commitSizes:   make(map[Oid]CommitSize),

		tagRecords: make(map[Oid]*tagRecord),
		tagSizes:   make(map[Oid]TagSize),
	}
}

func (g *Graph) RegisterReference(ref Reference) {
	g.historyLock.Lock()
	g.HistorySize.recordReference(ref)
	g.historyLock.Unlock()
}

// Record that the specified `oid` is a blob with the specified size.
func (g *Graph) RegisterBlob(oid Oid, objectSize Count32) error {
	g.blobLock.Lock()

	// It might be that we already have a record for this blob, in
	// which case we tell it the size, and it does the rest:
	record, ok := g.blobRecords[oid]
	if ok {
		g.blobLock.Unlock()
		record.record(g, oid, objectSize)
		return nil
	}

	// There are no listeners. Since this is a blob, we know all that
	// we need to know about it. So skip the record and just fill in
	// the size.
	g.blobSizes[oid] = BlobSize{Size: objectSize}
	g.blobLock.Unlock()
	return nil
}

// The `Require*Size` functions behave as follows:
//
// * If the size of the object with name `oid` is already known. In
//   this case, return true as the second value.
//
// * If the size of the object is not yet known, then register the
//   listener to be informed some time in the future when the size is
//   known. In this case, return false as the second value.

func (g *Graph) RequireBlobSize(oid Oid, listener func(BlobSize)) (BlobSize, bool) {
	g.blobLock.Lock()

	// See if we already know the size:
	size, ok := g.blobSizes[oid]
	if ok {
		g.blobLock.Unlock()
		return size, true
	}

	// We don't. Maybe we already have a record?
	record, ok := g.blobRecords[oid]
	if !ok {
		// We don't already have a record, so create and save an empty
		// one:
		record = &blobRecord{}
		g.blobRecords[oid] = record
	}

	record.addListener(listener)

	g.blobLock.Unlock()

	return BlobSize{}, false
}

func (g *Graph) finalizeBlobSize(oid Oid, size BlobSize) {
	g.blobLock.Lock()
	g.blobSizes[oid] = size
	delete(g.blobRecords, oid)
	g.blobLock.Unlock()

	g.historyLock.Lock()
	g.HistorySize.recordBlob(size)
	g.historyLock.Unlock()
}

type blobRecord struct {
	// Limit to only one mutator at a time.
	lock sync.Mutex

	// Is the size known yet?
	known bool

	// The size, if known:
	size BlobSize

	// The listeners waiting to learn our size.
	listeners []func(BlobSize)
}

func emptyBlobRecord() *blobRecord {
	return &blobRecord{
		known: false,
	}
}

// Must be called either before `r` is published or while it is
// locked.
func (r *blobRecord) addListener(listener func(BlobSize)) {
	r.listeners = append(r.listeners, listener)
}

func (r *blobRecord) record(g *Graph, oid Oid, objectSize Count32) {
	r.lock.Lock()

	r.size = BlobSize{Size: objectSize}
	r.known = true

	r.lock.Unlock()

	g.finalizeBlobSize(oid, r.size)

	for _, listener := range r.listeners {
		listener(r.size)
	}
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
	g.HistorySize.recordTree(size, objectSize, treeEntries)
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
			listener := func(size BlobSize) {
				r.lock.Lock()
				defer r.lock.Unlock()

				r.size.addBlob(name, size)
				r.pending--
				r.maybeFinalize(g)
			}
			blobSize, ok := g.RequireBlobSize(entry.Oid, listener)
			if ok {
				r.size.addBlob(name, blobSize)
			} else {
				r.pending++
			}
			r.entryCount.Increment(1)
		}
	}

	r.maybeFinalize(g)

	return nil
}

func (r *treeRecord) maybeFinalize(g *Graph) {
	if r.pending == 0 {
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

func (g *Graph) RequireCommitSize(oid Oid, listener func(CommitSize)) (CommitSize, bool) {
	g.commitLock.Lock()

	size, ok := g.commitSizes[oid]
	if ok {
		g.commitLock.Unlock()

		return size, true
	}

	record, ok := g.commitRecords[oid]
	if !ok {
		record = newCommitRecord(oid)
		g.commitRecords[oid] = record
	}
	record.addListener(listener)

	g.commitLock.Unlock()

	return CommitSize{}, false
}

// Record that the specified `oid` is the specified `commit`.
func (g *Graph) RegisterCommit(oid Oid, commit *Commit) error {
	g.commitLock.Lock()

	if _, ok := g.commitSizes[oid]; ok {
		panic(fmt.Sprintf("commit %s registered twice!", oid))
	}

	// See if we already have a record for this commit:
	record, ok := g.commitRecords[oid]
	if !ok {
		record = newCommitRecord(oid)
		g.commitRecords[oid] = record
	}

	g.commitLock.Unlock()

	// Let the record take care of the rest:
	return record.initialize(g, commit)
}

func (g *Graph) finalizeCommitSize(oid Oid, size CommitSize, objectSize Count32, parentCount Count32) {
	g.commitLock.Lock()
	g.commitSizes[oid] = size
	delete(g.commitRecords, oid)
	g.commitLock.Unlock()

	g.historyLock.Lock()
	g.HistorySize.recordCommit(size, objectSize, parentCount)
	g.historyLock.Unlock()
}

type commitRecord struct {
	oid Oid

	// Limit to only one mutator at a time.
	lock sync.Mutex

	// The size of this commit object in bytes.
	objectSize Count32

	// The number of direct parents of this commit.
	parentCount Count32

	// The size of the items we know so far:
	size CommitSize

	// See treeRecord.pending.
	pending int

	// See treeRecord.listeners.
	listeners []func(CommitSize)
}

func newCommitRecord(oid Oid) *commitRecord {
	return &commitRecord{
		oid:     oid,
		pending: -1,
	}
}

// Initialize `r` (which is empty) based on `commit`.
func (r *commitRecord) initialize(g *Graph, commit *Commit) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.objectSize = commit.Size
	r.pending = 0

	// The tree:
	listener := func(size TreeSize) {
		r.lock.Lock()
		defer r.lock.Unlock()

		r.size.addTree(size)
		r.pending--
		r.maybeFinalize(g)
	}
	treeSize, ok := g.RequireTreeSize(commit.Tree, listener)
	if ok {
		r.size.addTree(treeSize)
	} else {
		r.pending++
	}

	for _, parent := range commit.Parents {
		listener := func(size CommitSize) {
			r.lock.Lock()
			defer r.lock.Unlock()

			r.size.addParent(size)
			r.pending--
			r.maybeFinalize(g)
		}
		parentSize, ok := g.RequireCommitSize(parent, listener)
		if ok {
			r.size.addParent(parentSize)
		} else {
			r.pending++
		}
	}

	r.maybeFinalize(g)

	return nil
}

func (r *commitRecord) maybeFinalize(g *Graph) {
	if r.pending == 0 {
		g.finalizeCommitSize(r.oid, r.size, r.objectSize, r.parentCount)
		for _, listener := range r.listeners {
			listener(r.size)
		}
	}
}

// Must be called either before `r` is published or while it is
// locked.
func (r *commitRecord) addListener(listener func(CommitSize)) {
	r.listeners = append(r.listeners, listener)
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
func (g *Graph) RegisterTag(oid Oid, tag *Tag) error {
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
	return record.initialize(g, tag)
}

func (g *Graph) finalizeTagSize(oid Oid, size TagSize, objectSize Count32) {
	g.tagLock.Lock()
	g.tagSizes[oid] = size
	delete(g.tagRecords, oid)
	g.tagLock.Unlock()

	g.historyLock.Lock()
	g.HistorySize.recordTag(size, objectSize)
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
func (r *tagRecord) initialize(g *Graph, tag *Tag) error {
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

	return nil
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
