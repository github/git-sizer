package git

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// ObjectType represents the type of a Git object ("blob", "tree",
// "commit", "tag", or "missing").
type ObjectType string

// Repository represents a Git repository on disk.
type Repository struct {
	// gitDir is the path to the `GIT_DIR` for this repository. It
	// might be absolute or it might be relative to the current
	// directory.
	gitDir string

	// gitBin is the path of the `git` executable that should be used
	// when running commands in this repository.
	gitBin string
}

// smartJoin returns `relPath` if it is an absolute path. If not, it
// assumes that `relPath` is relative to `path`, so it joins them
// together and returns the result. In that case, if `path` itself is
// relative, then the return value is also relative.
func smartJoin(path, relPath string) string {
	if filepath.IsAbs(relPath) {
		return relPath
	}
	return filepath.Join(path, relPath)
}

// NewRepositoryFromGitDir creates a new `Repository` object that can
// be used for running `git` commands, given the value of `GIT_DIR`
// for the repository.
func NewRepositoryFromGitDir(gitDir string) (*Repository, error) {
	// Find the `git` executable to be used:
	gitBin, err := findGitBin()
	if err != nil {
		return nil, fmt.Errorf(
			"could not find 'git' executable (is it in your PATH?): %w", err,
		)
	}

	repo := Repository{
		gitDir: gitDir,
		gitBin: gitBin,
	}

	full, err := repo.IsFull()
	if err != nil {
		return nil, fmt.Errorf("determining whether the repository is a full clone: %w", err)
	}
	if !full {
		return nil, errors.New("this appears to be a shallow clone; full clone required")
	}

	return &repo, nil
}

// NewRepository creates a new `Repository` object that can be used
// for running `git` commands within `path`.
func NewRepository(path string) (*Repository, error) {
	// Find the `git` executable to be used:
	gitBin, err := findGitBin()
	if err != nil {
		return nil, fmt.Errorf(
			"could not find 'git' executable (is it in your PATH?): %w", err,
		)
	}

	//nolint:gosec // `gitBin` is chosen carefully, and `path` is the
	// path to the repository.
	cmd := exec.Command(gitBin, "-C", path, "rev-parse", "--git-dir")
	out, err := cmd.Output()
	if err != nil {
		switch err := err.(type) {
		case *exec.Error:
			return nil, fmt.Errorf(
				"could not run '%s': %w", gitBin, err.Err,
			)
		case *exec.ExitError:
			return nil, fmt.Errorf(
				"git rev-parse failed: %s", err.Stderr,
			)
		default:
			return nil, err
		}
	}
	gitDir := smartJoin(path, string(bytes.TrimSpace(out)))

	return NewRepositoryFromGitDir(gitDir)
}

// IsFull returns `true` iff `repo` appears to be a full clone.
func (repo *Repository) IsFull() (bool, error) {
	shallow, err := repo.GitPath("shallow")
	if err != nil {
		return false, err
	}

	_, err = os.Lstat(shallow)
	if err == nil {
		return false, nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}

	// The `shallow` file is absent, which is what we expect
	// for a full clone.
	return true, nil
}

func (repo *Repository) GitCommand(callerArgs ...string) *exec.Cmd {
	args := []string{
		// Disable replace references when running our commands:
		"--no-replace-objects",

		// Disable the warning that grafts are deprecated, since we
		// want to set the grafts file to `/dev/null` below (to
		// disable grafts even where they are supported):
		"-c", "advice.graftFileDeprecated=false",
	}

	args = append(args, callerArgs...)

	//nolint:gosec // `gitBin` is chosen carefully, and the rest of
	// the args have been checked.
	cmd := exec.Command(repo.gitBin, args...)

	cmd.Env = append(
		os.Environ(),
		"GIT_DIR="+repo.gitDir,
		// Disable grafts when running our commands:
		"GIT_GRAFT_FILE="+os.DevNull,
	)

	return cmd
}

// GitDir returns the path to `repo`'s `GIT_DIR`. It might be absolute
// or it might be relative to the current directory.
func (repo *Repository) GitDir() string {
	return repo.gitDir
}

// GitPath returns that path of a file within the git repository, by
// calling `git rev-parse --git-path $relPath`. The returned path is
// relative to the current directory.
func (repo *Repository) GitPath(relPath string) (string, error) {
	cmd := repo.GitCommand("rev-parse", "--git-path", relPath)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf(
			"running 'git rev-parse --git-path %s': %w", relPath, err,
		)
	}
	// `git rev-parse --git-path` is documented to return the path
	// relative to the current directory. Since we haven't changed the
	// current directory, we can use it as-is:
	return string(bytes.TrimSpace(out)), nil
}
