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

	"github.com/github/git-sizer/pipe"
)

// The type of an object ("blob", "tree", "commit", "tag", "missing").
type ObjectType string

type Oid struct {
	v [20]byte
}

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

type Repository struct {
	path string

	batch *pipe.CommandPipe
	check *pipe.CommandPipe
}

func NewRepository(path string) (*Repository, error) {
	batch, err := pipe.NewCommandPipe("git", "-C", path, "cat-file", "--batch")
	if err != nil {
		return nil, err
	}

	check, err := pipe.NewCommandPipe("git", "-C", path, "cat-file", "--batch-check")
	if err != nil {
		return nil, err
	}

	return &Repository{
		path:  path,
		batch: batch,
		check: check,
	}, nil
}

func (repo *Repository) Close() error {
	err1 := repo.batch.Close()
	err2 := repo.check.Close()

	if err1 != nil {
		return err1
	} else {
		return err2
	}
}

type Reference struct {
	Refname    string
	ObjectType ObjectType
	ObjectSize Count32
	Oid        Oid
}

type ReferenceOrError struct {
	Reference Reference
	Error     error
}

func (repo *Repository) ForEachRef(done <-chan interface{}) (<-chan ReferenceOrError, error) {
	command := exec.Command(
		"git", "-C", repo.path,
		"for-each-ref", "--format=%(objectname) %(objecttype) %(objectsize) %(refname)",
	)

	stdoutFile, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}

	command.Stderr = os.Stderr

	err = command.Start()
	if err != nil {
		return nil, err
	}
	stdout := bufio.NewReader(stdoutFile)

	out := make(chan ReferenceOrError)

	go func(done <-chan interface{}, out chan<- ReferenceOrError) {
		defer func() {
			close(out)
			stdoutFile.Close()
			command.Wait()
		}()

		for {
			line, err := stdout.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					out <- ReferenceOrError{Reference{}, err}
				}
				return
			}
			line = line[:len(line)-1]
			words := strings.Split(line, " ")
			if len(words) != 4 {
				break
			}
			oid, err := NewOid(words[0])
			if err != nil {
				break
			}
			objectType := ObjectType(words[1])
			objectSize, err := strconv.ParseUint(words[2], 10, 0)
			if err != nil {
				break
			}
			refname := words[3]
			re := ReferenceOrError{
				Reference{refname, objectType, NewCount32(objectSize), oid},
				nil,
			}
			select {
			case out <- re:
			case <-done:
				return
			}
		}

		out <- ReferenceOrError{Reference{}, errors.New("invalid for-each-ref output")}
	}(done, out)

	return out, nil
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

func (repo *Repository) ForEachFilteredRef(
	done <-chan interface{}, filter ReferenceFilter,
) (<-chan ReferenceOrError, error) {
	refs, err := repo.ForEachRef(done)
	if err != nil {
		return nil, err
	}

	out := make(chan ReferenceOrError)

	go func() {
		defer close(out)

		for re := range refs {
			if re.Error != nil || filter(re.Reference) {
				select {
				case out <- re:
				case <-done:
					return
				}
			}
		}
	}()

	return out, nil
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

func (repo *Repository) ReadHeader(spec string) (Oid, ObjectType, Count32, error) {
	var header string
	var err error

	repo.check.RunQuery(
		spec+"\n",
		func(f *bufio.Reader) {
			header, err = f.ReadString('\n')
		},
	)
	if err != nil {
		return Oid{}, "missing", 0, err
	}
	return parseBatchHeader(spec, header)
}

func (repo *Repository) readObject(spec string) (Oid, ObjectType, []byte, error) {
	var err error
	var oid Oid
	var objectType ObjectType
	var data []byte

	repo.batch.RunQuery(
		spec+"\n",
		func(f *bufio.Reader) {
			var header string
			header, err = f.ReadString('\n')
			if err != nil {
				return
			}
			var size Count32
			oid, objectType, size, err = parseBatchHeader(spec, header)
			if err != nil {
				return
			}
			// +1 for LF:
			data = make([]byte, size+1)
			_, err = io.ReadFull(f, data)
			if err != nil {
				return
			}
			data = data[:len(data)-1]
		},
	)

	if err != nil {
		return Oid{}, "missing", []byte{}, err
	}

	// -1 to remove LF:
	return oid, objectType, data, nil
}

type AllObjectIter struct {
	command *exec.Cmd
	stdout  io.ReadCloser
	f       *bufio.Reader
}

// NewAllObjectIter returns an iterator that iterates over all of the
// objects in `repo` (including unreachable objects).
func (repo *Repository) NewAllObjectIter() (*AllObjectIter, error) {
	command := exec.Command(
		"git", "-C", repo.path,
		"cat-file", "--batch-check", "--batch-all-objects", "--buffer",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}

	command.Stderr = os.Stderr

	err = command.Start()
	if err != nil {
		return nil, err
	}

	return &AllObjectIter{
		command: command,
		stdout:  stdout,
		f:       bufio.NewReader(stdout),
	}, nil
}

// Next returns the next object, or EOF when done.
func (l *AllObjectIter) Next() (Oid, ObjectType, Count32, error) {
	line, err := l.f.ReadString('\n')
	if err != nil {
		return Oid{}, "", 0, err
	}

	return parseBatchHeader("", line)
}

func (l *AllObjectIter) Close() error {
	l.stdout.Close()
	return l.command.Wait()
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

type CommitIter struct {
	command *exec.Cmd
	stdout  io.Closer
	f       *bufio.Reader
}

// NewCommitIter returns an iterator that iterates over all of the
// reachable commits in `repo` (including unreachable objects). `args`
// are selection arguments passed to `git log`. Note that the commit's
// sizes are not filled in by the iterator!
func (repo *Repository) NewCommitIter(args ...string) (*CommitIter, error) {
	cmdArgs := []string{"-C", repo.path, "log", "--format=%H %T %P"}
	cmdArgs = append(cmdArgs, args...)
	command := exec.Command("git", cmdArgs...)

	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}

	command.Stderr = os.Stderr

	err = command.Start()
	if err != nil {
		return nil, err
	}

	return &CommitIter{
		command: command,
		stdout:  stdout,
		f:       bufio.NewReader(stdout),
	}, nil
}

// Next returns the next object, or EOF when done. Note that the sizes
// are not filled in by this function!
func (l *CommitIter) Next() (Oid, Commit, error) {
	line, err := l.f.ReadString('\n')
	if err != nil {
		return Oid{}, Commit{}, err
	}
	line = strings.TrimSpace(line)

	words := strings.Split(line, " ")
	if len(words) < 2 {
		return Oid{}, Commit{}, fmt.Errorf("invalid rev-list output %v", line)
	}

	oid, err := NewOid(words[0])
	if err != nil {
		return Oid{}, Commit{}, fmt.Errorf("invalid oid in rev-list output %v", line)
	}

	treeOid, err := NewOid(words[1])
	if err != nil {
		return Oid{}, Commit{}, fmt.Errorf("invalid tree oid in rev-list output %v", line)
	}

	parentSha1s := words[2:]
	parentOids := make([]Oid, len(parentSha1s))
	for i, word := range parentSha1s {
		parentOid, err := NewOid(word)
		if err != nil {
			return Oid{}, Commit{}, fmt.Errorf("invalid parent oid in rev-list output %v", line)
		}
		parentOids[i] = parentOid
	}

	return oid, Commit{
		Size:    0, // Still needs to be filled in
		Parents: parentOids,
		Tree:    treeOid,
	}, nil
}

func (l *CommitIter) Close() error {
	l.stdout.Close()
	return l.command.Wait()
}

func (repo *Repository) ReadCommit(oid Oid) (*Commit, error) {
	oid, objectType, data, err := repo.readObject(oid.String())
	if err != nil {
		return nil, err
	}
	if objectType != "commit" {
		return nil, fmt.Errorf("expected commit; found %s for object %s", objectType, oid)
	}
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

func (iter *TreeIter) NextEntry() (TreeEntry, bool, error) {
	var entry TreeEntry

	if len(iter.data) == 0 {
		return TreeEntry{}, false, nil
	}

	spAt := bytes.IndexByte(iter.data, ' ')
	if spAt < 0 {
		return TreeEntry{}, false, errors.New("failed to find SP after mode")
	}
	mode, err := strconv.ParseUint(string(iter.data[:spAt]), 8, 32)
	if err != nil {
		return TreeEntry{}, false, err
	}
	entry.Filemode = uint(mode)

	iter.data = iter.data[spAt+1:]
	nulAt := bytes.IndexByte(iter.data, 0)
	if nulAt < 0 {
		return TreeEntry{}, false, errors.New("failed to find NUL after filename")
	}

	entry.Name = string(iter.data[:nulAt])

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

func (repo *Repository) ReadTag(oid Oid) (*Tag, error) {
	oid, objectType, data, err := repo.readObject(oid.String())
	if err != nil {
		return nil, err
	}
	if objectType != "tag" {
		return nil, fmt.Errorf("expected tag; found %s for object %s", objectType, oid)
	}
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
