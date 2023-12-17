package git

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/github/go-pipe/pipe"
)

// ObjectIter iterates over objects in a Git repository.
type ObjectIter struct {
	ctx      context.Context
	p        *pipe.Pipeline
	oidCh    chan OID
	errCh    chan error
	headerCh chan BatchHeader
}

// NewObjectIter returns an iterator that iterates over objects in
// `repo`. The arguments are passed to `git rev-list --objects`. The
// second return value is the stdin of the `rev-list` command. The
// caller can feed values into it but must close it in any case.
func (repo *Repository) NewObjectIter(ctx context.Context) (*ObjectIter, error) {
	iter := ObjectIter{
		ctx:      ctx,
		p:        pipe.New(),
		oidCh:    make(chan OID),
		errCh:    make(chan error),
		headerCh: make(chan BatchHeader),
	}
	hashHexSize := repo.HashSize() * 2
	iter.p.Add(
		// Read OIDs from `iter.oidCh` and write them to `git
		// rev-list`:
		pipe.Function(
			"request-objects",
			func(ctx context.Context, _ pipe.Env, _ io.Reader, stdout io.Writer) error {
				out := bufio.NewWriter(stdout)

				for {
					select {
					case oid, ok := <-iter.oidCh:
						if !ok {
							return out.Flush()
						}
						if _, err := fmt.Fprintln(out, oid.String()); err != nil {
							return fmt.Errorf("writing to 'git cat-file': %w", err)
						}
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			},
		),

		// Walk starting at the OIDs on `stdin` and output the OIDs
		// (possibly followed by paths) of all of the Git objects
		// found.
		pipe.CommandStage(
			"git-rev-list",
			repo.GitCommand("rev-list", "--objects", "--stdin", "--date-order"),
		),

		// Read the output of `git rev-list --objects`, strip off any
		// trailing information, and write the OIDs to `git cat-file`:
		pipe.LinewiseFunction(
			"copy-oids",
			func(_ context.Context, _ pipe.Env, line []byte, stdout *bufio.Writer) error {
				if len(line) < hashHexSize {
					return fmt.Errorf("line too short: '%s'", line)
				}
				if _, err := stdout.Write(line[:hashHexSize]); err != nil {
					return fmt.Errorf("writing OID to 'git cat-file': %w", err)
				}
				if err := stdout.WriteByte('\n'); err != nil {
					return fmt.Errorf("writing LF to 'git cat-file': %w", err)
				}
				return nil
			},
		),

		// Process the OIDs from stdin and, for each object, output a
		// header:
		pipe.CommandStage(
			"git-cat-file",
			repo.GitCommand("cat-file", "--batch-check", "--buffer"),
		),

		// Parse the object headers and shove them into `headerCh`:
		pipe.Function(
			"object-parser",
			func(ctx context.Context, _ pipe.Env, stdin io.Reader, _ io.Writer) error {
				defer close(iter.headerCh)

				f := bufio.NewReader(stdin)

				for {
					header, err := f.ReadString('\n')
					if err != nil {
						if err == io.EOF {
							return nil
						}
						return fmt.Errorf("reading from 'git cat-file': %w", err)
					}
					batchHeader, err := ParseBatchHeader("", header)
					if err != nil {
						return fmt.Errorf("parsing output of 'git cat-file': %w", err)
					}

					iter.headerCh <- batchHeader
				}
			},
		),
	)

	if err := iter.p.Start(ctx); err != nil {
		return nil, err
	}

	return &iter, nil
}

// AddRoot adds another OID to be included in the walk.
func (iter *ObjectIter) AddRoot(oid OID) error {
	select {
	case iter.oidCh <- oid:
		return nil
	case <-iter.ctx.Done():
		return iter.ctx.Err()
	}
}

// Close closes the iterator and frees up resources.
func (iter *ObjectIter) Close() {
	close(iter.oidCh)
}

// Next returns either the next object (its OID, type, and size), or a
// `false` boolean value to indicate that there are no data left.
func (iter *ObjectIter) Next() (BatchHeader, bool, error) {
	header, ok := <-iter.headerCh
	if !ok {
		return missingHeader, false, iter.p.Wait()
	}
	return header, true, nil
}
