// +build !isatty

package isatty

func Isatty(fd uintptr) (bool, error) {
	return true, nil
}
