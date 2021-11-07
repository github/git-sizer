package pipe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
)

// Env represents the environment that a pipeline stage should run in.
// It is passed to `Stage.Start()`.
type Env struct {
	// The directory in which external commands should be executed by
	// default.
	Dir string
}

// FinishEarly is an error that can be returned by a `Stage` to
// request that the iteration be ended early (possibly without reading
// all of its input). This "error" is considered a successful return,
// and is not reported to the caller.
//nolint:errname
var FinishEarly = errors.New("finish stage early")

// Pipeline represents a Unix-like pipe that can include multiple
// stages, including external processes but also and stages written in
// Go.
type Pipeline struct {
	env Env

	stdin  io.Reader
	stdout io.WriteCloser
	stages []Stage
	cancel func()

	// Atomically written and read value, nonzero if the pipeline has
	// been started. This is only used for lifecycle sanity checks but
	// does not guarantee that clients are using the class correctly.
	started uint32
}

type nopWriteCloser struct {
	io.Writer
}

func (w nopWriteCloser) Close() error {
	return nil
}

// NewPipeline returns a Pipeline struct with all of the `options`
// applied.
func New(options ...Option) *Pipeline {
	p := &Pipeline{}

	for _, option := range options {
		option(p)
	}

	return p
}

// Option is a type alias for Pipeline functional options.
type Option func(*Pipeline)

// WithDir sets the default directory for running external commands.
func WithDir(dir string) Option {
	return func(p *Pipeline) {
		p.env.Dir = dir
	}
}

// WithStdin assigns stdin to the first command in the pipeline.
func WithStdin(stdin io.Reader) Option {
	return func(p *Pipeline) {
		p.stdin = stdin
	}
}

// WithStdout assigns stdout to the last command in the pipeline.
func WithStdout(stdout io.Writer) Option {
	return func(p *Pipeline) {
		p.stdout = nopWriteCloser{stdout}
	}
}

// WithStdoutCloser assigns stdout to the last command in the
// pipeline, and closes stdout when it's done.
func WithStdoutCloser(stdout io.WriteCloser) Option {
	return func(p *Pipeline) {
		p.stdout = stdout
	}
}

func (p *Pipeline) hasStarted() bool {
	return atomic.LoadUint32(&p.started) != 0
}

// Add appends one or more stages to the pipeline.
func (p *Pipeline) Add(stages ...Stage) {
	if p.hasStarted() {
		panic("attempt to modify a pipeline that has already started")
	}

	p.stages = append(p.stages, stages...)
}

// AddWithIgnoredError appends one or more stages that are ignoring
// the passed in error to the pipeline.
func (p *Pipeline) AddWithIgnoredError(em ErrorMatcher, stages ...Stage) {
	if p.hasStarted() {
		panic("attempt to modify a pipeline that has already started")
	}

	for _, stage := range stages {
		p.stages = append(p.stages, IgnoreError(stage, em))
	}
}

// Start starts the commands in the pipeline. If `Start()` exits
// without an error, `Wait()` must also be called, to allow all
// resources to be freed.
func (p *Pipeline) Start(ctx context.Context) error {
	if p.hasStarted() {
		panic("attempt to start a pipeline that has already started")
	}

	atomic.StoreUint32(&p.started, 1)
	ctx, p.cancel = context.WithCancel(ctx)

	var nextStdin io.ReadCloser
	if p.stdin != nil {
		// We don't want the first stage to actually close this, and
		// it's not even an `io.ReadCloser`, so fake it:
		nextStdin = io.NopCloser(p.stdin)
	}

	for i, s := range p.stages {
		var err error
		stdout, err := s.Start(ctx, p.env, nextStdin)
		if err != nil {
			// Close the pipe that the previous stage was writing to.
			// That should cause it to exit even if it's not minding
			// its context.
			if nextStdin != nil {
				_ = nextStdin.Close()
			}

			// Kill and wait for any stages that have been started
			// already to finish:
			p.cancel()
			for _, s := range p.stages[:i] {
				_ = s.Wait()
			}
			return fmt.Errorf("starting pipeline stage %q: %w", s.Name(), err)
		}
		nextStdin = stdout
	}

	// If the pipeline was configured with a `stdout`, add a synthetic
	// stage to copy the last stage's stdout to that writer:
	if p.stdout != nil {
		c := newIOCopier(p.stdout)
		p.stages = append(p.stages, c)
		// `ioCopier.Start()` never fails:
		_, _ = c.Start(ctx, p.env, nextStdin)
	}

	return nil
}

func (p *Pipeline) Output(ctx context.Context) ([]byte, error) {
	var buf bytes.Buffer
	p.stdout = nopWriteCloser{&buf}
	err := p.Run(ctx)
	return buf.Bytes(), err
}

// Wait waits for each stage in the pipeline to exit.
func (p *Pipeline) Wait() error {
	if !p.hasStarted() {
		panic("unable to wait on a pipeline that has not started")
	}

	// Make sure that all of the cleanup eventually happens:
	defer p.cancel()

	var earliestStageErr error
	var earliestFailedStage Stage

	finishedEarly := false
	for i := len(p.stages) - 1; i >= 0; i-- {
		s := p.stages[i]
		err := s.Wait()

		// Handle errors:
		switch {
		case err == nil:
			// No error to handle. But unset the `finishedEarly` flag,
			// because earlier stages shouldn't be affected by the
			// later stage that finished early.
			finishedEarly = false
			continue

		case errors.Is(err, FinishEarly):
			// We ignore `FinishEarly` errors because that is how a
			// stage informs us that it intentionally finished early.
			// Moreover, if we see a `FinishEarly` error, ignore any
			// pipe error from the immediately preceding stage,
			// because it probably came from trying to write to this
			// stage after this stage closed its stdin.
			finishedEarly = true
			continue

		case IsPipeError(err):
			switch {
			case finishedEarly:
				// A successor stage finished early. It is common for
				// this to cause earlier stages to fail with pipe
				// errors. Such errors are uninteresting, so ignore
				// them. Leave the `finishedEarly` flag set, because
				// the preceding stage might get a pipe error from
				// trying to write to this one.
			case earliestStageErr != nil:
				// A later stage has already reported an error. This
				// means that we don't want to report the error from
				// this stage:
				//
				// * If the later error was also a pipe error: we want
				//   to report the _last_ pipe error seen, which would
				//   be the one already recorded.
				//
				// * If the later error was not a pipe error: non-pipe
				//   errors are always considered more important than
				//   pipe errors, so again we would want to keep the
				//   error that is already recorded.
			default:
				// In this case, the pipe error from this stage is the
				// most important error that we have seen so far, so
				// remember it:
				earliestFailedStage, earliestStageErr = s, err
			}

		default:
			// This stage exited with a non-pipe error. If multiple
			// stages exited with such errors, we want to report the
			// one that is most informative. We take that to be the
			// error from the earliest failing stage. Since we are
			// iterating through stages in reverse order, overwrite
			// any existing remembered errors (which would have come
			// from a later stage):
			earliestFailedStage, earliestStageErr = s, err
			finishedEarly = false
		}
	}

	if earliestStageErr != nil {
		return fmt.Errorf("%s: %w", earliestFailedStage.Name(), earliestStageErr)
	}

	return nil
}

// Run starts and waits for the commands in the pipeline.
func (p *Pipeline) Run(ctx context.Context) error {
	if err := p.Start(ctx); err != nil {
		return err
	}

	return p.Wait()
}
