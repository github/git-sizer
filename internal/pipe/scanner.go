package pipe

import (
	"bufio"
	"context"
	"io"
)

// Scanner defines the interface (which is implemented by
// `bufio.Scanner`) that is needed by `AddScannerFunction()`. See
// `bufio.Scanner` for how these methods should behave.
type Scanner interface {
	Scan() bool
	Bytes() []byte
	Err() error
}

// NewScannerFunc is used to create a `Scanner` for scanning input
// that is coming from `r`.
type NewScannerFunc func(r io.Reader) (Scanner, error)

// ScannerFunction creates a function-based `Stage`. The function will
// be passed input, one line at a time, and may emit output. See the
// definition of `LinewiseStageFunc` for more information.
func ScannerFunction(
	name string, newScanner NewScannerFunc, f LinewiseStageFunc,
) Stage {
	return Function(
		name,
		func(ctx context.Context, env Env, stdin io.Reader, stdout io.Writer) (theErr error) {
			scanner, err := newScanner(stdin)
			if err != nil {
				return err
			}

			var out *bufio.Writer
			if stdout != nil {
				out = bufio.NewWriter(stdout)
				defer func() {
					err := out.Flush()
					if err != nil && theErr == nil {
						// Note: this sets the named return value,
						// thereby causing the whole stage to report
						// the error.
						theErr = err
					}
				}()
			}

			for scanner.Scan() {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				err := f(ctx, env, scanner.Bytes(), out)
				if err != nil {
					return err
				}
			}
			if err := scanner.Err(); err != nil {
				return err
			}

			return nil
			// `p.AddFunction()` arranges for `stdout` to be closed.
		},
	)
}
