package pipe

import (
	"errors"
	"io"
	"os/exec"
	"syscall"
)

// ErrorFilter is a function that can filter errors from
// `Stage.Wait()`. The original error (possibly nil) is passed in as
// an argument, and whatever the function returns is the error
// (possibly nil) that is actually emitted.
type ErrorFilter func(err error) error

func FilterError(s Stage, filter ErrorFilter) Stage {
	return efStage{Stage: s, filter: filter}
}

type efStage struct {
	Stage
	filter ErrorFilter
}

func (s efStage) Wait() error {
	return s.filter(s.Stage.Wait())
}

// ErrorMatcher decides whether its argument matches some class of
// errors (e.g., errors that we want to ignore). The function will
// only be invoked for non-nil errors.
type ErrorMatcher func(err error) bool

// IgnoreError creates a stage that acts like `s` except that it
// ignores any errors that are matched by `em`. Use like
//
//     p.Add(pipe.IgnoreError(
//         someStage,
//         func(err error) bool {
//             var myError *MyErrorType
//             return errors.As(err, &myError) && myError.foo == 42
//         },
//     )
//
// The second argument can also be one of the `ErrorMatcher`s that are
// provided by this package (e.g., `IsError(target)`,
// IsSignal(signal), `IsSIGPIPE`, `IsEPIPE`, `IsPipeError`), or one of
// the functions from the standard library that has the same signature
// (e.g., `os.IsTimeout`), or some combination of these (e.g.,
// `AnyError(IsSIGPIPE, os.IsTimeout)`).
func IgnoreError(s Stage, em ErrorMatcher) Stage {
	return FilterError(s,
		func(err error) error {
			if err == nil || em(err) {
				return nil
			}
			return err
		},
	)
}

// AnyError returns an `ErrorMatcher` that returns true for an error
// that matches any of the `ems`.
func AnyError(ems ...ErrorMatcher) ErrorMatcher {
	return func(err error) bool {
		if err == nil {
			return false
		}
		for _, em := range ems {
			if em(err) {
				return true
			}
		}
		return false
	}
}

// IsError returns an ErrorIdentifier for the specified target error,
// matched using `errors.Is()`. Use like
//
//     p.Add(pipe.IgnoreError(someStage, IsError(io.EOF)))
func IsError(target error) ErrorMatcher {
	return func(err error) bool {
		return errors.Is(err, target)
	}
}

// IsSIGPIPE returns an `ErrorMatcher` that matches `*exec.ExitError`s
// that were caused by the specified signal. The match for
// `*exec.ExitError`s uses `errors.As()`. Note that under Windows this
// always returns false, because on that platform
// `WaitStatus.Signaled()` isn't implemented (it is hardcoded to
// return `false`).
func IsSignal(signal syscall.Signal) ErrorMatcher {
	return func(err error) bool {
		var eErr *exec.ExitError

		if !errors.As(err, &eErr) {
			return false
		}

		status, ok := eErr.Sys().(syscall.WaitStatus)
		return ok && status.Signaled() && status.Signal() == signal
	}
}

var (
	// IsSIGPIPE is an `ErrorMatcher` that matches `*exec.ExitError`s
	// that were caused by SIGPIPE. The match for `*exec.ExitError`s
	// uses `errors.As()`. Use like
	//
	//     p.Add(IgnoreError(someStage, IsSIGPIPE))
	IsSIGPIPE = IsSignal(syscall.SIGPIPE)

	// IsEPIPE is an `ErrorMatcher` that matches `syscall.EPIPE` using
	// `errors.Is()`. Use like
	//
	//     p.Add(IgnoreError(someStage, IsEPIPE))
	IsEPIPE = IsError(syscall.EPIPE)

	// IsErrClosedPipe is an `ErrorMatcher` that matches
	// `io.ErrClosedPipe` using `errors.Is()`. (`io.ErrClosedPipe` is
	// the error that results from writing to a closed
	// `*io.PipeWriter`.) Use like
	//
	//     p.Add(IgnoreError(someStage, IsErrClosedPipe))
	IsErrClosedPipe = IsError(io.ErrClosedPipe)

	// IsPipeError is an `ErrorMatcher` that matches a few different
	// errors that typically result if a stage writes to a subsequent
	// stage that has stopped reading from its stdin. Use like
	//
	//     p.Add(IgnoreError(someStage, IsPipeError))
	IsPipeError = AnyError(IsSIGPIPE, IsEPIPE, IsErrClosedPipe)
)
