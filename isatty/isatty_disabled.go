//go:build !isatty
// +build !isatty

package isatty

// Isatty is a stub implementation of `Isatty()` that always returns `true`.
func Isatty(fd uintptr) (bool, error) {
	return true, nil
}
