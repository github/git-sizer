package git

import (
	"path/filepath"
	"sync"

	"github.com/cli/safeexec"
)

// This variable will be used to memoize the result of `findGitBin()`,
// since its return value only depends on the environment.
var gitBinMemo struct {
	once sync.Once

	gitBin string
	err    error
}

// findGitBin finds the `git` binary in PATH that should be used by
// the rest of `git-sizer`. It uses `safeexec` to find the executable,
// because on Windows, `exec.Cmd` looks not only in PATH, but also in
// the current directory. This is a potential risk if the repository
// being scanned is hostile and non-bare because it might possibly
// contain an executable file named `git`.
func findGitBin() (string, error) {
	gitBinMemo.once.Do(func() {
		p, err := safeexec.LookPath("git")
		if err != nil {
			gitBinMemo.err = err
			return
		}

		p, err = filepath.Abs(p)
		if err != nil {
			gitBinMemo.err = err
			return
		}

		gitBinMemo.gitBin = p
	})
	return gitBinMemo.gitBin, gitBinMemo.err
}
