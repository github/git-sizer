package git

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/github/git-sizer/internal/pipe"
)

type ObjectRecord struct {
	BatchHeader
	Data []byte
}

// BatchObjectIter iterates over objects whose names are fed into its
// stdin. The output is buffered, so it has to be closed before you
// can be sure that you have gotten all of the objects.
type BatchObjectIter struct {
	ctx   context.Context
	p     *pipe.Pipeline
	oidCh chan OID
	objCh chan ObjectRecord
	errCh chan error
}

// NewBatchObjectIter returns a `*BatchObjectIterator` and an
// `io.WriteCloser`. The iterator iterates over objects whose names
// are fed into the `io.WriteCloser`, one per line. The
// `io.WriteCloser` should normally be closed and the iterator's
// output drained before `Close()` is called.
func (repo *Repository) NewBatchObjectIter(ctx context.Context) (*BatchObjectIter, error) {
	iter := BatchObjectIter{
		ctx:   ctx,
		p:     pipe.New(),
		oidCh: make(chan OID),
		objCh: make(chan ObjectRecord),
		errCh: make(chan error),
	}

	iter.p.Add(
		// Read OIDs from `iter.oidCh` and write them to `git
		// cat-file`:
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

		// Read OIDs from `stdin` and output a header line followed by
		// the contents of the corresponding Git objects:
		pipe.CommandStage(
			"git-cat-file",
			repo.GitCommand("cat-file", "--batch", "--buffer"),
		),

		// Parse the object headers and read the object contents, and
		// shove both into `objCh`:
		pipe.Function(
			"object-reader",
			func(ctx context.Context, _ pipe.Env, stdin io.Reader, _ io.Writer) error {
				defer close(iter.objCh)

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

					// Read the object contents plus the trailing LF
					// (which is discarded below while creating the
					// `ObjectRecord`):
					data := make([]byte, batchHeader.ObjectSize+1)
					if _, err := io.ReadFull(f, data); err != nil {
						return fmt.Errorf(
							"reading object data from 'git cat-file' for %s '%s': %w",
							batchHeader.ObjectType, batchHeader.OID, err,
						)
					}

					select {
					case iter.objCh <- ObjectRecord{
						BatchHeader: batchHeader,
						Data:        data[:batchHeader.ObjectSize],
					}:
					case <-iter.ctx.Done():
						return iter.ctx.Err()
					}
				}
			},
		),
	)

	if err := iter.p.Start(ctx); err != nil {
		return nil, err
	}

	return &iter, nil
}

// RequestObject requests that the object with the specified `oid` be
// processed. The objects registered via this method can be read using
// `Next()` in the order that they were requested.
func (iter *BatchObjectIter) RequestObject(oid OID) error {
	select {
	case iter.oidCh <- oid:
		return nil
	case <-iter.ctx.Done():
		return iter.ctx.Err()
	}
}

// Close closes the iterator and frees up resources. Close must be
// called exactly once.
func (iter *BatchObjectIter) Close() {
	close(iter.oidCh)
}

// Next either returns the next object (its header and contents), or a
// `false` boolean value if no more objects are left. Objects need to
// be read asynchronously, but the last objects won't necessarily show
// up here until `Close()` has been called.
func (iter *BatchObjectIter) Next() (ObjectRecord, bool, error) {
	obj, ok := <-iter.objCh
	if !ok {
		return ObjectRecord{
			BatchHeader: missingHeader,
		}, false, iter.p.Wait()
	}
	return obj, true, nil
}
