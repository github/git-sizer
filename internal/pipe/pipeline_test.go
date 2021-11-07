package pipe_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/github/git-sizer/internal/pipe"
)

func TestMain(m *testing.M) {
	// Check whether this package's test suite leaks any goroutines:
	goleak.VerifyTestMain(m)
}

func TestPipelineFirstStageFailsToStart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	startErr := errors.New("foo")

	p := pipe.New()
	p.Add(
		ErrorStartingStage{startErr},
		ErrorStartingStage{errors.New("this error should never happen")},
	)
	assert.ErrorIs(t, p.Run(ctx), startErr)
}

func TestPipelineSecondStageFailsToStart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	startErr := errors.New("foo")

	p := pipe.New()
	p.Add(
		seqFunction(20000),
		ErrorStartingStage{startErr},
	)
	assert.ErrorIs(t, p.Run(ctx), startErr)
}

func TestPipelineSingleCommandOutput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	p.Add(pipe.Command("echo", "hello world"))
	out, err := p.Output(ctx)
	if assert.NoError(t, err) {
		assert.EqualValues(t, "hello world\n", out)
	}
}

func TestPipelineSingleCommandWithStdout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	stdout := &bytes.Buffer{}

	p := pipe.New(pipe.WithStdout(stdout))
	p.Add(pipe.Command("echo", "hello world"))
	if assert.NoError(t, p.Run(ctx)) {
		assert.Equal(t, "hello world\n", stdout.String())
	}
}

func TestNontrivialPipeline(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	p.Add(
		pipe.Command("echo", "hello world"),
		pipe.Command("sed", "s/hello/goodbye/"),
	)
	out, err := p.Output(ctx)
	if assert.NoError(t, err) {
		assert.EqualValues(t, "goodbye world\n", out)
	}
}

func TestPipelineReadFromSlowly(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r, w := io.Pipe()

	var buf []byte
	readErr := make(chan error, 1)

	go func() {
		time.Sleep(200 * time.Millisecond)
		var err error
		buf, err = ioutil.ReadAll(r)
		readErr <- err
	}()

	p := pipe.New(pipe.WithStdout(w))
	p.Add(pipe.Command("echo", "hello world"))
	assert.NoError(t, p.Run(ctx))

	time.Sleep(100 * time.Millisecond)
	// It's not super-intuitive, but `w` has to be closed here so that
	// the `ioutil.ReadAll()` call above knows that it's done:
	_ = w.Close()

	assert.NoError(t, <-readErr)
	assert.Equal(t, "hello world\n", string(buf))
}

func TestPipelineReadFromSlowly2(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIXME: test skipped on Windows: 'seq' unavailable")
	}

	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r, w := io.Pipe()

	var buf []byte
	readErr := make(chan error, 1)

	go func() {
		time.Sleep(100 * time.Millisecond)
		for {
			var c [1]byte
			_, err := r.Read(c[:])
			if err != nil {
				if err == io.EOF {
					readErr <- nil
					return
				}
				readErr <- err
				return
			}
			buf = append(buf, c[0])
			time.Sleep(1 * time.Millisecond)
		}
	}()

	p := pipe.New(pipe.WithStdout(w))
	p.Add(pipe.Command("seq", "100"))
	assert.NoError(t, p.Run(ctx))

	time.Sleep(200 * time.Millisecond)
	// It's not super-intuitive, but `w` has to be closed here so that
	// the `ioutil.ReadAll()` call above knows that it's done:
	_ = w.Close()

	assert.NoError(t, <-readErr)
	assert.Equal(t, 292, len(buf))
}

func TestPipelineTwoCommandsPiping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	p.Add(pipe.Command("echo", "hello world"))
	assert.Panics(t, func() { p.Add(pipe.Command("")) })
	out, err := p.Output(ctx)
	if assert.NoError(t, err) {
		assert.EqualValues(t, "hello world\n", out)
	}
}

func TestPipelineDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIXME: test skipped on Windows: 'pwd' incompatibility")
	}

	t.Parallel()
	ctx := context.Background()

	wdir, err := os.Getwd()
	require.NoError(t, err)
	dir, err := ioutil.TempDir(wdir, "pipeline-test-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	p := pipe.New(pipe.WithDir(dir))
	p.Add(pipe.Command("pwd"))

	out, err := p.Output(ctx)
	if assert.NoError(t, err) {
		assert.Equal(t, dir, strings.TrimSuffix(string(out), "\n"))
	}
}

func TestPipelineExit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	p.Add(
		pipe.Command("false"),
		pipe.Command("true"),
	)
	assert.EqualError(t, p.Run(ctx), "false: exit status 1")
}

func TestPipelineStderr(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir, err := ioutil.TempDir("", "pipeline-test-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	p := pipe.New(pipe.WithDir(dir))
	p.Add(pipe.Command("ls", "doesnotexist"))

	_, err = p.Output(ctx)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "ls: exit status")
	}
}

func TestPipelineInterrupted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIXME: test skipped on Windows: 'sleep' unavailable")
	}

	t.Parallel()
	stdout := &bytes.Buffer{}

	p := pipe.New(pipe.WithStdout(stdout))
	p.Add(pipe.Command("sleep", "10"))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := p.Start(ctx)
	require.NoError(t, err)

	err = p.Wait()
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestPipelineCanceled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIXME: test skipped on Windows: 'sleep' unavailable")
	}

	t.Parallel()

	stdout := &bytes.Buffer{}

	p := pipe.New(pipe.WithStdout(stdout))
	p.Add(pipe.Command("sleep", "10"))

	ctx, cancel := context.WithCancel(context.Background())

	err := p.Start(ctx)
	require.NoError(t, err)

	cancel()

	err = p.Wait()
	assert.ErrorIs(t, err, context.Canceled)
}

// Verify the correct error if a command in the pipeline exits before
// reading all of its predecessor's output. Note that the amount of
// unread output in this case *does fit* within the OS-level pipe
// buffer.
func TestLittleEPIPE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIXME: test skipped on Windows: 'sleep' unavailable")
	}

	t.Parallel()

	p := pipe.New()
	p.Add(
		pipe.Command("sh", "-c", "sleep 1; echo foo"),
		pipe.Command("true"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.Run(ctx)
	assert.EqualError(t, err, "sh: signal: broken pipe")
}

// Verify the correct error if one command in the pipeline exits
// before reading all of its predecessor's output. Note that the
// amount of unread output in this case *does not fit* within the
// OS-level pipe buffer.
func TestBigEPIPE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIXME: test skipped on Windows: 'seq' unavailable")
	}

	t.Parallel()

	p := pipe.New()
	p.Add(
		pipe.Command("seq", "100000"),
		pipe.Command("true"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.Run(ctx)
	assert.EqualError(t, err, "seq: signal: broken pipe")
}

// Verify the correct error if one command in the pipeline exits
// before reading all of its predecessor's output. Note that the
// amount of unread output in this case *does not fit* within the
// OS-level pipe buffer.
func TestIgnoredSIGPIPE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIXME: test skipped on Windows: 'seq' unavailable")
	}

	t.Parallel()

	p := pipe.New()
	p.Add(
		pipe.IgnoreError(pipe.Command("seq", "100000"), pipe.IsSIGPIPE),
		pipe.Command("echo", "foo"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := p.Output(ctx)
	assert.NoError(t, err)
	assert.EqualValues(t, "foo\n", out)
}

func TestFunction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	p.Add(
		pipe.Print("hello world"),
		pipe.Function(
			"farewell",
			func(_ context.Context, _ pipe.Env, stdin io.Reader, stdout io.Writer) error {
				buf, err := ioutil.ReadAll(stdin)
				if err != nil {
					return err
				}
				if string(buf) != "hello world" {
					return fmt.Errorf("expected \"hello world\"; got %q", string(buf))
				}
				_, err = stdout.Write([]byte("goodbye, cruel world"))
				return err
			},
		),
	)

	out, err := p.Output(ctx)
	assert.NoError(t, err)
	assert.EqualValues(t, "goodbye, cruel world", out)
}

func TestPipelineWithFunction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	p.Add(
		pipe.Command("echo", "-n", "hello world"),
		pipe.Function(
			"farewell",
			func(_ context.Context, _ pipe.Env, stdin io.Reader, stdout io.Writer) error {
				buf, err := ioutil.ReadAll(stdin)
				if err != nil {
					return err
				}
				if string(buf) != "hello world" {
					return fmt.Errorf("expected \"hello world\"; got %q", string(buf))
				}
				_, err = stdout.Write([]byte("goodbye, cruel world"))
				return err
			},
		),
		pipe.Command("tr", "a-z", "A-Z"),
	)

	out, err := p.Output(ctx)
	assert.NoError(t, err)
	assert.EqualValues(t, "GOODBYE, CRUEL WORLD", out)
}

type ErrorStartingStage struct {
	err error
}

func (s ErrorStartingStage) Name() string {
	return "errorStartingStage"
}

func (s ErrorStartingStage) Start(
	ctx context.Context, env pipe.Env, stdin io.ReadCloser,
) (io.ReadCloser, error) {
	return ioutil.NopCloser(&bytes.Buffer{}), s.err
}

func (s ErrorStartingStage) Wait() error {
	return nil
}

func seqFunction(n int) pipe.Stage {
	return pipe.Function(
		"seq",
		func(_ context.Context, _ pipe.Env, _ io.Reader, stdout io.Writer) error {
			for i := 1; i <= n; i++ {
				_, err := fmt.Fprintf(stdout, "%d\n", i)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)
}

func TestPipelineWithLinewiseFunction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	// Print the numbers from 1 to 20 (generated from scratch):
	p.Add(
		seqFunction(20),
		// Discard all but the multiples of 5, and emit the results
		// separated by spaces on one line:
		pipe.LinewiseFunction(
			"multiples-of-5",
			func(_ context.Context, _ pipe.Env, line []byte, w *bufio.Writer) error {
				n, err := strconv.Atoi(string(line))
				if err != nil {
					return err
				}
				if n%5 != 0 {
					return nil
				}
				_, err = fmt.Fprintf(w, " %d", n)
				return err
			},
		),
		// Read the words and square them, emitting the results one per
		// line:
		pipe.ScannerFunction(
			"square-multiples-of-5",
			func(r io.Reader) (pipe.Scanner, error) {
				scanner := bufio.NewScanner(r)
				scanner.Split(bufio.ScanWords)
				return scanner, nil
			},
			func(_ context.Context, _ pipe.Env, line []byte, w *bufio.Writer) error {
				n, err := strconv.Atoi(string(line))
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(w, "%d\n", n*n)
				return err
			},
		),
	)

	out, err := p.Output(ctx)
	assert.NoError(t, err)
	assert.EqualValues(t, "25\n100\n225\n400\n", out)
}

func TestScannerAlwaysFlushes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var length int64

	p := pipe.New()
	// Print the numbers from 1 to 20 (generated from scratch):
	p.Add(
		pipe.IgnoreError(
			seqFunction(20),
			pipe.IsPipeError,
		),
		// Pass the numbers through up to 7, then exit with an
		// ignored error:
		pipe.IgnoreError(
			pipe.LinewiseFunction(
				"error-after-7",
				func(_ context.Context, _ pipe.Env, line []byte, w *bufio.Writer) error {
					fmt.Fprintf(w, "%s\n", line)
					if string(line) == "7" {
						return errors.New("ignore")
					}
					return nil
				},
			),
			func(err error) bool {
				return err.Error() == "ignore"
			},
		),
		// Read the numbers and add them into the sum:
		pipe.Function(
			"compute-length",
			func(_ context.Context, _ pipe.Env, stdin io.Reader, _ io.Writer) error {
				var err error
				length, err = io.Copy(ioutil.Discard, stdin)
				return err
			},
		),
	)

	err := p.Run(ctx)
	assert.NoError(t, err)
	// Make sure that all of the bytes emitted before the second
	// stage's error were received by the third stage:
	assert.EqualValues(t, 14, length)
}

func TestScannerFinishEarly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var length int64

	p := pipe.New()
	p.Add(
		// Print the numbers from 1 to 20 (generated from scratch):
		seqFunction(20),

		// Pass the numbers through up to 7, then exit with an ignored
		// error:
		pipe.LinewiseFunction(
			"finish-after-7",
			func(_ context.Context, _ pipe.Env, line []byte, w *bufio.Writer) error {
				fmt.Fprintf(w, "%s\n", line)
				if string(line) == "7" {
					return pipe.FinishEarly
				}
				return nil
			},
		),

		// Read the numbers and add them into the sum:
		pipe.Function(
			"compute-length",
			func(_ context.Context, _ pipe.Env, stdin io.Reader, _ io.Writer) error {
				var err error
				length, err = io.Copy(ioutil.Discard, stdin)
				return err
			},
		),
	)

	err := p.Run(ctx)
	assert.NoError(t, err)
	// Make sure that all of the bytes emitted before the second
	// stage's error were received by the third stage:
	assert.EqualValues(t, 14, length)
}

func TestPrintln(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	p.Add(pipe.Println("Look Ma, no hands!"))
	out, err := p.Output(ctx)
	if assert.NoError(t, err) {
		assert.EqualValues(t, "Look Ma, no hands!\n", out)
	}
}

func TestPrintf(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	p := pipe.New()
	p.Add(pipe.Printf("Strangely recursive: %T", p))
	out, err := p.Output(ctx)
	if assert.NoError(t, err) {
		assert.EqualValues(t, "Strangely recursive: *pipe.Pipeline", out)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	err1 := errors.New("error1")
	err2 := errors.New("error2")

	for _, tc := range []struct {
		name        string
		stages      []pipe.Stage
		expectedErr error
	}{
		{
			name: "no-error",
			stages: []pipe.Stage{
				pipe.Function("noop1", genErr(nil)),
				pipe.Function("noop2", genErr(nil)),
				pipe.Function("noop3", genErr(nil)),
			},
			expectedErr: nil,
		},
		{
			name: "lonely-error",
			stages: []pipe.Stage{
				pipe.Function("err1", genErr(err1)),
			},
			expectedErr: err1,
		},
		{
			name: "error",
			stages: []pipe.Stage{
				pipe.Function("noop1", genErr(nil)),
				pipe.Function("err1", genErr(err1)),
				pipe.Function("noop2", genErr(nil)),
			},
			expectedErr: err1,
		},
		{
			name: "two-consecutive-errors",
			stages: []pipe.Stage{
				pipe.Function("noop1", genErr(nil)),
				pipe.Function("err1", genErr(err1)),
				pipe.Function("err2", genErr(err2)),
				pipe.Function("noop2", genErr(nil)),
			},
			expectedErr: err1,
		},
		{
			name: "pipe-then-error",
			stages: []pipe.Stage{
				pipe.Function("noop1", genErr(nil)),
				pipe.Function("pipe-error", genErr(io.ErrClosedPipe)),
				pipe.Function("err1", genErr(err1)),
				pipe.Function("noop2", genErr(nil)),
			},
			expectedErr: err1,
		},
		{
			name: "error-then-pipe",
			stages: []pipe.Stage{
				pipe.Function("noop1", genErr(nil)),
				pipe.Function("err1", genErr(err1)),
				pipe.Function("pipe-error", genErr(io.ErrClosedPipe)),
				pipe.Function("noop2", genErr(nil)),
			},
			expectedErr: err1,
		},
		{
			name: "two-spaced-errors",
			stages: []pipe.Stage{
				pipe.Function("noop1", genErr(nil)),
				pipe.Function("err1", genErr(err1)),
				pipe.Function("noop2", genErr(nil)),
				pipe.Function("err2", genErr(err2)),
				pipe.Function("noop3", genErr(nil)),
			},
			expectedErr: err1,
		},
		{
			name: "finish-early-ignored",
			stages: []pipe.Stage{
				pipe.Function("noop1", genErr(nil)),
				pipe.Function("finish-early1", genErr(pipe.FinishEarly)),
				pipe.Function("noop2", genErr(nil)),
				pipe.Function("finish-early2", genErr(pipe.FinishEarly)),
				pipe.Function("noop3", genErr(nil)),
			},
			expectedErr: nil,
		},
		{
			name: "error-before-finish-early",
			stages: []pipe.Stage{
				pipe.Function("err1", genErr(err1)),
				pipe.Function("finish-early", genErr(pipe.FinishEarly)),
			},
			expectedErr: err1,
		},
		{
			name: "error-after-finish-early",
			stages: []pipe.Stage{
				pipe.Function("finish-early", genErr(pipe.FinishEarly)),
				pipe.Function("err1", genErr(err1)),
			},
			expectedErr: err1,
		},
		{
			name: "pipe-then-finish-early",
			stages: []pipe.Stage{
				pipe.Function("pipe-error", genErr(io.ErrClosedPipe)),
				pipe.Function("finish-early", genErr(pipe.FinishEarly)),
			},
			expectedErr: nil,
		},
		{
			name: "pipe-then-two-finish-early",
			stages: []pipe.Stage{
				pipe.Function("pipe-error", genErr(io.ErrClosedPipe)),
				pipe.Function("finish-early1", genErr(pipe.FinishEarly)),
				pipe.Function("finish-early2", genErr(pipe.FinishEarly)),
			},
			expectedErr: nil,
		},
		{
			name: "two-pipe-then-finish-early",
			stages: []pipe.Stage{
				pipe.Function("pipe-error1", genErr(io.ErrClosedPipe)),
				pipe.Function("pipe-error2", genErr(io.ErrClosedPipe)),
				pipe.Function("finish-early", genErr(pipe.FinishEarly)),
			},
			expectedErr: nil,
		},
		{
			name: "pipe-then-finish-early-with-gap",
			stages: []pipe.Stage{
				pipe.Function("pipe-error", genErr(io.ErrClosedPipe)),
				pipe.Function("noop", genErr(nil)),
				pipe.Function("finish-early1", genErr(pipe.FinishEarly)),
			},
			expectedErr: io.ErrClosedPipe,
		},
		{
			name: "finish-early-then-pipe",
			stages: []pipe.Stage{
				pipe.Function("finish-early", genErr(pipe.FinishEarly)),
				pipe.Function("pipe-error", genErr(io.ErrClosedPipe)),
			},
			expectedErr: io.ErrClosedPipe,
		},
		{
			name: "error-then-pipe-then-finish-early",
			stages: []pipe.Stage{
				pipe.Function("err1", genErr(err1)),
				pipe.Function("pipe-error", genErr(io.ErrClosedPipe)),
				pipe.Function("finish-early", genErr(pipe.FinishEarly)),
			},
			expectedErr: err1,
		},
		{
			name: "pipe-then-error-then-finish-early",
			stages: []pipe.Stage{
				pipe.Function("pipe-error", genErr(io.ErrClosedPipe)),
				pipe.Function("err1", genErr(err1)),
				pipe.Function("finish-early", genErr(pipe.FinishEarly)),
			},
			expectedErr: err1,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := pipe.New()
			p.Add(tc.stages...)
			err := p.Run(ctx)
			if tc.expectedErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.expectedErr)
			}
		})
	}
}

func BenchmarkSingleProgram(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		p := pipe.New()
		p.Add(
			pipe.Command("true"),
		)
		assert.NoError(b, p.Run(ctx))
	}
}

func BenchmarkTenPrograms(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		p := pipe.New()
		p.Add(
			pipe.Command("echo", "hello world"),
			pipe.Command("cat"),
			pipe.Command("cat"),
			pipe.Command("cat"),
			pipe.Command("cat"),
			pipe.Command("cat"),
			pipe.Command("cat"),
			pipe.Command("cat"),
			pipe.Command("cat"),
			pipe.Command("cat"),
		)
		out, err := p.Output(ctx)
		if assert.NoError(b, err) {
			assert.EqualValues(b, "hello world\n", out)
		}
	}
}

func BenchmarkTenFunctions(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		p := pipe.New()
		p.Add(
			pipe.Println("hello world"),
			pipe.Function("copy1", catFn),
			pipe.Function("copy2", catFn),
			pipe.Function("copy3", catFn),
			pipe.Function("copy4", catFn),
			pipe.Function("copy5", catFn),
			pipe.Function("copy6", catFn),
			pipe.Function("copy7", catFn),
			pipe.Function("copy8", catFn),
			pipe.Function("copy9", catFn),
		)
		out, err := p.Output(ctx)
		if assert.NoError(b, err) {
			assert.EqualValues(b, "hello world\n", out)
		}
	}
}

func BenchmarkTenMixedStages(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		p := pipe.New()
		p.Add(
			pipe.Command("echo", "hello world"),
			pipe.Function("copy1", catFn),
			pipe.Command("cat"),
			pipe.Function("copy2", catFn),
			pipe.Command("cat"),
			pipe.Function("copy3", catFn),
			pipe.Command("cat"),
			pipe.Function("copy4", catFn),
			pipe.Command("cat"),
			pipe.Function("copy5", catFn),
		)
		out, err := p.Output(ctx)
		if assert.NoError(b, err) {
			assert.EqualValues(b, "hello world\n", out)
		}
	}
}

func catFn(_ context.Context, _ pipe.Env, stdin io.Reader, stdout io.Writer) error {
	_, err := io.Copy(stdout, stdin)
	return err
}

func genErr(err error) pipe.StageFunc {
	return func(_ context.Context, _ pipe.Env, _ io.Reader, _ io.Writer) error {
		return err
	}
}
