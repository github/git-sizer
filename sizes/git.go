package sizes

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// The type of an object ("blob", "tree", "commit", "tag", "missing").
type ObjectType string

type Oid struct {
	v [20]byte
}

var NullOid Oid

func OidFromBytes(oidBytes []byte) (Oid, error) {
	var oid Oid
	if len(oidBytes) != len(oid.v) {
		return Oid{}, errors.New("bytes oid has the wrong length")
	}
	copy(oid.v[0:20], oidBytes)
	return oid, nil
}

func NewOid(s string) (Oid, error) {
	oidBytes, err := hex.DecodeString(s)
	if err != nil {
		return Oid{}, err
	}
	return OidFromBytes(oidBytes)
}

func (oid Oid) String() string {
	return hex.EncodeToString(oid.v[:])
}

func (oid Oid) MarshalJSON() ([]byte, error) {
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

func NewRepository(path string) (*Repository, error) {
	command := exec.Command(
		"git", "-C", path,
		"rev-parse", "--git-dir",
	)
	out, err := command.Output()
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
	repo := &Repository{
		path: string(bytes.TrimSpace(out)),
	}
	return repo, nil
}

func (repo *Repository) Close() error {
	return nil
}

type Reference struct {
	Refname    string
	ObjectType ObjectType
	ObjectSize Count32
	Oid        Oid
}

type ReferenceIter struct {
	command *exec.Cmd
	out     io.ReadCloser
	f       *bufio.Reader
	errChan <-chan error
}

// NewReferenceIter returns an iterator that iterates over all of the
// references in `repo`.
func (repo *Repository) NewReferenceIter() (*ReferenceIter, error) {
	command := exec.Command(
		"git", "-C", repo.path,
		"for-each-ref", "--format=%(objectname) %(objecttype) %(objectsize) %(refname)",
	)

	out, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}

	command.Stderr = os.Stderr

	err = command.Start()
	if err != nil {
		return nil, err
	}

	return &ReferenceIter{
		command: command,
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
	oid, err := NewOid(words[0])
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
		ObjectSize: Count32(objectSize),
		Oid:        oid,
	}, true, nil
}

func (l *ReferenceIter) Close() error {
	err := l.out.Close()
	err2 := l.command.Wait()
	if err == nil {
		err = err2
	}
	return err
}

type BatchObjectIter struct {
	command *exec.Cmd
	out     io.ReadCloser
	f       *bufio.Reader
}

// NewBatchObjectIter returns iterates over objects whose names are
// fed into its stdin. The output is buffered, so it has to be closed
// before you can be sure to read all of the objects.
func (repo *Repository) NewBatchObjectIter() (*BatchObjectIter, io.WriteCloser, error) {
	command := exec.Command(
		"git", "-C", repo.path,
		"cat-file", "--batch", "--buffer",
	)

	in, err := command.StdinPipe()
	if err != nil {
		return nil, nil, err
	}

	out, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	command.Stderr = os.Stderr

	err = command.Start()
	if err != nil {
		return nil, nil, err
	}

	return &BatchObjectIter{
		command: command,
		out:     out,
		f:       bufio.NewReader(out),
	}, in, nil
}

func (iter *BatchObjectIter) Next() (Oid, ObjectType, Count32, []byte, error) {
	header, err := iter.f.ReadString('\n')
	if err != nil {
		return Oid{}, "", 0, nil, err
	}
	oid, objectType, objectSize, err := parseBatchHeader("", header)
	if err != nil {
		return Oid{}, "", 0, nil, err
	}
	// +1 for LF:
	data := make([]byte, objectSize+1)
	_, err = io.ReadFull(iter.f, data)
	if err != nil {
		return Oid{}, "", 0, nil, err
	}
	data = data[:len(data)-1]
	return oid, objectType, objectSize, data, nil
}

func (l *BatchObjectIter) Close() error {
	err := l.out.Close()
	err2 := l.command.Wait()
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
func parseBatchHeader(spec string, header string) (Oid, ObjectType, Count32, error) {
	header = header[:len(header)-1]
	words := strings.Split(header, " ")
	if words[len(words)-1] == "missing" {
		if spec == "" {
			spec = words[0]
		}
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
	return oid, ObjectType(words[1]), NewCount32(size), nil
}

type ObjectIter struct {
	command1 *exec.Cmd
	command2 *exec.Cmd
	in1      io.Writer
	out1     io.ReadCloser
	out2     io.ReadCloser
	f        *bufio.Reader
	errChan  <-chan error
}

// NewObjectIter returns an iterator that iterates over objects in
// `repo`. The second return value is the stdin of the `rev-list`
// command. The caller can feed values into it but must close it in
// any case.
func (repo *Repository) NewObjectIter(args ...string) (
	*ObjectIter, io.WriteCloser, error,
) {
	cmdArgs := []string{"-C", repo.path, "rev-list", "--objects"}
	cmdArgs = append(cmdArgs, args...)
	command1 := exec.Command("git", cmdArgs...)
	in1, err := command1.StdinPipe()
	if err != nil {
		return nil, nil, err
	}

	out1, err := command1.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	command1.Stderr = os.Stderr

	err = command1.Start()
	if err != nil {
		return nil, nil, err
	}

	command2 := exec.Command(
		"git", "-C", repo.path,
		"cat-file", "--batch-check", "--buffer",
	)

	in2, err := command2.StdinPipe()
	if err != nil {
		out1.Close()
		command1.Wait()
		return nil, nil, err
	}

	out2, err := command2.StdoutPipe()
	if err != nil {
		in2.Close()
		out1.Close()
		command1.Wait()
		return nil, nil, err
	}

	command2.Stderr = os.Stderr

	err = command2.Start()
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
		command1: command1,
		command2: command2,
		out1:     out1,
		out2:     out2,
		f:        bufio.NewReader(out2),
		errChan:  errChan,
	}, in1, nil
}

// Next returns the next object, or EOF when done.
func (l *ObjectIter) Next() (Oid, ObjectType, Count32, error) {
	line, err := l.f.ReadString('\n')
	if err != nil {
		return Oid{}, "", 0, err
	}

	return parseBatchHeader("", line)
}

func (l *ObjectIter) Close() error {
	l.out1.Close()
	err := <-l.errChan
	l.out2.Close()
	err2 := l.command1.Wait()
	if err == nil {
		err = err2
	}
	err2 = l.command2.Wait()
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
	Size    Count32
	Parents []Oid
	Tree    Oid
}

func ParseCommit(oid Oid, data []byte) (*Commit, error) {
	var parents []Oid
	var tree Oid
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
	if !treeFound {
		return nil, fmt.Errorf("no tree found in commit %s", oid)
	}
	return &Commit{
		Size:    NewCount32(uint64(len(data))),
		Parents: parents,
		Tree:    tree,
	}, nil
}

type Tree struct {
	data string
}

func ParseTree(oid Oid, data []byte) (*Tree, error) {
	return &Tree{string(data)}, nil
}

// Note that Name shares memory with the tree data that were
// originally read; i.e., retaining a pointer to Name keeps the tree
// data reachable.
type TreeEntry struct {
	Name     string
	Oid      Oid
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

	copy(entry.Oid.v[0:20], iter.data[0:20])
	iter.data = iter.data[20:]

	return entry, true, nil
}

type Tag struct {
	Size         Count32
	Referent     Oid
	ReferentType ObjectType
}

func ParseTag(oid Oid, data []byte) (*Tag, error) {
	var referent Oid
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
			referent, err = NewOid(value)
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
		Size:         NewCount32(uint64(len(data))),
		Referent:     referent,
		ReferentType: referentType,
	}, nil
}
