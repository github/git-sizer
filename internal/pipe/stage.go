package pipe

import (
	"context"
	"io"
)

// Stage is an element of a `Pipeline`.
type Stage interface {
	// Name returns the name of the stage.
	Name() string

	// Start starts the stage in the background, in the environment
	// described by `env`, and using `stdin` as input. (`stdin` should
	// be set to `nil` if the stage is to receive no input, which
	// might be the case for the first stage in a pipeline.) It
	// returns an `io.ReadCloser` from which the stage's output can be
	// read (or `nil` if it generates no output, which should only be
	// the case for the last stage in a pipeline). It is the stages'
	// responsibility to close `stdin` (if it is not nil) when it has
	// read all of the input that it needs, and to close the write end
	// of its output reader when it is done, as that is generally how
	// the subsequent stage knows that it has received all of its
	// input and can finish its work, too.
	//
	// If `Start()` returns without an error, `Wait()` must also be
	// called, to allow all resources to be freed.
	Start(ctx context.Context, env Env, stdin io.ReadCloser) (io.ReadCloser, error)

	// Wait waits for the stage to be done, either because it has
	// finished or because it has been killed due to the expiration of
	// the context passed to `Start()`.
	Wait() error
}
