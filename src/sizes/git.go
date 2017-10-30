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
type Type string

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

type Reference struct {
	Refname    string
	ObjectType Type
	ObjectSize Count
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
			objectType := Type(words[1])
			objectSize, err := strconv.ParseUint(words[2], 10, 0)
			if err != nil {
				break
			}
			refname := words[3]
			out <- ReferenceOrError{Reference{refname, objectType, Count(objectSize), oid}, nil}
		}

		out <- ReferenceOrError{Reference{}, errors.New("invalid for-each-ref output")}
	}(done, out)

	return out, nil
}

// Parse a `cat-file --batch[-check]` output header line (including
// the trailing LF). `spec` is used in error messages.
func (repo *Repository) parseBatchHeader(spec string, header string) (Oid, Type, Count, error) {
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
	return repo.parseBatchHeader(spec, header)
}

func (repo *Repository) readObject(spec string) (Oid, Type, []byte, error) {
	fmt.Fprintf(repo.batchStdin, "%s\n", spec)
	header, err := repo.batchStdout.ReadString('\n')
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
