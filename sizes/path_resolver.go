package sizes

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/github/git-sizer/git"
)

// PathResolver figures out a "reachability path" (i.e., Git
// `rev-parse` input, including commit and/or file path) by which
// specified objects are reachable. It is used as follows:
//
// * Request an object's path using `RequestPath()`. The returned
//   `Path` object is a placeholder for the object's path.
//
// * Tell the `PathResolver` about objects that might be along the
//   object's reachability path, *in depth-first* order (i.e.,
//   referents before referers) by calling `RecordTree()`,
//   `RecordCommit()`, `RecordTag()`, and `RecordReference()`,.
//
// * Read the path out of the `Path` object using `Path.Path()`.
//
// Multiple objects can be processed at once.
//
// It is important that interested is registered in an object's path
// via `RequestPath()` *before* any of the objects along its
// reachability path are recorded.
//
// If a caller decides that it is not interested in a path after all,
// it can call `ForgetPath()`. This might free up some resources that
// would otherwise continue consuming memory.
type PathResolver interface {
	RequestPath(oid git.OID, objectType string) *Path
	ForgetPath(p *Path)
	RecordReference(ref git.Reference)
	RecordTreeEntry(oid git.OID, name string, childOID git.OID)
	RecordCommit(oid, tree git.OID)
	RecordTag(oid git.OID, tag *git.Tag)
}

type NullPathResolver struct {
	useHash bool
}

func (n NullPathResolver) RequestPath(oid git.OID, objectType string) *Path {
	// The caller is the only one retaining a reference to this
	// object. When it loses interest, the object will be GCed,
	// without our having to do anything to manage its lifetime.
	if n.useHash {
		return &Path{
			OID:        oid,
			objectType: objectType,
		}
	} else {
		return nil
	}
}

func (_ NullPathResolver) ForgetPath(p *Path) {}

func (_ NullPathResolver) RecordReference(ref git.Reference) {}

func (_ NullPathResolver) RecordTreeEntry(oid git.OID, name string, childOID git.OID) {}

func (_ NullPathResolver) RecordCommit(oid, tree git.OID) {}

func (_ NullPathResolver) RecordTag(oid git.OID, tag *git.Tag) {}

type InOrderPathResolver struct {
	lock        sync.Mutex
	soughtPaths map[git.OID]*Path
}

// Structure for keeping track of an object whose path we want to know
// (e.g., the biggest blob, or a tree containing the biggest blob, or
// a commit whose tree contains the biggest blob). Valid states:
//
// * `parent == nil && relativePath == ""`—we have not yet found
//   anything that refers to this object.
//
// * `parent != nil && relativePath == ""`—this object is a tree, and
//   we have found a commit that refers to it.
//
// * `parent == nil && relativePath != ""`—we have found a reference
//   that points directly at this object; `relativePath` is the full
//   name of the reference.
//
// * `parent != nil && relativePath != ""`—this object is a blob or
//   tree, and we have found another tree that refers to it;
//   `relativePath` is the corresponding tree entry name.
type Path struct {
	// The OID of the object whose path we seek. This member is always
	// set.
	git.OID

	// The type of the object whose path we seek. This member is
	// always set.
	objectType string

	// The number of seekers that want this object's path, including 1
	// for the caller of `RequestPath()` (i.e., it is initialized to
	// 1). If this value goes to zero, we can remove the object from
	// the PathResolver.
	seekerCount uint8

	// A path we found of a parent from which this object is
	// referenced. This is set when we find a parent then never
	// changed again. It is never set if the "parent" we find is a
	// reference.
	parent *Path

	// The relative path from the parent's path to this object; i.e.,
	// what has to be appended to the parent path to create the path
	// to this object.
	relativePath string
}

// Return the path of this object under the assumption that another
// path component will be appended to it.
func (p *Path) TreePrefix() string {
	switch p.objectType {
	case "blob", "tree":
		if p.parent != nil {
			if p.relativePath == "" {
				// This is a top-level tree or blob.
				return p.parent.TreePrefix()
			} else {
				// The parent is also a tree.
				return p.parent.TreePrefix() + p.relativePath + "/"
			}
		} else {
			return "???"
		}
	case "commit", "tag":
		if p.parent != nil {
			// The parent is a tag.
			return fmt.Sprintf("%s^{%s}", p.parent.BestPath(), p.objectType)
		} else if p.relativePath != "" {
			return p.relativePath + ":"
		} else {
			return p.OID.String() + ":"
		}
	default:
		return "???"
	}
}

// Return a human-readable path for this object if we can do better
// than its OID; otherwise, return "".
func (p *Path) Path() string {
	switch p.objectType {
	case "blob", "tree":
		if p.parent != nil {
			if p.relativePath == "" {
				// This is a top-level tree or blob.
				return fmt.Sprintf("%s^{%s}", p.parent.BestPath(), p.objectType)
			} else {
				// The parent is also a tree.
				return p.parent.TreePrefix() + p.relativePath
			}
		} else {
			return ""
		}
	case "commit", "tag":
		if p.parent != nil {
			// The parent is a tag.
			return fmt.Sprintf("%s^{%s}", p.parent.BestPath(), p.objectType)
		} else if p.relativePath != "" {
			return p.relativePath
		} else {
			return ""
		}
	default:
		return ""
	}
}

// Return some human-readable path for this object, even if it's just
// the OID.
func (p *Path) BestPath() string {
	path := p.Path()
	if path != "" {
		return path
	}

	return p.OID.String()
}

func (p *Path) String() string {
	path := p.Path()
	if path == "" {
		return p.OID.String()
	} else {
		return fmt.Sprintf("%s (%s)", p.OID, path)
	}
}

func (p *Path) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

func NewPathResolver(nameStyle NameStyle) PathResolver {
	switch nameStyle {
	case NameStyleNone:
		return NullPathResolver{false}
	case NameStyleHash:
		return NullPathResolver{true}
	case NameStyleFull:
		return &InOrderPathResolver{
			soughtPaths: make(map[git.OID]*Path),
		}
	default:
		panic("Unexpected NameStyle value")
	}
}

// Request that a path to the object named `oid` be computed.
func (pr *InOrderPathResolver) RequestPath(oid git.OID, objectType string) *Path {
	pr.lock.Lock()
	defer pr.lock.Unlock()
	return pr.requestPathLocked(oid, objectType)
}

// Request that a path to the object named `oid` be computed.
func (pr *InOrderPathResolver) requestPathLocked(oid git.OID, objectType string) *Path {
	p, ok := pr.soughtPaths[oid]
	if ok {
		p.seekerCount++
		return p
	}

	p = &Path{
		OID:         oid,
		objectType:  objectType,
		seekerCount: 1,
	}
	pr.soughtPaths[oid] = p
	return p
}

// Record that the specified path is wanted by one less seeker. If its
// seeker count goes to zero, remove it from `pr.soughtPaths`.
func (pr *InOrderPathResolver) ForgetPath(p *Path) {
	pr.lock.Lock()
	defer pr.lock.Unlock()

	pr.forgetPathLocked(p)
}

func (pr *InOrderPathResolver) forgetPathLocked(p *Path) {
	if p.seekerCount == 0 {
		panic("forgetPathLocked() called when refcount zero")
	}
	p.seekerCount--
	if p.seekerCount > 0 {
		// The path is still wanted (by another seeker).
		return
	} else if p.parent != nil {
		// We already found the object's parent, and the parent's path
		// is wanted on account if this object. Decrement its
		// seekerCount.
		pr.forgetPathLocked(p.parent)
	} else if p.relativePath == "" {
		// We were still looking for this object's parent. Stop doing
		// so.
		delete(pr.soughtPaths, p.OID)
	}
}

func (pr *InOrderPathResolver) RecordReference(ref git.Reference) {
	pr.lock.Lock()
	defer pr.lock.Unlock()

	p, ok := pr.soughtPaths[ref.OID]
	if !ok {
		// Nobody is looking for the path to the referent.
		return
	}

	p.relativePath = ref.Refname
	delete(pr.soughtPaths, ref.OID)
}

// Record that the tree with OID `oid` has an entry with the specified
// `name` and `childOID`.
func (pr *InOrderPathResolver) RecordTreeEntry(oid git.OID, name string, childOID git.OID) {
	pr.lock.Lock()
	defer pr.lock.Unlock()

	p, ok := pr.soughtPaths[childOID]
	if !ok {
		// Nobody is looking for the path to the child.
		return
	}

	if p.parent != nil {
		panic("tree path parent unexpectedly filled in")
	}
	p.parent = pr.requestPathLocked(oid, "tree")

	p.relativePath = name

	// We don't need to keep looking for the child anymore:
	delete(pr.soughtPaths, childOID)
}

func (pr *InOrderPathResolver) RecordCommit(oid, tree git.OID) {
	pr.lock.Lock()
	defer pr.lock.Unlock()

	p, ok := pr.soughtPaths[tree]
	if !ok {
		// Nobody is looking for the path to our tree.
		return
	}

	if p.parent != nil {
		panic("commit tree parent unexpectedly filled in")
	}
	p.parent = pr.requestPathLocked(oid, "commit")

	p.relativePath = ""

	// We don't need to keep looking for the child anymore:
	delete(pr.soughtPaths, tree)
}

func (pr *InOrderPathResolver) RecordTag(oid git.OID, tag *git.Tag) {
	// Not implemented.
}
