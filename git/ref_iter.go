package git

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/github/git-sizer/internal/pipe"
)

// ReferenceIter is an iterator that interates over references.
type ReferenceIter struct {
	refCh chan Reference
	errCh chan error
}

// NewReferenceIter returns an iterator that iterates over all of the
// references in `repo`.
func (repo *Repository) NewReferenceIter(ctx context.Context) (*ReferenceIter, error) {
	iter := ReferenceIter{
		refCh: make(chan Reference),
		errCh: make(chan error),
	}

	p := pipe.New()
	p.Add(
		// Output all references and their values:
		pipe.CommandStage(
			"git-for-each-ref",
			repo.GitCommand(
				"for-each-ref",
				"--format=%(objectname) %(objecttype) %(objectsize) %(refname)",
			),
		),

		// Read the references and send them to `iter.refCh`, then close
		// the channel.
		pipe.Function(
			"parse-refs",
			func(ctx context.Context, env pipe.Env, stdin io.Reader, stdout io.Writer) error {
				defer close(iter.refCh)

				in := bufio.NewReader(stdin)
				for {
					line, err := in.ReadBytes('\n')
					if err != nil {
						if err == io.EOF {
							return nil
						}
						return fmt.Errorf("reading 'git for-each-ref' output: %w", err)
					}

					ref, err := ParseReference(string(line[:len(line)-1]))
					if err != nil {
						return fmt.Errorf("parsing 'git for-each-ref' output: %w", err)
					}
					select {
					case iter.refCh <- ref:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			},
		),
	)

	err := p.Start(ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		iter.errCh <- p.Wait()
	}()

	return &iter, nil
}

// Next returns either the next reference or a boolean `false` value
// indicating that the iteration is over. On errors, return an error
// (in this case, the caller must still call `Close()`).
func (iter *ReferenceIter) Next() (Reference, bool, error) {
	ref, ok := <-iter.refCh
	if !ok {
		return Reference{}, false, <-iter.errCh
	}

	return ref, true, nil
}
