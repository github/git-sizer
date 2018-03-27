package git

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/github/git-sizer/counts"
)

// The type of an object ("blob", "tree", "commit", "tag", "missing").
type ObjectType string

type OID struct {
	v [20]byte
}

var NullOID OID

func OIDFromBytes(oidBytes []byte) (OID, error) {
	var oid OID
	if len(oidBytes) != len(oid.v) {
		return OID{}, errors.New("bytes oid has the wrong length")
	}
	copy(oid.v[0:20], oidBytes)
	return oid, nil
}

func NewOID(s string) (OID, error) {
	oidBytes, err := hex.DecodeString(s)
	if err != nil {
		return OID{}, err
	}
	return OIDFromBytes(oidBytes)
}

func (oid OID) String() string {
	return hex.EncodeToString(oid.v[:])
}

func (oid OID) Bytes() []byte {
	return oid.v[:]
}

func (oid OID) MarshalJSON() ([]byte, error) {
	src := oid.v[:]
	dst := make([]byte, hex.EncodedLen(len(src))+2)
	dst[0] = '"'
	dst[len(dst)-1] = '"'
	hex.Encode(dst[1:len(dst)-1], src)
	return dst, nil
}

type Repository struct {
	path string
}

// smartJoin returns the path that can be described as `relPath`
// relative to `path`, given that `path` is either absolute or is
// relative to the current directory.
func smartJoin(path, relPath string) string {
	if filepath.IsAbs(relPath) {
		return relPath
	} else {
		return filepath.Join(path, relPath)
	}
}

func NewRepository(path string) (*Repository, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	out, err := cmd.Output()
	if err != nil {
		switch err := err.(type) {
		case *exec.Error:
			return nil, errors.New(
				fmt.Sprintf(
					"could not run git (is it in your PATH?): %s",
					err.Err,
				),
			)
		case *exec.ExitError:
			return nil, errors.New(
				fmt.Sprintf(
					"git rev-parse failed: %s",
					err.Stderr,
				),
			)
		default:
			return nil, err
		}
	}
	gitDir := smartJoin(path, string(bytes.TrimSpace(out)))

	cmd = exec.Command("git", "rev-parse", "--git-path", "shallow")
	cmd.Dir = gitDir
	out, err = cmd.Output()
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf(
				"could not run 'git rev-parse --git-path shallow': %s", err,
			),
		)
	}
	shallow := smartJoin(gitDir, string(bytes.TrimSpace(out)))
	_, err = os.Lstat(shallow)
	if err == nil {
		return nil, errors.New("this appears to be a shallow clone; full clone required")
	}

	return &Repository{path: gitDir}, nil
}

func (repo *Repository) gitCommand(callerArgs ...string) *exec.Cmd {
	// Disable replace references when running our commands:
	args := []string{"--no-replace-objects"}

	args = append(args, callerArgs...)

	cmd := exec.Command("git", args...)

	cmd.Env = append(
		os.Environ(),
		"GIT_DIR="+repo.path,
		// Disable grafts when running our commands:
		"GIT_GRAFT_FILE="+os.DevNull,
	)

	return cmd
}

func (repo *Repository) Path() string {
	return repo.path
}

func (repo *Repository) Close() error {
	return nil
}

type Reference struct {
	Refname    string
	ObjectType ObjectType
	ObjectSize counts.Count32
	OID        OID
}

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

func (l *ReferenceIter) Close() error {
	err := l.out.Close()
	err2 := l.cmd.Wait()
	if err == nil {
		err = err2
	}
	return err
}

type BatchObjectIter struct {
	cmd *exec.Cmd
	out io.ReadCloser
	f   *bufio.Reader
}

// NewBatchObjectIter returns iterates over objects whose names are
// fed into its stdin. The output is buffered, so it has to be closed
// before you can be sure to read all of the objects.
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

func (l *BatchObjectIter) Close() error {
	err := l.out.Close()
	err2 := l.cmd.Wait()
	if err == nil {
		err = err2
	}
	return err
}

type ReferenceFilter func(Reference) bool

func AllReferencesFilter(_ Reference) bool {
	return true
}

func PrefixFilter(prefix string) ReferenceFilter {
	return func(r Reference) bool {
		return strings.HasPrefix(r.Refname, prefix)
	}
}

var (
	BranchesFilter ReferenceFilter
	TagsFilter     ReferenceFilter
	RemotesFilter  ReferenceFilter
)

func init() {
	BranchesFilter = PrefixFilter("refs/heads/")
	TagsFilter = PrefixFilter("refs/tags/")
	RemotesFilter = PrefixFilter("refs/remotes/")
}

func notNilFilters(filters ...ReferenceFilter) []ReferenceFilter {
	var ret []ReferenceFilter
	for _, filter := range filters {
		if filter != nil {
			ret = append(ret, filter)
		}
	}
	return ret
}

func OrFilter(filters ...ReferenceFilter) ReferenceFilter {
	filters = notNilFilters(filters...)
	if len(filters) == 0 {
		return AllReferencesFilter
	} else if len(filters) == 1 {
		return filters[0]
	} else {
		return func(r Reference) bool {
			for _, filter := range filters {
				if filter(r) {
					return true
				}
			}
			return false
		}
	}
}

func AndFilter(filters ...ReferenceFilter) ReferenceFilter {
	filters = notNilFilters(filters...)
	if len(filters) == 0 {
		return AllReferencesFilter
	} else if len(filters) == 1 {
		return filters[0]
	} else {
		return func(r Reference) bool {
			for _, filter := range filters {
				if !filter(r) {
					return false
				}
			}
			return true
		}
	}
}

func NotFilter(filter ReferenceFilter) ReferenceFilter {
	return func(r Reference) bool {
		return !filter(r)
	}
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
		return OID{}, "missing", 0, errors.New(fmt.Sprintf("missing object %s", spec))
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

type ObjectIter struct {
	cmd1    *exec.Cmd
	cmd2    *exec.Cmd
	in1     io.Writer
	out1    io.ReadCloser
	out2    io.ReadCloser
	f       *bufio.Reader
	errChan <-chan error
}

// NewObjectIter returns an iterator that iterates over objects in
// `repo`. The second return value is the stdin of the `rev-list`
// command. The caller can feed values into it but must close it in
// any case.
func (repo *Repository) NewObjectIter(args ...string) (
	*ObjectIter, io.WriteCloser, error,
) {
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

// CreateObject creates a new Git object, of the specified type, in
// `Repository`. `writer` is a function that writes the object in `git
// hash-object` input format. This is used for testing only.
func (repo *Repository) CreateObject(t ObjectType, writer func(io.Writer) error) (OID, error) {
	cmd := repo.gitCommand("hash-object", "-w", "-t", string(t), "--stdin")
	in, err := cmd.StdinPipe()
	if err != nil {
		return OID{}, err
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		return OID{}, err
	}

	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return OID{}, err
	}

	err = writer(in)
	err2 := in.Close()
	if err != nil {
		cmd.Wait()
		return OID{}, err
	}
	if err2 != nil {
		cmd.Wait()
		return OID{}, err2
	}

	output, err := ioutil.ReadAll(out)
	err2 = cmd.Wait()
	if err != nil {
		return OID{}, err
	}
	if err2 != nil {
		return OID{}, err2
	}

	return NewOID(string(bytes.TrimSpace(output)))
}

func (repo *Repository) UpdateRef(refname string, oid OID) error {
	var cmd *exec.Cmd

	if oid == NullOID {
		cmd = repo.gitCommand("update-ref", "-d", refname)
	} else {
		cmd = repo.gitCommand("update-ref", refname, oid.String())
	}
	return cmd.Run()
}

// Next returns the next object, or EOF when done.
func (l *ObjectIter) Next() (OID, ObjectType, counts.Count32, error) {
	line, err := l.f.ReadString('\n')
	if err != nil {
		return OID{}, "", 0, err
	}

	return parseBatchHeader("", line)
}

func (l *ObjectIter) Close() error {
	l.out1.Close()
	err := <-l.errChan
	l.out2.Close()
	err2 := l.cmd1.Wait()
	if err == nil {
		err = err2
	}
	err2 = l.cmd2.Wait()
	if err == nil {
		err = err2
	}
	return err
}

type ObjectHeaderIter struct {
	name string
	data string
}

// Iterate over an object header. `data` should be the object's
// contents, including the "\n\n" that separates the header from the
// rest of the contents. `name` is used in error messages.
func NewObjectHeaderIter(name string, data []byte) (ObjectHeaderIter, error) {
	headerEnd := bytes.Index(data, []byte("\n\n"))
	if headerEnd == -1 {
		return ObjectHeaderIter{}, fmt.Errorf("%s has no header separator", name)
	}
	return ObjectHeaderIter{name, string(data[:headerEnd+1])}, nil
}

func (iter *ObjectHeaderIter) HasNext() bool {
	return len(iter.data) > 0
}

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

type Commit struct {
	Size    counts.Count32
	Parents []OID
	Tree    OID
}

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

type Tree struct {
	data string
}

func ParseTree(oid OID, data []byte) (*Tree, error) {
	return &Tree{string(data)}, nil
}

func (tree Tree) Size() counts.Count32 {
	return counts.NewCount32(uint64(len(tree.data)))
}

// Note that Name shares memory with the tree data that were
// originally read; i.e., retaining a pointer to Name keeps the tree
// data reachable.
type TreeEntry struct {
	Name     string
	OID      OID
	Filemode uint
}

type TreeIter struct {
	// The as-yet-unread part of the tree's data.
	data string
}

func (tree *Tree) Iter() *TreeIter {
	return &TreeIter{
		data: tree.data,
	}
}

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

type Tag struct {
	Size         counts.Count32
	Referent     OID
	ReferentType ObjectType
}

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
