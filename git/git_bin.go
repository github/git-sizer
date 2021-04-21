package git

import (
	"path/filepath"

	"github.com/cli/safeexec"
)

// findGitBin finds the `git` binary in PATH that should be used by
// the rest of `git-sizer`. It uses `safeexec` to find the executable,
// because on Windows, `exec.Cmd` looks not only in PATH, but also in
// the current directory. This is a potential risk if the repository
// being scanned is hostile and non-bare because it might possibly
// contain an executable file named `git`.
func findGitBin() (string, error) {
	gitBin, err := safeexec.LookPath("git")
	if err != nil {
		return "", err
	}

	gitBin, err = filepath.Abs(gitBin)
	if err != nil {
		return "", err
	}

	return gitBin, nil
}
