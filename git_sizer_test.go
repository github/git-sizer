package main_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/github/git-sizer/counts"
	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/sizes"

	"github.com/stretchr/testify/assert"
)

// Smoke test that the program runs.
func TestExec(t *testing.T) {
	cmd := exec.Command("bin/git-sizer")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("command failed (%s); output: %#v", err, string(output))
	}
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
