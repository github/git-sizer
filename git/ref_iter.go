package git

import (
	"bufio"
	"io"
	"os"
	"os/exec"
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
	cmd := repo.GitCommand(
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
	ref, err := ParseReference(line[:len(line)-1])
	if err != nil {
		return ref, false, err
	}

	return ref, true, nil
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
