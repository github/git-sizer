package sizes

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// The type of an object ("blob", "tree", "commit", "tag", "missing").
type ObjectType string

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

type commandPipe struct {
	command      *exec.Cmd
	stdin        io.WriteCloser
	stdoutWriter io.ReadCloser
	stdout       *bufio.Reader
}

func newCommandPipe(name string, arg ...string) (*commandPipe, error) {
	command := exec.Command(name, arg...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = command.Start()
	if err != nil {
		return nil, err
	}
	return &commandPipe{
		command:      command,
		stdin:        stdin,
		stdoutWriter: stdout,
		stdout:       bufio.NewReader(stdout),
	}, nil
}

type Repository struct {
	path string

	batch *commandPipe
	check *commandPipe
}

func NewRepository(path string) (*Repository, error) {
	batch, err := newCommandPipe("git", "-C", path, "cat-file", "--batch")
	if err != nil {
		return nil, err
	}

	check, err := newCommandPipe("git", "-C", path, "cat-file", "--batch-check")
	if err != nil {
		return nil, err
	}

	return &Repository{
		path:  path,
		batch: batch,
		check: check,
	}, nil
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
			out <- ReferenceOrError{Reference{refname, objectType, NewCount32(objectSize), oid}, nil}
		}

		out <- ReferenceOrError{Reference{}, errors.New("invalid for-each-ref output")}
	}(done, out)

	return out, nil
}

// Parse a `cat-file --batch[-check]` output header line (including
// the trailing LF). `spec` is used in error messages.
func (repo *Repository) parseBatchHeader(spec string, header string) (Oid, ObjectType, Count32, error) {
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
	return oid, ObjectType(words[1]), NewCount32(size), nil
}

func (repo *Repository) ReadHeader(spec string) (Oid, ObjectType, Count32, error) {
	fmt.Fprintf(repo.check.stdin, "%s\n", spec)
	header, err := repo.check.stdout.ReadString('\n')
	if err != nil {
		return Oid{}, "missing", 0, err
	}
	return repo.parseBatchHeader(spec, header)
}

func (repo *Repository) readObject(spec string) (Oid, ObjectType, []byte, error) {
	fmt.Fprintf(repo.batch.stdin, "%s\n", spec)
	header, err := repo.batch.stdout.ReadString('\n')
	if err != nil {
		return Oid{}, "missing", []byte{}, err
	}
	oid, objectType, size, err := repo.parseBatchHeader(spec, header)
	if err != nil {
		return Oid{}, "missing", []byte{}, err
	}
	// +1 for LF:
	data := make([]byte, size+1)
	rest := data
	for len(rest) > 0 {
		n, err := repo.batch.stdout.Read(rest)
		if err != nil {
			return Oid{}, "missing", []byte{}, err
		}
		rest = rest[n:]
	}
	// -1 to remove LF:
	return oid, objectType, data[:len(data)-1], nil
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
	Name       string
	Oid        Oid
	ObjectType ObjectType
	Filemode   uint
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
