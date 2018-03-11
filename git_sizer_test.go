package main_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/github/git-sizer/counts"
	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/sizes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Smoke test that the program runs.
func TestExec(t *testing.T) {
	cmd := exec.Command("bin/git-sizer")
	output, err := cmd.CombinedOutput()
	assert.NoErrorf(t, err, "command failed; output: %#v", string(output))
}

func gitCommand(t *testing.T, repo *git.Repository, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_DIR="+repo.Path())
	return cmd
}

func addAuthorInfo(cmd *exec.Cmd, timestamp *time.Time) {
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

func newGitBomb(
	repoName string, depth, breadth int, body string,
) (repo *git.Repository, err error) {
	path, err := ioutil.TempDir("", repoName)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			os.RemoveAll(path)
		}
	}()

	cmd := exec.Command("git", "init", "--bare", path)
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	repo, err = git.NewRepository(path)
	if err != nil {
		return nil, err
	}

	oid, err := repo.CreateObject("blob", func(w io.Writer) error {
		_, err := io.WriteString(w, body)
		return err
	})

	digits := len(fmt.Sprintf("%d", breadth-1))

	mode := "100644"
	prefix := "f"

	for ; depth > 0; depth-- {
		oid, err = repo.CreateObject("tree", func(w io.Writer) error {
			for i := 0; i < breadth; i++ {
				_, err = fmt.Fprintf(
					w, "%s %s%0*d\x00%s",
					mode, prefix, digits, i, oid.Bytes(),
				)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		mode = "40000"
		prefix = "d"
	}

	oid, err = repo.CreateObject("commit", func(w io.Writer) error {
		_, err := fmt.Fprintf(
			w,
			"tree %s\n"+
				"author Example <example@example.com> 1112911993 -0700\n"+
				"committer Example <example@example.com> 1112911993 -0700\n"+
				"\n"+
				"Mwahahaha!\n",
			oid,
		)
		return err
	})
	if err != nil {
		return nil, err
	}

	err = repo.UpdateRef("refs/heads/master", oid)
	if err != nil {
		return nil, err
	}

	return repo, nil
}

func pow(x uint64, n int) uint64 {
	p := uint64(1)
	for ; n > 0; n-- {
		p *= x
	}
	return p
}

func TestBomb(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	repo, err := newGitBomb("bomb", 10, 10, "boom!\n")
	if err != nil {
		t.Errorf("failed to create bomb: %s", err)
	}
	defer os.RemoveAll(repo.Path())

	h, err := sizes.ScanRepositoryUsingGraph(
		repo, git.AllReferencesFilter, sizes.NameStyleNone, false,
	)
	if !assert.NoError(err) {
		return
	}

	assert.Equal(counts.Count32(1), h.UniqueCommitCount, "unique commit count")
	assert.Equal(counts.Count64(169), h.UniqueCommitSize, "unique commit size")
	assert.Equal(counts.Count32(169), h.MaxCommitSize, "max commit size")
	assert.Equal(counts.Count32(1), h.MaxHistoryDepth, "max history depth")
	assert.Equal(counts.Count32(0), h.MaxParentCount, "max parent count")

	assert.Equal(counts.Count32(10), h.UniqueTreeCount, "unique tree count")
	assert.Equal(counts.Count64(2910), h.UniqueTreeSize, "unique tree size")
	assert.Equal(counts.Count64(100), h.UniqueTreeEntries, "unique tree entries")
	assert.Equal(counts.Count32(10), h.MaxTreeEntries, "max tree entries")

	assert.Equal(counts.Count32(1), h.UniqueBlobCount, "unique blob count")
	assert.Equal(counts.Count64(6), h.UniqueBlobSize, "unique blob size")
	assert.Equal(counts.Count32(6), h.MaxBlobSize, "max blob size")

	assert.Equal(counts.Count32(0), h.UniqueTagCount, "unique tag count")
	assert.Equal(counts.Count32(0), h.MaxTagDepth, "max tag depth")

	assert.Equal(counts.Count32(1), h.ReferenceCount, "reference count")

	assert.Equal(counts.Count32(11), h.MaxPathDepth, "max path depth")
	assert.Equal(counts.Count32(29), h.MaxPathLength, "max path length")

	assert.Equal(counts.Count32((pow(10, 10)-1)/(10-1)), h.MaxExpandedTreeCount, "max expanded tree count")
	assert.Equal(counts.Count32(0xffffffff), h.MaxExpandedBlobCount, "max expanded blob count")
	assert.Equal(counts.Count64(6*pow(10, 10)), h.MaxExpandedBlobSize, "max expanded blob size")
	assert.Equal(counts.Count32(0), h.MaxExpandedLinkCount, "max expanded link count")
	assert.Equal(counts.Count32(0), h.MaxExpandedSubmoduleCount, "max expanded submodule count")
}

func TestTaggedTags(t *testing.T) {
	t.Parallel()
	path, err := ioutil.TempDir("", "tagged-tags")
	require.NoError(t, err, "creating temporary directory")

	defer func() {
		os.RemoveAll(path)
	}()

	cmd := exec.Command("git", "init", path)
	require.NoError(t, cmd.Run(), "initializing repo")
	repo, err := git.NewRepository(path)
	require.NoError(t, err, "initializing Repository object")

	timestamp := time.Unix(1112911993, 0)

	cmd = gitCommand(t, repo, "commit", "-m", "initial", "--allow-empty")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating commit")

	// The lexicographical order of these tags is important, hence
	// their strange names.
	cmd = gitCommand(t, repo, "tag", "-m", "tag 1", "tag", "master")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 1")

	cmd = gitCommand(t, repo, "tag", "-m", "tag 2", "bag", "tag")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 2")

	cmd = gitCommand(t, repo, "tag", "-m", "tag 3", "wag", "bag")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 3")

	h, err := sizes.ScanRepositoryUsingGraph(
		repo, git.AllReferencesFilter, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(3), h.MaxTagDepth, "tag depth")
}
