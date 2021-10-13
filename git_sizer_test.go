package main_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/git-sizer/counts"
	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/internal/testutils"
	"github.com/github/git-sizer/sizes"
)

// Smoke test that the program runs.
func TestExec(t *testing.T) {
	cmd := exec.Command("bin/git-sizer")
	output, err := cmd.CombinedOutput()
	assert.NoErrorf(t, err, "command failed; output: %#v", string(output))
}

func newGitBomb(t *testing.T, repo *testutils.TestRepo, depth, breadth int, body string) {
	t.Helper()

	oid := repo.CreateObject(t, "blob", func(w io.Writer) error {
		_, err := io.WriteString(w, body)
		return err
	})

	digits := len(fmt.Sprintf("%d", breadth-1))

	mode := "100644"
	prefix := "f"

	for ; depth > 0; depth-- {
		oid = repo.CreateObject(t, "tree", func(w io.Writer) error {
			for i := 0; i < breadth; i++ {
				_, err := fmt.Fprintf(
					w, "%s %s%0*d\x00%s",
					mode, prefix, digits, i, oid.Bytes(),
				)
				if err != nil {
					return err
				}
			}
			return nil
		})

		mode = "40000"
		prefix = "d"
	}

	oid = repo.CreateObject(t, "commit", func(w io.Writer) error {
		_, err := fmt.Fprintf(
			w,
			"tree %s\n"+
				"author Example <example@example.com> 1112911993 -0700\n"+
				"committer Example <example@example.com> 1112911993 -0700\n"+
				"\n"+
				"Test git bomb\n",
			oid,
		)
		return err
	})

	repo.UpdateRef(t, "refs/heads/master", oid)
}

// TestRefSelections tests various combinations of reference selection
// options.
func TestRefSelections(t *testing.T) {
	t.Parallel()

	references := []struct {
		// The plusses and spaces in the `results` string correspond
		// to the expected results for one of the tests: `results[i]`
		// tells whether we expect `refname` to be included ('+') or
		// excluded (' ') in test case number `i`.
		results string

		refname string
	}{
		//          111111111
		//0123456789012345678
		{"+ + + + + + +   + +", "refs/barfoo"},
		{"+ + + + + + +++    ", "refs/foo"},
		{"+ + + + + + +   + +", "refs/foobar"},
		{"++  + + + +++   +++", "refs/heads/foo"},
		{"++  + + + ++    +++", "refs/heads/master"},
		{"+ + + ++  +        ", "refs/notes/discussion"},
		{"+ + ++  + +        ", "refs/remotes/origin/master"},
		{"+ + ++  + + +   + +", "refs/remotes/upstream/foo"},
		{"+ + ++  + +        ", "refs/remotes/upstream/master"},
		{"+ + + + ++         ", "refs/stash"},
		{"+ ++  + + +++   + +", "refs/tags/foolish"},
		{"+ ++  + + ++    + +", "refs/tags/other"},
		{"+ ++  + + ++   +   ", "refs/tags/release-1"},
		{"+ ++  + + ++   +   ", "refs/tags/release-2"},
	}

	// computeExpectations assembles and returns the results expected
	// for test `i` from the `references` slice.
	computeExpectations := func(i int) (string, int) {
		var sb strings.Builder
		fmt.Fprintln(&sb, "References (included references marked with '+'):")
		count := 0
		for _, p := range references {
			present := p.results[i]
			fmt.Fprintf(&sb, "%c %s\n", present, p.refname)
			if present == '+' {
				count++
			}
		}
		return sb.String(), count
	}

	// Create a test repo with one orphan commit per refname:
	repo := testutils.NewTestRepo(t, true, "ref-selection")
	defer repo.Remove(t)

	for _, p := range references {
		repo.CreateReferencedOrphan(t, p.refname)
	}

	executable, err := exec.LookPath("bin/git-sizer")
	require.NoError(t, err)
	executable, err = filepath.Abs(executable)
	require.NoError(t, err)

	for i, p := range []struct {
		name   string
		args   []string
		config []git.ConfigEntry
	}{
		{ // 0
			name: "no arguments",
		},
		{ // 1
			name: "branches",
			args: []string{"--branches"},
		},
		{ // 2
			name: "no branches",
			args: []string{"--no-branches"},
		},
		{ // 3
			name: "tags",
			args: []string{"--tags"},
		},
		{ // 4
			name: "no tags",
			args: []string{"--no-tags"},
		},
		{ // 5
			name: "remotes",
			args: []string{"--remotes"},
		},
		{ // 6
			name: "no remotes",
			args: []string{"--no-remotes"},
		},
		{ // 7
			name: "notes",
			args: []string{"--notes"},
		},
		{ // 8
			name: "no notes",
			args: []string{"--no-notes"},
		},
		{ // 9
			name: "stash",
			args: []string{"--stash"},
		},
		{ // 10
			name: "no stash",
			args: []string{"--no-stash"},
		},
		{ // 11
			name: "branches and tags",
			args: []string{"--branches", "--tags"},
		},
		{ // 12
			name: "foo",
			args: []string{"--include-regexp", ".*foo.*"},
		},
		{ // 13
			name: "refs/foo as prefix",
			args: []string{"--include", "refs/foo"},
		},
		{ // 14
			name: "refs/foo as regexp",
			args: []string{"--include-regexp", "refs/foo"},
		},
		{ // 15
			name: "release tags",
			args: []string{"--include-regexp", "refs/tags/release-.*"},
		},
		{ // 16
			name: "combination",
			args: []string{
				"--include=refs/heads",
				"--tags",
				"--exclude", "refs/heads/foo",
				"--include-regexp", ".*foo.*",
				"--exclude", "refs/foo",
				"--exclude-regexp", "refs/tags/release-.*",
			},
		},
		{ // 17
			name: "branches-refgroup",
			args: []string{"--refgroup=mygroup"},
			config: []git.ConfigEntry{
				{"refgroup.mygroup.include", "refs/heads"},
			},
		},
		{ // 18
			name: "combination-refgroup",
			args: []string{"--refgroup=mygroup"},
			config: []git.ConfigEntry{
				{"refgroup.mygroup.include", "refs/heads"},
				{"refgroup.mygroup.include", "refs/tags"},
				{"refgroup.mygroup.exclude", "refs/heads/foo"},
				{"refgroup.mygroup.includeRegexp", ".*foo.*"},
				{"refgroup.mygroup.exclude", "refs/foo"},
				{"refgroup.mygroup.excludeRegexp", "refs/tags/release-.*"},
			},
		},
	} {
		t.Run(
			p.name,
			func(t *testing.T) {
				repo := repo.Clone(t, "ref-selection")
				defer repo.Remove(t)

				for _, e := range p.config {
					repo.ConfigAdd(t, e.Key, e.Value)
				}

				args := []string{"--show-refs", "--no-progress", "--json", "--json-version=2"}
				args = append(args, p.args...)
				cmd := exec.Command(executable, args...)
				cmd.Dir = repo.Path
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				err = cmd.Run()
				assert.NoError(t, err)

				expectedStderr, expectedUniqueCommitCount := computeExpectations(i)

				// Make sure that the right number of commits was scanned:
				var v struct {
					UniqueCommitCount struct {
						Value int
					}
				}
				err = json.Unmarshal(stdout.Bytes(), &v)
				if assert.NoError(t, err) {
					assert.EqualValues(t, expectedUniqueCommitCount, v.UniqueCommitCount.Value)
				}

				// Make sure that the right references were reported scanned:
				assert.Equal(t, expectedStderr, stderr.String())
			},
		)
	}
}

func pow(x uint64, n int) uint64 {
	p := uint64(1)
	for ; n > 0; n-- {
		p *= x
	}
	return p
}

type refGrouper struct{}

func (rg refGrouper) Categorize(refname string) (bool, []sizes.RefGroupSymbol) {
	return true, nil
}

func (rg refGrouper) Groups() []sizes.RefGroup {
	return nil
}

func TestBomb(t *testing.T) {
	t.Parallel()

	repo := testutils.NewTestRepo(t, true, "bomb")
	defer repo.Remove(t)

	newGitBomb(t, repo, 10, 10, "boom!\n")

	h, err := sizes.ScanRepositoryUsingGraph(
		repo.Repository(t),
		refGrouper{}, sizes.NameStyleFull, false,
	)
	require.NoError(t, err)

	assert.Equal(t, counts.Count32(1), h.UniqueCommitCount, "unique commit count")
	assert.Equal(t, counts.Count64(172), h.UniqueCommitSize, "unique commit size")
	assert.Equal(t, counts.Count32(172), h.MaxCommitSize, "max commit size")
	assert.Equal(t, "refs/heads/master", h.MaxCommitSizeCommit.Path(), "max commit size commit")
	assert.Equal(t, counts.Count32(1), h.MaxHistoryDepth, "max history depth")
	assert.Equal(t, counts.Count32(0), h.MaxParentCount, "max parent count")
	assert.Equal(t, "refs/heads/master", h.MaxParentCountCommit.Path(), "max parent count commit")

	assert.Equal(t, counts.Count32(10), h.UniqueTreeCount, "unique tree count")
	assert.Equal(t, counts.Count64(2910), h.UniqueTreeSize, "unique tree size")
	assert.Equal(t, counts.Count64(100), h.UniqueTreeEntries, "unique tree entries")
	assert.Equal(t, counts.Count32(10), h.MaxTreeEntries, "max tree entries")
	assert.Equal(t, "refs/heads/master:d0/d0/d0/d0/d0/d0/d0/d0/d0", h.MaxTreeEntriesTree.Path(), "max tree entries tree")

	assert.Equal(t, counts.Count32(1), h.UniqueBlobCount, "unique blob count")
	assert.Equal(t, counts.Count64(6), h.UniqueBlobSize, "unique blob size")
	assert.Equal(t, counts.Count32(6), h.MaxBlobSize, "max blob size")
	assert.Equal(t, "refs/heads/master:d0/d0/d0/d0/d0/d0/d0/d0/d0/f0", h.MaxBlobSizeBlob.Path(), "max blob size blob")

	assert.Equal(t, counts.Count32(0), h.UniqueTagCount, "unique tag count")
	assert.Equal(t, counts.Count32(0), h.MaxTagDepth, "max tag depth")

	assert.Equal(t, counts.Count32(1), h.ReferenceCount, "reference count")

	assert.Equal(t, counts.Count32(10), h.MaxPathDepth, "max path depth")
	assert.Equal(t, "refs/heads/master^{tree}", h.MaxPathDepthTree.Path(), "max path depth tree")
	assert.Equal(t, counts.Count32(29), h.MaxPathLength, "max path length")
	assert.Equal(t, "refs/heads/master^{tree}", h.MaxPathLengthTree.Path(), "max path length tree")

	assert.Equal(t, counts.Count32((pow(10, 10)-1)/(10-1)), h.MaxExpandedTreeCount, "max expanded tree count")
	assert.Equal(t, "refs/heads/master^{tree}", h.MaxExpandedTreeCountTree.Path(), "max expanded tree count tree")
	assert.Equal(t, counts.Count32(0xffffffff), h.MaxExpandedBlobCount, "max expanded blob count")
	assert.Equal(t, "refs/heads/master^{tree}", h.MaxExpandedBlobCountTree.Path(), "max expanded blob count tree")
	assert.Equal(t, counts.Count64(6*pow(10, 10)), h.MaxExpandedBlobSize, "max expanded blob size")
	assert.Equal(t, "refs/heads/master^{tree}", h.MaxExpandedBlobSizeTree.Path(), "max expanded blob size tree")
	assert.Equal(t, counts.Count32(0), h.MaxExpandedLinkCount, "max expanded link count")
	assert.Nil(t, h.MaxExpandedLinkCountTree, "max expanded link count tree")
	assert.Equal(t, counts.Count32(0), h.MaxExpandedSubmoduleCount, "max expanded submodule count")
	assert.Nil(t, h.MaxExpandedSubmoduleCountTree, "max expanded submodule count tree")
}

func TestTaggedTags(t *testing.T) {
	t.Parallel()

	repo := testutils.NewTestRepo(t, false, "tagged-tags")
	defer repo.Remove(t)

	timestamp := time.Unix(1112911993, 0)

	cmd := repo.GitCommand(t, "commit", "-m", "initial", "--allow-empty")
	testutils.AddAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating commit")

	// The lexicographical order of these tags is important, hence
	// their strange names.
	cmd = repo.GitCommand(t, "tag", "-m", "tag 1", "tag", "master")
	testutils.AddAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 1")

	cmd = repo.GitCommand(t, "tag", "-m", "tag 2", "bag", "tag")
	testutils.AddAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 2")

	cmd = repo.GitCommand(t, "tag", "-m", "tag 3", "wag", "bag")
	testutils.AddAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 3")

	h, err := sizes.ScanRepositoryUsingGraph(
		repo.Repository(t),
		refGrouper{}, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(3), h.MaxTagDepth, "tag depth")
}

func TestFromSubdir(t *testing.T) {
	t.Parallel()

	repo := testutils.NewTestRepo(t, false, "subdir")
	defer repo.Remove(t)

	timestamp := time.Unix(1112911993, 0)

	repo.AddFile(t, "subdir/file.txt", "Hello, world!\n")

	cmd := repo.GitCommand(t, "commit", "-m", "initial")
	testutils.AddAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating commit")

	h, err := sizes.ScanRepositoryUsingGraph(
		repo.Repository(t),
		refGrouper{}, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(2), h.MaxPathDepth, "max path depth")
}

func TestSubmodule(t *testing.T) {
	t.Parallel()

	tmp, err := ioutil.TempDir("", "submodule")
	require.NoError(t, err, "creating temporary directory")

	defer func() {
		os.RemoveAll(tmp)
	}()

	timestamp := time.Unix(1112911993, 0)

	submRepo := testutils.TestRepo{
		Path: filepath.Join(tmp, "subm"),
	}
	submRepo.Init(t, false)
	submRepo.AddFile(t, "submfile1.txt", "Hello, submodule!\n")
	submRepo.AddFile(t, "submfile2.txt", "Hello again, submodule!\n")
	submRepo.AddFile(t, "submfile3.txt", "Hello again, submodule!\n")

	cmd := submRepo.GitCommand(t, "commit", "-m", "subm initial")
	testutils.AddAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating subm commit")

	mainRepo := testutils.TestRepo{
		Path: filepath.Join(tmp, "main"),
	}
	mainRepo.Init(t, false)

	mainRepo.AddFile(t, "mainfile.txt", "Hello, main!\n")

	cmd = mainRepo.GitCommand(t, "commit", "-m", "main initial")
	testutils.AddAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating main commit")

	// Make subm a submodule of main:
	cmd = mainRepo.GitCommand(t, "submodule", "add", submRepo.Path, "sub")
	cmd.Dir = mainRepo.Path
	require.NoError(t, cmd.Run(), "adding submodule")

	cmd = mainRepo.GitCommand(t, "commit", "-m", "add submodule")
	testutils.AddAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "committing submodule to main")

	// Analyze the main repo:
	h, err := sizes.ScanRepositoryUsingGraph(
		mainRepo.Repository(t),
		refGrouper{}, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(2), h.UniqueBlobCount, "unique blob count")
	assert.Equal(t, counts.Count32(2), h.MaxExpandedBlobCount, "max expanded blob count")
	assert.Equal(t, counts.Count32(1), h.MaxExpandedSubmoduleCount, "max expanded submodule count")

	// Analyze the submodule:
	submRepo2 := testutils.TestRepo{
		Path: filepath.Join(mainRepo.Path, "sub"),
	}
	h, err = sizes.ScanRepositoryUsingGraph(
		submRepo2.Repository(t),
		refGrouper{}, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(2), h.UniqueBlobCount, "unique blob count")
	assert.Equal(t, counts.Count32(3), h.MaxExpandedBlobCount, "max expanded blob count")
}
