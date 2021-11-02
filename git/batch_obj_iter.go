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
