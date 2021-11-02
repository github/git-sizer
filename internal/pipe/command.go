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
	"time"

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

	// Put the command in its own process group:
	if s.cmd.SysProcAttr == nil {
		s.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	s.cmd.SysProcAttr.Setpgid = true

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

// kill is called to kill the process if the context expires. `err` is
// the corresponding value of `Context.Err()`.
func (s *commandStage) kill(err error) {
	// I believe that the calls to `syscall.Kill()` in this method are
	// racy. It could be that s.cmd.Wait() succeeds immediately before
	// this call, in which case the process group wouldn't exist
	// anymore. But I don't see any way to avoid this without
	// duplicating a lot of code from `exec.Cmd`. (`os.Cmd.Kill()` and
	// `os.Cmd.Signal()` appear to be race-free, but only because they
	// use internal synchronization. But those methods only kill the
	// process, not the process group, so they are not suitable here.

	// We started the process with PGID == PID:
	pid := s.cmd.Process.Pid
	select {
	case <-s.done:
		// Process has ended; no need to kill it again.
		return
	default:
	}

	// Record the `ctx.Err()`, which will be used as the error result
	// for this stage.
	s.ctxErr.Store(err)

	// First try to kill using a relatively gentle signal so that
	// the processes have a chance to clean up after themselves:
	_ = syscall.Kill(-pid, syscall.SIGTERM)

	// Well-behaved processes should commit suicide after the above,
	// but if they don't exit within 2s, murder the whole lot of them:
	go func() {
		// Use an explicit `time.Timer` rather than `time.After()` so
		// that we can stop it (freeing resources) promptly if the
		// command exits before the timer triggers.
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()

		select {
		case <-s.done:
			// Process has ended; no need to kill it again.
		case <-timer.C:
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}()
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
		// `ctxErr` for the process's own exit error.
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
