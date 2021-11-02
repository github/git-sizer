package git

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
)

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
	cmd1 := repo.GitCommand(append([]string{"rev-list", "--objects"}, args...)...)
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

	cmd2 := repo.GitCommand("cat-file", "--batch-check", "--buffer")
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
func (iter *ObjectIter) Next() (BatchHeader, error) {
	line, err := iter.f.ReadString('\n')
	if err != nil {
		return missingHeader, err
	}

	return ParseBatchHeader("", line)
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
