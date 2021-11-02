package git

import (
	"bufio"
	"io"
	"os"
	"os/exec"
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
	cmd := repo.GitCommand("cat-file", "--batch", "--buffer")

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
func (iter *BatchObjectIter) Next() (BatchHeader, []byte, error) {
	header, err := iter.f.ReadString('\n')
	if err != nil {
		return missingHeader, nil, err
	}
	obj, err := ParseBatchHeader("", header)
	if err != nil {
		return missingHeader, nil, err
	}
	// +1 for LF:
	data := make([]byte, obj.ObjectSize+1)
	_, err = io.ReadFull(iter.f, data)
	if err != nil {
		return missingHeader, nil, err
	}
	data = data[:len(data)-1]
	return obj, data, nil
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
