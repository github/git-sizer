package sizes

import (
	"os"
	"path/filepath"

	"github.com/github/git-sizer/counts"
)

// CalculateGitDirSize returns the total size in bytes of the .git directory.
func CalculateGitDirSize(gitDir string) (counts.Count64, error) {
	var totalSize counts.Count64

	err := filepath.Walk(gitDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Only skip errors for files we cannot access.
			if os.IsNotExist(err) || os.IsPermission(err) {
				return nil
			}
			return err
		}

		// Only count files, not directories.
		if !info.IsDir() {
			totalSize.Increment(counts.Count64(info.Size()))
		}
		return nil
	})

	return totalSize, err
}
