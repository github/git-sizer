package pipe

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"

	"golang.org/x/sync/errgroup"
)

// commandStage is a pipeline `Stage` based on running an external
// command and piping the data through its stdin and stdout.
type commandStage struct {
	name   string
	stdin  io.Closer
	cmd    *exec.Cmd
	done   chan struct{}
	wg     errgroup.Group
	stderr bytes.Buffer

	// If the context expired and we attempted to kill the command,
	// `ctx.Err()` is stored here.
	ctxErr atomic.Value
}

// Command returns a pipeline `Stage` based on the specified external
// `command`, run with the given command-line `args`. Its stdin and
// stdout are handled as usual, and its stderr is collected and
// included in any `*exec.ExitError` that the command might emit.
func Command(command string, args ...string) Stage {
	if len(command) == 0 {
		panic("attempt to create command with empty command")
	}

	cmd := exec.Command(command, args...)
	return CommandStage(command, cmd)
}

// Command returns a pipeline `Stage` with the name `name`, based on
// the specified `cmd`. Its stdin and stdout are handled as usual, and
// its stderr is collected and included in any `*exec.ExitError` that
// the command might emit.
func CommandStage(name string, cmd *exec.Cmd) Stage {
	return &commandStage{
		name: name,
		cmd:  cmd,
		done: make(chan struct{}),
	}
}

func (s *commandStage) Name() string {
	return s.name
}

func (s *commandStage) Start(
	ctx context.Context, env Env, stdin io.ReadCloser,
) (io.ReadCloser, error) {
	if s.cmd.Dir == "" {
		s.cmd.Dir = env.Dir
	}

	if stdin != nil {
		s.cmd.Stdin = stdin
		// Also keep a copy so that we can close it when the command exits:
		s.stdin = stdin
	}

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	// If the caller hasn't arranged otherwise, read the command's
	// standard error into our `stderr` field:
	if s.cmd.Stderr == nil {
		// We can't just set `s.cmd.Stderr = &s.stderr`, because if we
		// do then `s.cmd.Wait()` doesn't wait to be sure that all
		// error output has been captured. By doing this ourselves, we
		// can be sure.
		p, err := s.cmd.StderrPipe()
		if err != nil {
			return nil, err
		}
		s.wg.Go(func() error {
			_, err := io.Copy(&s.stderr, p)
			// We don't consider `ErrClosed` an error (FIXME: is this
			// correct?):
			if err != nil && !errors.Is(err, os.ErrClosed) {
				return err
			}
			return nil
		})
	}

	// Put the command in its own process group, if possible:
	s.runInOwnProcessGroup()

	if err := s.cmd.Start(); err != nil {
		return nil, err
	}

	// Arrange for the process to be killed (gently) if the context
	// expires before the command exits normally:
	go func() {
		select {
		case <-ctx.Done():
			s.kill(ctx.Err())
		case <-s.done:
			// Process already done; no need to kill anything.
		}
	}()

	return stdout, nil
}

// filterCmdError interprets `err`, which was returned by `Cmd.Wait()`
// (possibly `nil`), possibly modifying it or ignoring it. It returns
// the error that should actually be returned to the caller (possibly
// `nil`).
func (s *commandStage) filterCmdError(err error) error {
	if err == nil {
		return nil
	}

	eErr, ok := err.(*exec.ExitError)
	if !ok {
		return err
	}

	ctxErr, ok := s.ctxErr.Load().(error)
	if ok {
		// If the process looks like it was killed by us, substitute
		// `ctxErr` for the process's own exit error. Note that this
		// doesn't do anything on Windows, where the `Signaled()`
		// method isn't implemented (it is hardcoded to return
		// `false`).
		ps, ok := eErr.ProcessState.Sys().(syscall.WaitStatus)
		if ok && ps.Signaled() &&
			(ps.Signal() == syscall.SIGTERM || ps.Signal() == syscall.SIGKILL) {
			return ctxErr
		}
	}

	eErr.Stderr = s.stderr.Bytes()
	return eErr
}

func (s *commandStage) Wait() error {
	defer close(s.done)

	// Make sure that any stderr is copied before `s.cmd.Wait()`
	// closes the read end of the pipe:
	wErr := s.wg.Wait()

	err := s.cmd.Wait()
	err = s.filterCmdError(err)

	if err == nil && wErr != nil {
		err = wErr
	}

	if s.stdin != nil {
		cErr := s.stdin.Close()
		if cErr != nil && err == nil {
			return cErr
		}
	}

	return err
}
