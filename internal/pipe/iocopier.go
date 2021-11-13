package pipe

import (
	"context"
	"errors"
	"io"
	"os"
)

// ioCopier is a stage that copies its stdin to a specified
// `io.Writer`. It generates no stdout itself.
type ioCopier struct {
	w    io.WriteCloser
	done chan struct{}
	err  error
}

func newIOCopier(w io.WriteCloser) *ioCopier {
	return &ioCopier{
		w:    w,
		done: make(chan struct{}),
	}
}

func (s *ioCopier) Name() string {
	return "ioCopier"
}

// This method always returns `nil, nil`.
func (s *ioCopier) Start(ctx context.Context, _ Env, r io.ReadCloser) (io.ReadCloser, error) {
	go func() {
		_, err := io.Copy(s.w, r)
		// We don't consider `ErrClosed` an error (FIXME: is this
		// correct?):
		if err != nil && !errors.Is(err, os.ErrClosed) {
			s.err = err
		}
		if err := r.Close(); err != nil && s.err == nil {
			s.err = err
		}
		if err := s.w.Close(); err != nil && s.err == nil {
			s.err = err
		}
		close(s.done)
	}()

	// FIXME: if `s.w.Write()` is blocking (e.g., because there is a
	// downstream process that is not reading from the other side),
	// there's no way to terminate the copy when the context expires.
	// This is not too bad, because the `io.Copy()` call will exit by
	// itself when its input is closed.
	//
	// We could, however, be smarter about exiting more quickly if the
	// context expires but `s.w.Write()` is not blocking.

	return nil, nil
}

func (s *ioCopier) Wait() error {
	<-s.done
	return s.err
}
