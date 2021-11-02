package pipe

import (
	"context"
	"fmt"
	"io"
)

func Print(a ...interface{}) Stage {
	return Function(
		"print",
		func(_ context.Context, _ Env, _ io.Reader, stdout io.Writer) error {
			_, err := fmt.Fprint(stdout, a...)
			return err
		},
	)
}

func Println(a ...interface{}) Stage {
	return Function(
		"println",
		func(_ context.Context, _ Env, _ io.Reader, stdout io.Writer) error {
			_, err := fmt.Fprintln(stdout, a...)
			return err
		},
	)
}

func Printf(format string, a ...interface{}) Stage {
	return Function(
		"printf",
		func(_ context.Context, _ Env, _ io.Reader, stdout io.Writer) error {
			_, err := fmt.Fprintf(stdout, format, a...)
			return err
		},
	)
}
