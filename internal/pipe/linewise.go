package pipe

import (
	"bufio"
	"bytes"
	"context"
	"io"
)

// LinewiseStageFunc is a function that can be embedded in a
// `goStage`. It is called once per line in the input (where "line"
// can be defined via any `bufio.Scanner`). It should process the line
// and may write whatever it likes to `stdout`, which is a buffered
// writer whose contents are forwarded to the input of the next stage
// of the pipeline. The function needn't write one line of output per
// line of input.
//
// The function mustn't retain copies of `line`, since it may be
// overwritten every time the function is called.
//
// The function needn't flush or close `stdout` (this will be done
// automatically when all of the input has been processed).
//
// If there is an error parsing the input into lines, or if this
// function returns an error, then the whole pipeline will be aborted
// with that error. However, if the function returns the special error
// `pipe.FinishEarly`, the stage will stop processing immediately with
// a `nil` error value.
//
// The function will be called in a separate goroutine, so it must be
// careful to synchronize any data access aside from writing to
// `stdout`.
type LinewiseStageFunc func(
	ctx context.Context, env Env, line []byte, stdout *bufio.Writer,
) error

// LinewiseFunction returns a function-based `Stage`. The input will
// be split into LF-terminated lines and passed to the function one
// line at a time (without the LF). The function may emit output to
// its `stdout` argument. See the definition of `LinewiseStageFunc`
// for more information.
//
// Note that the stage will emit an error if any line (including its
// end-of-line terminator) exceeds 64 kiB in length. If this is too
// short, use `ScannerFunction()` directly with your own
// `NewScannerFunc` as argument, or use `Function()` directly with
// your own `StageFunc`.
func LinewiseFunction(name string, f LinewiseStageFunc) Stage {
	return ScannerFunction(
		name,
		func(r io.Reader) (Scanner, error) {
			scanner := bufio.NewScanner(r)
			// Split based on strict LF (we don't accept CRLF):
			scanner.Split(ScanLFTerminatedLines)
			return scanner, nil
		},
		f,
	)
}

// ScanLFTerminatedLines is a `bufio.SplitFunc` that splits its input
// into lines at LF characters (not treating CR specially).
func ScanLFTerminatedLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i != -1 {
		return i + 1, data[0:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
