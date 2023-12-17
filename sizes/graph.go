package sizes

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/github/git-sizer/counts"
	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/meter"
)

type Root interface {
	Name() string
	OID() git.OID
	Walk() bool
}

type ReferenceRoot interface {
	Root
	Reference() git.Reference
	Groups() []RefGroupSymbol
}

// ScanRepositoryUsingGraph scans `repo`, using `rg` to decide which
// references to scan and how to group them. `nameStyle` specifies
// whether the output should include full names, hashes only, or
// nothing in the footnotes. `progress` tells whether a progress meter
// should be displayed while it works.
//
// It returns the size data for the repository.
func ScanRepositoryUsingGraph(
	ctx context.Context,
	repo *git.Repository,
	roots []Root,
	nameStyle NameStyle,
	progressMeter meter.Progress,
) (HistorySize, error) {
	nullOID := repo.HashAlgo().NullOID()
	graph := NewGraph(nameStyle)

	objIter, err := repo.NewObjectIter(ctx)
	if err != nil {
		return HistorySize{}, err
	}

	errChan := make(chan error, 1)
	// Feed the references that we want to walk into the stdin of the
	// object iterator:
	go func() {
		defer objIter.Close()

		errChan <- func() error {
			for _, root := range roots {
				if !root.Walk() {
					continue
				}

				if err := objIter.AddRoot(root.OID()); err != nil {
					return err
				}
			}
			return nil
		}()
	}()

	type ObjectHeader struct {
		oid        git.OID
		objectSize counts.Count32
	}

	type CommitHeader struct {
		ObjectHeader
		tree git.OID
	}

	// We process the blobs right away, but record these other types
	// of objects for later processing. The order of processing
	// strongly affects performance, which prefers object locality and
	// prefers having as few "dangling pointers" as possible. It also
	// affects which of multiple equally-sized objects are chosen and
	// which references the `PathResolver` chooses to refer to
	// commits. Note that we process different types of objects in
	// different orders:
	//
	// * Blobs are processed in roughly reverse-chronological order
	//   This is relatively inconsequential because blobs can't point
	//   at any other objects.
	//
	// * Trees are processed in roughly reverse-chronological order
	//   (the order that they come out of `git rev-parse --date-order
	//   --objects`). This is more efficient than the reverse because
	//   the Git command outputs the whole tree corresponding to a
	//   commit before moving onto the next commit. So when we process
	//   them in this order, we have at most one "treeful" of trees
	//   pending at any given moment (and usually much less); there
	//   are no "dangling pointers" carried over from one commit to
	//   the next. Plus, this allows us to use
	//   `AdjustMaxIfNecessary()`, which leads to less churn in the
	//   `PathResolver`.
	//
	// * Commits are processed in roughly chronological order when
	//   computing sizes and looking for the "biggest" commits. This
	//   is preferable because the opposite order would leave most
	//   commits pending until we worked all the way to the start of
	//   history. But by using `AdjustMaxIfPossible()`, we still
	//   preferentially choose the newest commits.
	//
	//   But when feeding commits to the `PathResolver`, we process
	//   the commits in reverse chronological order. This helps prefer
	//   new commits when naming blobs and trees.
	//
	// * References are processed in alphabetical order. (It might be
	//   a tiny improvement to pick the order more intentionally, to
	//   favor certain references when naming commits that are pointed
	//   to by multiple references, but it doesn't seem worth the
	//   effort.)
	var trees, tags []ObjectHeader
	var commits []CommitHeader

	progressMeter.Start("Processing blobs: %d")
	for {
		obj, ok, err := objIter.Next()
		if err != nil {
			return HistorySize{}, err
		}
		if !ok {
			break
		}
		switch obj.ObjectType {
		case "blob":
			progressMeter.Inc()
			graph.RegisterBlob(obj.OID, obj.ObjectSize)
		case "tree":
			trees = append(trees, ObjectHeader{obj.OID, obj.ObjectSize})
		case "commit":
			commits = append(commits, CommitHeader{ObjectHeader{obj.OID, obj.ObjectSize}, nullOID})
		case "tag":
			tags = append(tags, ObjectHeader{obj.OID, obj.ObjectSize})
		default:
			return HistorySize{}, fmt.Errorf("unexpected object type: %s", obj.ObjectType)
		}
	}
	progressMeter.Done()

	err = <-errChan
	if err != nil {
		return HistorySize{}, err
	}

	objectIter, err := repo.NewBatchObjectIter(ctx)
	if err != nil {
		return HistorySize{}, err
	}

	go func() {
		defer objectIter.Close()

		errChan <- func() error {
			for _, obj := range trees {
				if err := objectIter.RequestObject(obj.oid); err != nil {
					return fmt.Errorf("requesting tree '%s': %w", obj.oid, err)
				}
			}

			for i := len(commits); i > 0; i-- {
				obj := commits[i-1]
				if err := objectIter.RequestObject(obj.oid); err != nil {
					return fmt.Errorf("requesting commit '%s': %w", obj.oid, err)
				}
			}

			for _, obj := range tags {
				if err := objectIter.RequestObject(obj.oid); err != nil {
					return fmt.Errorf("requesting tag '%s': %w", obj.oid, err)
				}
			}

			return nil
		}()
	}()

	progressMeter.Start("Processing trees: %d")
	for range trees {
		obj, ok, err := objectIter.Next()
		if err != nil {
			return HistorySize{}, err
		}
		if !ok {
			return HistorySize{}, errors.New("fewer trees read than expected")
		}
		if obj.ObjectType != "tree" {
			return HistorySize{}, fmt.Errorf("expected tree; read %#v", obj.ObjectType)
		}
		progressMeter.Inc()
		tree, err := git.ParseTree(obj.OID, obj.Data)
		if err != nil {
			return HistorySize{}, err
		}
		err = graph.RegisterTree(obj.OID, tree)
		if err != nil {
			return HistorySize{}, err
		}
	}
	progressMeter.Done()

	// Process the commits in (roughly) chronological order, to
	// minimize the number of commits that are pending at any one
	// time:
	progressMeter.Start("Processing commits: %d")
	for i := len(commits); i > 0; i-- {
		obj, ok, err := objectIter.Next()
		if err != nil {
			return HistorySize{}, err
		}
		if !ok {
			return HistorySize{}, errors.New("fewer commits read than expected")
		}
		if obj.ObjectType != "commit" {
			return HistorySize{}, fmt.Errorf("expected commit; read %#v", obj.ObjectType)
		}
		commit, err := git.ParseCommit(obj.OID, obj.Data)
		if err != nil {
			return HistorySize{}, err
		}
		if obj.OID != commits[i-1].oid {
			panic("commits not read in same order as requested")
		}
		commits[i-1].tree = commit.Tree
		progressMeter.Inc()
		graph.RegisterCommit(obj.OID, commit)
	}
	progressMeter.Done()

	// Tell PathResolver about the commits in (roughly) reverse
	// chronological order, to favor new ones in the paths of trees:
	if nameStyle != NameStyleNone {
		progressMeter.Start("Matching commits to trees: %d")
		for _, commit := range commits {
			progressMeter.Inc()
			graph.pathResolver.RecordCommit(commit.oid, commit.tree)
		}
		progressMeter.Done()
	}

	progressMeter.Start("Processing annotated tags: %d")
	for range tags {
		obj, ok, err := objectIter.Next()
		if err != nil {
			return HistorySize{}, err
		}
		if !ok {
			return HistorySize{}, errors.New("fewer tags read than expected")
		}
		if obj.ObjectType != "tag" {
			return HistorySize{}, fmt.Errorf("expected tag; read %#v", obj.ObjectType)
		}
		tag, err := git.ParseTag(obj.OID, obj.Data)
		if err != nil {
			return HistorySize{}, err
		}
		progressMeter.Inc()
		graph.RegisterTag(obj.OID, tag)
	}
	progressMeter.Done()

	err = <-errChan
	if err != nil {
		return HistorySize{}, err
	}

	progressMeter.Start("Processing references: %d")
	for _, root := range roots {
		progressMeter.Inc()
		if refRoot, ok := root.(ReferenceRoot); ok {
			graph.RegisterReference(refRoot.Reference(), refRoot.Groups())
		}

		if root.Walk() {
			graph.pathResolver.RecordName(root.Name(), root.OID())
		}
	}
	progressMeter.Done()

	return graph.HistorySize(), nil
}

// Graph is an object graph that is being built up.
type Graph struct {
	blobLock  sync.Mutex
	blobSizes map[git.OID]BlobSize

	treeLock    sync.Mutex
	treeRecords map[git.OID]*treeRecord
	treeSizes   map[git.OID]TreeSize

	commitLock  sync.Mutex
	commitSizes map[git.OID]CommitSize

	tagLock    sync.Mutex
	tagRecords map[git.OID]*tagRecord
	tagSizes   map[git.OID]TagSize

	// Statistics about the overall history size:
	historyLock sync.Mutex
	historySize HistorySize

	pathResolver PathResolver
}

// NewGraph creates and returns a new `*Graph` instance.
func NewGraph(nameStyle NameStyle) *Graph {
	return &Graph{
		blobSizes: make(map[git.OID]BlobSize),

		treeRecords: make(map[git.OID]*treeRecord),
		treeSizes:   make(map[git.OID]TreeSize),

		commitSizes: make(map[git.OID]CommitSize),

		tagRecords: make(map[git.OID]*tagRecord),
		tagSizes:   make(map[git.OID]TagSize),

		historySize: HistorySize{
			ReferenceGroups: make(map[RefGroupSymbol]*counts.Count32),
		},

		pathResolver: NewPathResolver(nameStyle),
	}
}

// RegisterReference records the specified reference in `g`.
func (g *Graph) RegisterReference(ref git.Reference, groups []RefGroupSymbol) {
	g.historyLock.Lock()
	g.historySize.recordReference(g, ref)
	for _, group := range groups {
		g.historySize.recordReferenceGroup(g, group)
	}
	g.historyLock.Unlock()
}

// Register a name that can be used for the specified OID.
func (g *Graph) RegisterName(name string, oid git.OID) {
	g.pathResolver.RecordName(name, oid)
}

// HistorySize returns the size data that have been collected.
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

// RegisterBlob records that the specified `oid` is a blob with the
// specified size.
func (g *Graph) RegisterBlob(oid git.OID, objectSize counts.Count32) {
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

func (g *Graph) GetBlobSize(oid git.OID) BlobSize {
	// See if we already know the size:
	size, ok := g.blobSizes[oid]
	if !ok {
		panic("blob size not known")
	}
	return size
}

func (g *Graph) RequireTreeSize(oid git.OID, listener func(TreeSize)) (TreeSize, bool) {
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

func (g *Graph) GetTreeSize(oid git.OID) TreeSize {
	g.treeLock.Lock()

	size, ok := g.treeSizes[oid]
	if !ok {
		panic("tree size not available!")
	}
	g.treeLock.Unlock()
	return size
}

// Record that the specified `oid` is the specified `tree`.
func (g *Graph) RegisterTree(oid git.OID, tree *git.Tree) error {
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
	return record.initialize(g, oid, tree)
}

func (g *Graph) finalizeTreeSize(
	oid git.OID, size TreeSize, objectSize counts.Count32, treeEntries counts.Count32,
) {
	g.treeLock.Lock()
	g.treeSizes[oid] = size
	delete(g.treeRecords, oid)
	g.treeLock.Unlock()

	g.historyLock.Lock()
	g.historySize.recordTree(g, oid, size, objectSize, treeEntries)
	g.historyLock.Unlock()
}

type treeRecord struct {
	oid git.OID

	// Limit to only one mutator at a time.
	lock sync.Mutex

	// The size of this object, in bytes. Initialized iff pending !=
	// -1.
	objectSize counts.Count32

	// The number of entries directly in this tree. Initialized iff
	// pending != -1.
	entryCount counts.Count32

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

func newTreeRecord(oid git.OID) *treeRecord {
	return &treeRecord{
		oid:     oid,
		size:    TreeSize{ExpandedTreeCount: 1},
		pending: -1,
	}
}

// Initialize `r` (which is empty) based on `tree`.
func (r *treeRecord) initialize(g *Graph, oid git.OID, tree *git.Tree) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.objectSize = tree.Size()
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
		case entry.Filemode&0o170000 == 0o40000:
			// Tree
			listener := func(size TreeSize) {
				// This listener is called when the tree pointed to by
				// `entry` has been fully processed.
				r.lock.Lock()
				defer r.lock.Unlock()

				g.pathResolver.RecordTreeEntry(oid, name, entry.OID)

				r.size.addDescendent(name, size)
				r.pending--
				// This might inform *our* listeners that we are now
				// fully processed:
				r.maybeFinalize(g)
			}
			treeSize, ok := g.RequireTreeSize(entry.OID, listener)
			if ok {
				r.size.addDescendent(name, treeSize)
			} else {
				r.pending++
			}
			r.entryCount.Increment(1)

		case entry.Filemode&0o170000 == 0o160000:
			// Commit (i.e., submodule)
			r.size.addSubmodule(name)
			r.entryCount.Increment(1)

		case entry.Filemode&0o170000 == 0o120000:
			// Symlink
			g.pathResolver.RecordTreeEntry(oid, name, entry.OID)

			r.size.addLink(name)
			r.entryCount.Increment(1)

		default:
			// Blob
			g.pathResolver.RecordTreeEntry(oid, name, entry.OID)

			blobSize := g.GetBlobSize(entry.OID)
			r.size.addBlob(name, blobSize)
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

func (g *Graph) GetCommitSize(oid git.OID) CommitSize {
	g.commitLock.Lock()

	size, ok := g.commitSizes[oid]
	if !ok {
		panic("commit is not available")
	}
	g.commitLock.Unlock()

	return size
}

// Record that the specified `oid` is the specified `commit`.
func (g *Graph) RegisterCommit(oid git.OID, commit *git.Commit) {
	g.commitLock.Lock()
	if _, ok := g.commitSizes[oid]; ok {
		panic(fmt.Sprintf("commit %s registered twice!", oid))
	}
	g.commitLock.Unlock()

	// The number of direct parents of this commit.
	parentCount := counts.NewCount32(uint64(len(commit.Parents)))

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

func (g *Graph) RequireTagSize(oid git.OID, listener func(TagSize)) (TagSize, bool) {
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
func (g *Graph) RegisterTag(oid git.OID, tag *git.Tag) {
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
	record.initialize(g, oid, tag)
}

func (g *Graph) finalizeTagSize(oid git.OID, size TagSize, objectSize counts.Count32) {
	g.tagLock.Lock()
	g.tagSizes[oid] = size
	delete(g.tagRecords, oid)
	g.tagLock.Unlock()

	g.historyLock.Lock()
	g.historySize.recordTag(g, oid, size, objectSize)
	g.historyLock.Unlock()
}

type tagRecord struct {
	oid git.OID

	// Limit to only one mutator at a time.
	lock sync.Mutex

	// The size of this commit object in bytes.
	objectSize counts.Count32

	// The size of the items we know so far:
	size TagSize

	// See treeRecord.pending. This value can be at most 1 for a Tag.
	pending int8

	// See treeRecord.listeners.
	listeners []func(TagSize)
}

func newTagRecord(oid git.OID) *tagRecord {
	return &tagRecord{
		oid:     oid,
		pending: -1,
	}
}

// Initialize `r` (which is empty) based on `tag`.
func (r *tagRecord) initialize(g *Graph, oid git.OID, tag *git.Tag) {
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

	r.maybeFinalize(g)
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
