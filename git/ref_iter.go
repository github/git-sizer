package git

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/github/git-sizer/counts"
)

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
