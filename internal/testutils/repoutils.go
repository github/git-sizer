package testutils

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/github/git-sizer/git"
)

// TestRepo represents a git repository used for tests.
type TestRepo struct {
	Path string
}

// NewTestRepo creates and initializes a test repository in a
// temporary directory constructed using `pattern`. The caller must
// delete the repository by calling `repo.Remove()`.
func NewTestRepo(t *testing.T, bare bool, pattern string) *TestRepo {
	t.Helper()

	path, err := ioutil.TempDir("", pattern)
	require.NoError(t, err)

	repo := TestRepo{Path: path}

	repo.Init(t, bare)

	return &TestRepo{
		Path: path,
	}
}

// Init initializes a git repository at `repo.Path`.
func (repo *TestRepo) Init(t *testing.T, bare bool) {
	t.Helper()

	// Don't use `GitCommand()` because the directory might not
	// exist yet:
	var cmd *exec.Cmd
	if bare {
		cmd = exec.Command("git", "init", "--bare", repo.Path)
	} else {
		cmd = exec.Command("git", "init", repo.Path)
	}
	cmd.Env = CleanGitEnv()
	err := cmd.Run()
	require.NoError(t, err)
}

// Remove deletes the test repository at `repo.Path`.
func (repo *TestRepo) Remove(t *testing.T) {
	t.Helper()

	_ = os.RemoveAll(repo.Path)
}

// Clone creates a clone of `repo` at a temporary path constructued
// using `pattern`. The caller is responsible for removing it when
// done by calling `Remove()`.
func (repo *TestRepo) Clone(t *testing.T, pattern string) *TestRepo {
	t.Helper()

	path, err := ioutil.TempDir("", pattern)
	require.NoError(t, err)

	err = repo.GitCommand(
		t, "clone", "--bare", "--mirror", repo.Path, path,
	).Run()
	require.NoError(t, err)

	return &TestRepo{
		Path: path,
	}
}

// Repository returns a `*git.Repository` for `repo`.
func (repo *TestRepo) Repository(t *testing.T) *git.Repository {
	t.Helper()

	r, err := git.NewRepository(repo.Path)
	require.NoError(t, err)
	return r
}

// localEnvVars is a list of the variable names that should be cleared
// to give Git a clean environment.
var localEnvVars = func() map[string]bool {
	m := map[string]bool{
		"HOME":            true,
		"XDG_CONFIG_HOME": true,
	}
	out, err := exec.Command("git", "rev-parse", "--local-env-vars").Output()
	if err != nil {
		return m
	}
	for _, k := range strings.Fields(string(out)) {
		m[k] = true
	}
	return m
}()

// GitEnv returns an appropriate environment for running `git`
// commands without being confused by any existing environment or
// gitconfig.
func CleanGitEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		i := strings.IndexByte(e, '=')
		if i == -1 {
			// This shouldn't happen, but if it does,
			// ignore it.
			continue
		}
		k := e[:i]
		if localEnvVars[k] {
			continue
		}
		env = append(env, e)
	}
	return append(
		env,
		fmt.Sprintf("HOME=%s", os.DevNull),
		fmt.Sprintf("XDG_CONFIG_HOME=%s", os.DevNull),
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

// GitCommand creates an `*exec.Cmd` for running `git` in `repo` with
// the specified arguments.
func (repo *TestRepo) GitCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()

	gitArgs := []string{"-C", repo.Path}
	gitArgs = append(gitArgs, args...)
	cmd := exec.Command("git", gitArgs...)
	cmd.Env = CleanGitEnv()
	return cmd
}

func (repo *TestRepo) UpdateRef(t *testing.T, refname string, oid git.OID) {
	t.Helper()

	var cmd *exec.Cmd

	if oid == git.NullOID {
		cmd = repo.GitCommand(t, "update-ref", "-d", refname)
	} else {
		cmd = repo.GitCommand(t, "update-ref", refname, oid.String())
	}
	require.NoError(t, cmd.Run())
}

// createObject creates a new Git object, of the specified type, in
// the repository at `repoPath`. `writer` is a function that writes
// the object in `git hash-object` input format.
func (repo *TestRepo) CreateObject(
	t *testing.T, otype git.ObjectType, writer func(io.Writer) error,
) git.OID {
	t.Helper()

	cmd := repo.GitCommand(t, "hash-object", "-w", "-t", string(otype), "--stdin")
	in, err := cmd.StdinPipe()
	require.NoError(t, err)

	out, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	require.NoError(t, err)

	err = writer(in)
	err2 := in.Close()
	if err != nil {
		cmd.Wait()
		require.NoError(t, err)
	}
	if err2 != nil {
		cmd.Wait()
		require.NoError(t, err2)
	}

	output, err := ioutil.ReadAll(out)
	err2 = cmd.Wait()
	require.NoError(t, err)
	require.NoError(t, err2)

	oid, err := git.NewOID(string(bytes.TrimSpace(output)))
	require.NoError(t, err)
	return oid
}

// AddFile adds and stages a file in `repo` at path `relativePath`
// with the specified `contents`. This must be run in a non-bare
// repository.
func (repo *TestRepo) AddFile(t *testing.T, relativePath, contents string) {
	t.Helper()

	dirPath := filepath.Dir(relativePath)
	if dirPath != "." {
		require.NoError(
			t,
			os.MkdirAll(filepath.Join(repo.Path, dirPath), 0777),
			"creating subdir",
		)
	}

	filename := filepath.Join(repo.Path, relativePath)
	f, err := os.Create(filename)
	require.NoErrorf(t, err, "creating file %q", filename)
	_, err = f.WriteString(contents)
	require.NoErrorf(t, err, "writing to file %q", filename)
	require.NoErrorf(t, f.Close(), "closing file %q", filename)

	cmd := repo.GitCommand(t, "add", relativePath)
	require.NoErrorf(t, cmd.Run(), "adding file %q", relativePath)
}

// CreateReferencedOrphan creates a simple new orphan commit and
// points the reference with name `refname` at it. This can be run in
// a bare or non-bare repository.
func (repo *TestRepo) CreateReferencedOrphan(t *testing.T, refname string) {
	t.Helper()

	oid := repo.CreateObject(t, "blob", func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "%s\n", refname)
		return err
	})

	oid = repo.CreateObject(t, "tree", func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "100644 a.txt\x00%s", oid.Bytes())
		return err
	})

	oid = repo.CreateObject(t, "commit", func(w io.Writer) error {
		_, err := fmt.Fprintf(
			w,
			"tree %s\n"+
				"author Example <example@example.com> 1112911993 -0700\n"+
				"committer Example <example@example.com> 1112911993 -0700\n"+
				"\n"+
				"Commit for reference %s\n",
			oid, refname,
		)
		return err
	})

	repo.UpdateRef(t, refname, oid)
}

func AddAuthorInfo(cmd *exec.Cmd, timestamp *time.Time) {
	cmd.Env = append(cmd.Env,
		"GIT_AUTHOR_NAME=Arthur",
		"GIT_AUTHOR_EMAIL=arthur@example.com",
		fmt.Sprintf("GIT_AUTHOR_DATE=%d -0700", timestamp.Unix()),
		"GIT_COMMITTER_NAME=Constance",
		"GIT_COMMITTER_EMAIL=constance@example.com",
		fmt.Sprintf("GIT_COMMITTER_DATE=%d -0700", timestamp.Unix()),
	)
	*timestamp = timestamp.Add(60 * time.Second)
}

// ConfigAdd adds a key-value pair to the gitconfig in `repo`.
func (repo *TestRepo) ConfigAdd(t *testing.T, key, value string) {
	t.Helper()

	err := repo.GitCommand(t, "config", "--add", key, value).Run()
	require.NoError(t, err)
}
