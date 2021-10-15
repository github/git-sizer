package testutils

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/github/git-sizer/git"
)

func NewRepository(t *testing.T, repoPath string) *git.Repository {
	t.Helper()

	repo, err := git.NewRepository(repoPath)
	require.NoError(t, err)
	return repo
}

func GitCommand(t *testing.T, repoPath string, args ...string) *exec.Cmd {
	t.Helper()

	gitArgs := []string{"-C", repoPath}
	gitArgs = append(gitArgs, args...)
	return exec.Command("git", gitArgs...)
}

func UpdateRef(t *testing.T, repoPath string, refname string, oid git.OID) {
	t.Helper()

	var cmd *exec.Cmd

	if oid == git.NullOID {
		cmd = GitCommand(t, repoPath, "update-ref", "-d", refname)
	} else {
		cmd = GitCommand(t, repoPath, "update-ref", refname, oid.String())
	}
	require.NoError(t, cmd.Run())
}

// createObject creates a new Git object, of the specified type, in
// the repository at `repoPath`. `writer` is a function that writes
// the object in `git hash-object` input format. This is used for
// testing only.
func CreateObject(
	t *testing.T, repoPath string, otype git.ObjectType, writer func(io.Writer) error,
) git.OID {
	t.Helper()

	cmd := GitCommand(t, repoPath, "hash-object", "-w", "-t", string(otype), "--stdin")
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

func AddFile(t *testing.T, repoPath string, relativePath, contents string) {
	t.Helper()

	dirPath := filepath.Dir(relativePath)
	if dirPath != "." {
		require.NoError(t, os.MkdirAll(filepath.Join(repoPath, dirPath), 0777), "creating subdir")
	}

	filename := filepath.Join(repoPath, relativePath)
	f, err := os.Create(filename)
	require.NoErrorf(t, err, "creating file %q", filename)
	_, err = f.WriteString(contents)
	require.NoErrorf(t, err, "writing to file %q", filename)
	require.NoErrorf(t, f.Close(), "closing file %q", filename)

	cmd := GitCommand(t, repoPath, "add", relativePath)
	require.NoErrorf(t, cmd.Run(), "adding file %q", relativePath)
}

func CreateReferencedOrphan(t *testing.T, repoPath string, refname string) {
	t.Helper()

	oid := CreateObject(t, repoPath, "blob", func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "%s\n", refname)
		return err
	})

	oid = CreateObject(t, repoPath, "tree", func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "100644 a.txt\x00%s", oid.Bytes())
		return err
	})

	oid = CreateObject(t, repoPath, "commit", func(w io.Writer) error {
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

	UpdateRef(t, repoPath, refname, oid)
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

// ConfigAdd adds a key-value pair to the gitconfig in the repository
// at `repoPath`.
func ConfigAdd(t *testing.T, repoPath string, key, value string) {
	t.Helper()

	err := GitCommand(t, repoPath, "config", "--add", key, value).Run()
	require.NoError(t, err)
}
