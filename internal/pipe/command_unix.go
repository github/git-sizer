//go:build !windows
// +build !windows

package pipe

import (
	"syscall"
	"time"
)

// runInOwnProcessGroup arranges for `cmd` to be run in its own
// process group.
func (s *commandStage) runInOwnProcessGroup() {
	// Put the command in its own process group:
	if s.cmd.SysProcAttr == nil {
		s.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	s.cmd.SysProcAttr.Setpgid = true
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
