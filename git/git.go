package git

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/github/git-sizer/counts"
)

// ObjectType represents the type of a Git object ("blob", "tree",
// "commit", "tag", or "missing").
type ObjectType string

// OID represents the SHA-1 object ID of a Git object, in binary
// format.
type OID struct {
	v [20]byte
}

// NullOID is the null object ID; i.e., all zeros.
var NullOID OID

// OIDFromBytes converts a byte slice containing an object ID in
// binary format into an `OID`.
func OIDFromBytes(oidBytes []byte) (OID, error) {
	var oid OID
	if len(oidBytes) != len(oid.v) {
		return OID{}, errors.New("bytes oid has the wrong length")
	}
	copy(oid.v[0:20], oidBytes)
	return oid, nil
}

// NewOID converts an object ID in hex format (i.e., `[0-9a-f]{40}`)
// into an `OID`.
func NewOID(s string) (OID, error) {
	oidBytes, err := hex.DecodeString(s)
	if err != nil {
		return OID{}, err
	}
	return OIDFromBytes(oidBytes)
}

// String formats `oid` as a string in hex format.
func (oid OID) String() string {
	return hex.EncodeToString(oid.v[:])
}

// Bytes returns a byte slice view of `oid`, in binary format.
func (oid OID) Bytes() []byte {
	return oid.v[:]
}

// MarshalJSON expresses `oid` as a JSON string with its enclosing
// quotation marks.
func (oid OID) MarshalJSON() ([]byte, error) {
	src := oid.v[:]
	dst := make([]byte, hex.EncodedLen(len(src))+2)
	dst[0] = '"'
	dst[len(dst)-1] = '"'
	hex.Encode(dst[1:len(dst)-1], src)
	return dst, nil
}

// Repository represents a Git repository on disk.
type Repository struct {
	path string

	// gitBin is the path of the `git` executable that should be used
	// when running commands in this repository.
	gitBin string
}

// smartJoin returns the path that can be described as `relPath`
// relative to `path`, given that `path` is either absolute or is
// relative to the current directory.
func smartJoin(path, relPath string) string {
	if filepath.IsAbs(relPath) {
		return relPath
	}
	return filepath.Join(path, relPath)
}

// NewRepository creates a new repository object that can be used for
// running `git` commands within that repository.
func NewRepository(path string) (*Repository, error) {
	// Find the `git` executable to be used:
	gitBin, err := findGitBin()
	if err != nil {
		return nil, fmt.Errorf(
			"could not find 'git' executable (is it in your PATH?): %w", err,
		)
	}

	cmd := exec.Command(gitBin, "-C", path, "rev-parse", "--git-dir")
	out, err := cmd.Output()
	if err != nil {
		switch err := err.(type) {
		case *exec.Error:
			return nil, fmt.Errorf(
				"could not run '%s': %w", gitBin, err.Err,
			)
		case *exec.ExitError:
			return nil, fmt.Errorf(
				"git rev-parse failed: %s", err.Stderr,
			)
		default:
			return nil, err
		}
	}
	gitDir := smartJoin(path, string(bytes.TrimSpace(out)))

	cmd = exec.Command(gitBin, "rev-parse", "--git-path", "shallow")
	cmd.Dir = gitDir
	out, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf(
			"could not run 'git rev-parse --git-path shallow': %w", err,
		)
	}
	shallow := smartJoin(gitDir, string(bytes.TrimSpace(out)))
	_, err = os.Lstat(shallow)
	if err == nil {
		return nil, errors.New("this appears to be a shallow clone; full clone required")
	}

	return &Repository{
		path:   gitDir,
		gitBin: gitBin,
	}, nil
}

func (repo *Repository) gitCommand(callerArgs ...string) *exec.Cmd {
	args := []string{
		// Disable replace references when running our commands:
		"--no-replace-objects",

		// Disable the warning that grafts are deprecated, since we
		// want to set the grafts file to `/dev/null` below (to
		// disable grafts even where they are supported):
		"-c", "advice.graftFileDeprecated=false",
	}

	args = append(args, callerArgs...)

	cmd := exec.Command(repo.gitBin, args...)

	cmd.Env = append(
		os.Environ(),
		"GIT_DIR="+repo.path,
		// Disable grafts when running our commands:
		"GIT_GRAFT_FILE="+os.DevNull,
	)

	return cmd
}

// Path returns the path to `repo`.
func (repo *Repository) Path() string {
	return repo.path
}

// Reference represents a Git reference.
type Reference struct {
	// Refname is the full reference name of the reference.
	Refname string

	// ObjectType is the type of the object referenced.
	ObjectType ObjectType

	// ObjectSize is the size of the referred-to object, in bytes.
	ObjectSize counts.Count32

	// OID is the OID of the referred-to object.
	OID OID
}

// ReferenceIter is an iterator that interates over references.
type ReferenceIter struct {
	cmd     *exec.Cmd
	out     io.ReadCloser
	f       *bufio.Reader
	errChan <-chan error
}

// NewReferenceIter returns an iterator that iterates over all of the
// references in `repo`.
func (repo *Repository) NewReferenceIter() (*ReferenceIter, error) {
	cmd := repo.gitCommand(
		"for-each-ref", "--format=%(objectname) %(objecttype) %(objectsize) %(refname)",
	)

	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return &ReferenceIter{
		cmd:     cmd,
		out:     out,
		f:       bufio.NewReader(out),
		errChan: make(chan error, 1),
	}, nil
}

// Next returns either the next reference or a boolean `false` value
// indicating that the iteration is over. On errors, return an error
// (in this case, the caller must still call `Close()`).
func (iter *ReferenceIter) Next() (Reference, bool, error) {
	line, err := iter.f.ReadString('\n')
	if err != nil {
		if err != io.EOF {
			return Reference{}, false, err
		}
		return Reference{}, false, nil
	}
	line = line[:len(line)-1]
	words := strings.Split(line, " ")
	if len(words) != 4 {
		return Reference{}, false, fmt.Errorf("line improperly formatted: %#v", line)
	}
	oid, err := NewOID(words[0])
	if err != nil {
		return Reference{}, false, fmt.Errorf("SHA-1 improperly formatted: %#v", words[0])
	}
	objectType := ObjectType(words[1])
	objectSize, err := strconv.ParseUint(words[2], 10, 32)
	if err != nil {
		return Reference{}, false, fmt.Errorf("object size improperly formatted: %#v", words[2])
	}
	refname := words[3]
	return Reference{
		Refname:    refname,
		ObjectType: objectType,
		ObjectSize: counts.Count32(objectSize),
		OID:        oid,
	}, true, nil
}

// Close closes the iterator and frees up resources.
func (iter *ReferenceIter) Close() error {
	err := iter.out.Close()
	err2 := iter.cmd.Wait()
	if err == nil {
		err = err2
	}
	return err
}

// BatchObjectIter iterates over objects whose names are fed into its
// stdin. The output is buffered, so it has to be closed before you
// can be sure that you have gotten all of the objects.
type BatchObjectIter struct {
	cmd *exec.Cmd
	out io.ReadCloser
	f   *bufio.Reader
}

// NewBatchObjectIter returns a `*BatchObjectIterator` and an
// `io.WriteCloser`. The iterator iterates over objects whose names
// are fed into the `io.WriteCloser`, one per line. The
// `io.WriteCloser` should normally be closed and the iterator's
// output drained before `Close()` is called.
func (repo *Repository) NewBatchObjectIter() (*BatchObjectIter, io.WriteCloser, error) {
	cmd := repo.gitCommand("cat-file", "--batch", "--buffer")

	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return nil, nil, err
	}

	return &BatchObjectIter{
		cmd: cmd,
		out: out,
		f:   bufio.NewReader(out),
	}, in, nil
}

// Next returns the next object: its OID, type, size, and contents.
// When no more data are available, it returns an `io.EOF` error.
func (iter *BatchObjectIter) Next() (OID, ObjectType, counts.Count32, []byte, error) {
	header, err := iter.f.ReadString('\n')
	if err != nil {
		return OID{}, "", 0, nil, err
	}
	oid, objectType, objectSize, err := parseBatchHeader("", header)
	if err != nil {
		return OID{}, "", 0, nil, err
	}
	// +1 for LF:
	data := make([]byte, objectSize+1)
	_, err = io.ReadFull(iter.f, data)
	if err != nil {
		return OID{}, "", 0, nil, err
	}
	data = data[:len(data)-1]
	return oid, objectType, objectSize, data, nil
}

// Close closes the iterator and frees up resources. If any iterator
// output hasn't been read yet, it will be lost.
func (iter *BatchObjectIter) Close() error {
	err := iter.out.Close()
	err2 := iter.cmd.Wait()
	if err == nil {
		err = err2
	}
	return err
}

// Parse a `cat-file --batch[-check]` output header line (including
// the trailing LF). `spec`, if not "", is used in error messages.
func parseBatchHeader(spec string, header string) (OID, ObjectType, counts.Count32, error) {
	header = header[:len(header)-1]
	words := strings.Split(header, " ")
	if words[len(words)-1] == "missing" {
		if spec == "" {
			spec = words[0]
		}
		return OID{}, "missing", 0, fmt.Errorf("missing object %s", spec)
	}

	oid, err := NewOID(words[0])
	if err != nil {
		return OID{}, "missing", 0, err
	}

	size, err := strconv.ParseUint(words[2], 10, 0)
	if err != nil {
		return OID{}, "missing", 0, err
	}
	return oid, ObjectType(words[1]), counts.NewCount32(size), nil
}

// ObjectIter iterates over objects in a Git repository.
type ObjectIter struct {
	cmd1    *exec.Cmd
	cmd2    *exec.Cmd
	out1    io.ReadCloser
	out2    io.ReadCloser
	f       *bufio.Reader
	errChan <-chan error
}

// NewObjectIter returns an iterator that iterates over objects in
// `repo`. The arguments are passed to `git rev-list --objects`. The
// second return value is the stdin of the `rev-list` command. The
// caller can feed values into it but must close it in any case.
func (repo *Repository) NewObjectIter(
	args ...string,
) (*ObjectIter, io.WriteCloser, error) {
	cmd1 := repo.gitCommand(append([]string{"rev-list", "--objects"}, args...)...)
	in1, err := cmd1.StdinPipe()
	if err != nil {
		return nil, nil, err
	}

	out1, err := cmd1.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	cmd1.Stderr = os.Stderr

	err = cmd1.Start()
	if err != nil {
		return nil, nil, err
	}

	cmd2 := repo.gitCommand("cat-file", "--batch-check", "--buffer")
	in2, err := cmd2.StdinPipe()
	if err != nil {
		out1.Close()
		cmd1.Wait()
		return nil, nil, err
	}

	out2, err := cmd2.StdoutPipe()
	if err != nil {
		in2.Close()
		out1.Close()
		cmd1.Wait()
		return nil, nil, err
	}

	cmd2.Stderr = os.Stderr

	err = cmd2.Start()
	if err != nil {
		return nil, nil, err
	}

	errChan := make(chan error, 1)

	go func() {
		defer in2.Close()
		f1 := bufio.NewReader(out1)
		f2 := bufio.NewWriter(in2)
		defer f2.Flush()
		for {
			line, err := f1.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					errChan <- err
				} else {
					errChan <- nil
				}
				return
			}
			if len(line) <= 40 {
				errChan <- fmt.Errorf("line too short: %#v", line)
			}
			f2.WriteString(line[:40])
			f2.WriteByte('\n')
		}
	}()

	return &ObjectIter{
		cmd1:    cmd1,
		cmd2:    cmd2,
		out1:    out1,
		out2:    out2,
		f:       bufio.NewReader(out2),
		errChan: errChan,
	}, in1, nil
}

// Next returns the next object: its OID, type, and size. When no more
// data are available, it returns an `io.EOF` error.
func (iter *ObjectIter) Next() (OID, ObjectType, counts.Count32, error) {
	line, err := iter.f.ReadString('\n')
	if err != nil {
		return OID{}, "", 0, err
	}

	return parseBatchHeader("", line)
}

// Close closes the iterator and frees up resources.
func (iter *ObjectIter) Close() error {
	iter.out1.Close()
	err := <-iter.errChan
	iter.out2.Close()
	err2 := iter.cmd1.Wait()
	if err == nil {
		err = err2
	}
	err2 = iter.cmd2.Wait()
	if err == nil {
		err = err2
	}
	return err
}

// ObjectHeaderIter iterates over the headers within a commit or tag
// object.
type ObjectHeaderIter struct {
	name string
	data string
}

// NewObjectHeaderIter returns an `ObjectHeaderIter` that iterates
// over the headers in a commit or tag object. `data` should be the
// object's contents, which is usually terminated by a blank line that
// separates the header from the comment. However, annotated tags
// don't always include comments, and Git even tolerates commits
// without comments, so don't insist on a blank line. `name` is used
// in error messages.
func NewObjectHeaderIter(name string, data []byte) (ObjectHeaderIter, error) {
	headerEnd := bytes.Index(data, []byte("\n\n"))
	if headerEnd == -1 {
		if len(data) == 0 {
			return ObjectHeaderIter{}, fmt.Errorf("%s has zero length", name)
		}

		if data[len(data)-1] != '\n' {
			return ObjectHeaderIter{}, fmt.Errorf("%s has no terminating LF", name)
		}

		return ObjectHeaderIter{name, string(data)}, nil
	}
	return ObjectHeaderIter{name, string(data[:headerEnd+1])}, nil
}

// HasNext returns true iff there are more headers to retrieve.
func (iter *ObjectHeaderIter) HasNext() bool {
	return len(iter.data) > 0
}

// Next returns the key and value of the next header.
func (iter *ObjectHeaderIter) Next() (string, string, error) {
	if len(iter.data) == 0 {
		return "", "", fmt.Errorf("header for %s read past end", iter.name)
	}
	header := iter.data
	keyEnd := strings.IndexByte(header, ' ')
	if keyEnd == -1 {
		return "", "", fmt.Errorf("malformed header in %s", iter.name)
	}
	key := header[:keyEnd]
	header = header[keyEnd+1:]
	valueEnd := strings.IndexByte(header, '\n')
	if valueEnd == -1 {
		return "", "", fmt.Errorf("malformed header in %s", iter.name)
	}
	value := header[:valueEnd]
	iter.data = header[valueEnd+1:]
	return key, value, nil
}

// Commit represents the parts of a commit object that we need.
type Commit struct {
	Size    counts.Count32
	Parents []OID
	Tree    OID
}

// ParseCommit parses the commit object whose contents are in `data`.
// `oid` is used only in error messages.
func ParseCommit(oid OID, data []byte) (*Commit, error) {
	var parents []OID
	var tree OID
	var treeFound bool
	iter, err := NewObjectHeaderIter(oid.String(), data)
	if err != nil {
		return nil, err
	}
	for iter.HasNext() {
		key, value, err := iter.Next()
		if err != nil {
			return nil, err
		}
		switch key {
		case "parent":
			parent, err := NewOID(value)
			if err != nil {
				return nil, fmt.Errorf("malformed parent header in commit %s", oid)
			}
			parents = append(parents, parent)
		case "tree":
			if treeFound {
				return nil, fmt.Errorf("multiple trees found in commit %s", oid)
			}
			tree, err = NewOID(value)
			if err != nil {
				return nil, fmt.Errorf("malformed tree header in commit %s", oid)
			}
			treeFound = true
		}
	}
	if !treeFound {
		return nil, fmt.Errorf("no tree found in commit %s", oid)
	}
	return &Commit{
		Size:    counts.NewCount32(uint64(len(data))),
		Parents: parents,
		Tree:    tree,
	}, nil
}

// Tree represents a Git tree object.
type Tree struct {
	data string
}

// ParseTree parses the tree object whose contents are contained in
// `data`. `oid` is currently unused.
func ParseTree(oid OID, data []byte) (*Tree, error) {
	return &Tree{string(data)}, nil
}

// Size returns the size of the tree object.
func (tree Tree) Size() counts.Count32 {
	return counts.NewCount32(uint64(len(tree.data)))
}

// TreeEntry represents an entry in a Git tree object. Note that Name
// shares memory with the tree data that were originally read; i.e.,
// retaining a pointer to Name keeps the tree data reachable.
type TreeEntry struct {
	Name     string
	OID      OID
	Filemode uint
}

// TreeIter is an iterator over the entries in a Git tree object.
type TreeIter struct {
	// The as-yet-unread part of the tree's data.
	data string
}

// Iter returns an iterator over the entries in `tree`.
func (tree *Tree) Iter() *TreeIter {
	return &TreeIter{
		data: tree.data,
	}
}

// NextEntry returns either the next entry in a Git tree, or a `false`
// boolean value if there are no more entries.
func (iter *TreeIter) NextEntry() (TreeEntry, bool, error) {
	var entry TreeEntry

	if len(iter.data) == 0 {
		return TreeEntry{}, false, nil
	}

	spAt := strings.IndexByte(iter.data, ' ')
	if spAt < 0 {
		return TreeEntry{}, false, errors.New("failed to find SP after mode")
	}
	mode, err := strconv.ParseUint(iter.data[:spAt], 8, 32)
	if err != nil {
		return TreeEntry{}, false, err
	}
	entry.Filemode = uint(mode)

	iter.data = iter.data[spAt+1:]
	nulAt := strings.IndexByte(iter.data, 0)
	if nulAt < 0 {
		return TreeEntry{}, false, errors.New("failed to find NUL after filename")
	}

	entry.Name = iter.data[:nulAt]

	iter.data = iter.data[nulAt+1:]
	if len(iter.data) < 20 {
		return TreeEntry{}, false, errors.New("tree entry ends unexpectedly")
	}

	copy(entry.OID.v[0:20], iter.data[0:20])
	iter.data = iter.data[20:]

	return entry, true, nil
}

// Tag represents the information that we need about a Git tag object.
type Tag struct {
	Size         counts.Count32
	Referent     OID
	ReferentType ObjectType
}

// ParseTag parses the Git tag object whose contents are contained in
// `data`. `oid` is used only in error messages.
func ParseTag(oid OID, data []byte) (*Tag, error) {
	var referent OID
	var referentFound bool
	var referentType ObjectType
	var referentTypeFound bool
	iter, err := NewObjectHeaderIter(oid.String(), data)
	if err != nil {
		return nil, err
	}
	for iter.HasNext() {
		key, value, err := iter.Next()
		if err != nil {
			return nil, err
		}
		switch key {
		case "object":
			if referentFound {
				return nil, fmt.Errorf("multiple referents found in tag %s", oid)
			}
			referent, err = NewOID(value)
			if err != nil {
				return nil, fmt.Errorf("malformed object header in tag %s", oid)
			}
			referentFound = true
		case "type":
			if referentTypeFound {
				return nil, fmt.Errorf("multiple types found in tag %s", oid)
			}
			referentType = ObjectType(value)
			referentTypeFound = true
		}
	}
	if !referentFound {
		return nil, fmt.Errorf("no object found in tag %s", oid)
	}
	if !referentTypeFound {
		return nil, fmt.Errorf("no type found in tag %s", oid)
	}
	return &Tag{
		Size:         counts.NewCount32(uint64(len(data))),
		Referent:     referent,
		ReferentType: referentType,
	}, nil
}
