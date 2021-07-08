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
	"github.com/github/git-sizer/sizes"
)

// Smoke test that the program runs.
func TestExec(t *testing.T) {
	cmd := exec.Command("bin/git-sizer")
	output, err := cmd.CombinedOutput()
	assert.NoErrorf(t, err, "command failed; output: %#v", string(output))
}

func newRepository(t *testing.T, repoPath string) *git.Repository {
	t.Helper()

	repo, err := git.NewRepository(repoPath)
	require.NoError(t, err)
	return repo
}

func gitCommand(t *testing.T, repoPath string, args ...string) *exec.Cmd {
	t.Helper()

	gitArgs := []string{"-C", repoPath}
	gitArgs = append(gitArgs, args...)
	return exec.Command("git", gitArgs...)
}

func updateRef(t *testing.T, repoPath string, refname string, oid git.OID) {
	t.Helper()

	var cmd *exec.Cmd

	if oid == git.NullOID {
		cmd = gitCommand(t, repoPath, "update-ref", "-d", refname)
	} else {
		cmd = gitCommand(t, repoPath, "update-ref", refname, oid.String())
	}
	require.NoError(t, cmd.Run())
}

// createObject creates a new Git object, of the specified type, in
// the repository at `repoPath`. `writer` is a function that writes
// the object in `git hash-object` input format. This is used for
// testing only.
func createObject(
	t *testing.T, repoPath string, otype git.ObjectType, writer func(io.Writer) error,
) git.OID {
	t.Helper()

	cmd := gitCommand(t, repoPath, "hash-object", "-w", "-t", string(otype), "--stdin")
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

func addFile(t *testing.T, repoPath string, relativePath, contents string) {
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

	cmd := gitCommand(t, repoPath, "add", relativePath)
	require.NoErrorf(t, cmd.Run(), "adding file %q", relativePath)
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
	t *testing.T, path string, depth, breadth int, body string,
) {
	t.Helper()

	cmd := exec.Command("git", "init", "--bare", path)
	err := cmd.Run()
	require.NoError(t, err)

	oid := createObject(t, path, "blob", func(w io.Writer) error {
		_, err := io.WriteString(w, body)
		return err
	})

	digits := len(fmt.Sprintf("%d", breadth-1))

	mode := "100644"
	prefix := "f"

	for ; depth > 0; depth-- {
		oid = createObject(t, path, "tree", func(w io.Writer) error {
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

		mode = "40000"
		prefix = "d"
	}

	oid = createObject(t, path, "commit", func(w io.Writer) error {
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

	updateRef(t, path, "refs/heads/master", oid)
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
		//          1111111
		//01234567890123456
		{"+ + + + + + +   +", "refs/barfoo"},
		{"+ + + + + + +++  ", "refs/foo"},
		{"+ + + + + + +   +", "refs/foobar"},
		{"++  + + + +++   +", "refs/heads/foo"},
		{"++  + + + ++    +", "refs/heads/master"},
		{"+ + + ++  +      ", "refs/notes/discussion"},
		{"+ + ++  + +      ", "refs/remotes/origin/master"},
		{"+ + ++  + + +   +", "refs/remotes/upstream/foo"},
		{"+ + ++  + +      ", "refs/remotes/upstream/master"},
		{"+ + + + ++       ", "refs/stash"},
		{"+ ++  + + +++   +", "refs/tags/foolish"},
		{"+ ++  + + ++    +", "refs/tags/other"},
		{"+ ++  + + ++   + ", "refs/tags/release-1"},
		{"+ ++  + + ++   + ", "refs/tags/release-2"},
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
	path, err := ioutil.TempDir("", "ref-selection")
	require.NoError(t, err)

	defer os.RemoveAll(path)

	err = exec.Command("git", "init", "--bare", path).Run()
	require.NoError(t, err)

	for _, p := range references {
		oid := createObject(t, path, "blob", func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "%s\n", p.refname)
			return err
		})

		oid = createObject(t, path, "tree", func(w io.Writer) error {
			_, err = fmt.Fprintf(w, "100644 a.txt\x00%s", oid.Bytes())
			return err
		})

		oid = createObject(t, path, "commit", func(w io.Writer) error {
			_, err := fmt.Fprintf(
				w,
				"tree %s\n"+
					"author Example <example@example.com> 1112911993 -0700\n"+
					"committer Example <example@example.com> 1112911993 -0700\n"+
					"\n"+
					"Commit for reference %s\n",
				oid, p.refname,
			)
			return err
		})

		updateRef(t, path, p.refname, oid)
	}

	executable, err := exec.LookPath("bin/git-sizer")
	require.NoError(t, err)
	executable, err = filepath.Abs(executable)
	require.NoError(t, err)

	for i, p := range []struct {
		name string
		args []string
	}{
		{"no arguments", nil},                                                  // 0
		{"branches", []string{"--branches"}},                                   // 1
		{"no branches", []string{"--no-branches"}},                             // 2
		{"tags", []string{"--tags"}},                                           // 3
		{"no tags", []string{"--no-tags"}},                                     // 4
		{"remotes", []string{"--remotes"}},                                     // 5
		{"no remotes", []string{"--no-remotes"}},                               // 6
		{"notes", []string{"--notes"}},                                         // 7
		{"no notes", []string{"--no-notes"}},                                   // 8
		{"stash", []string{"--stash"}},                                         // 9
		{"no stash", []string{"--no-stash"}},                                   // 10
		{"branches and tags", []string{"--branches", "--tags"}},                // 11
		{"foo", []string{"--include-regexp", ".*foo.*"}},                       // 12
		{"refs/foo as prefix", []string{"--include", "refs/foo"}},              // 13
		{"refs/foo as regexp", []string{"--include-regexp", "refs/foo"}},       // 14
		{"release tags", []string{"--include-regexp", "refs/tags/release-.*"}}, // 15
		{
			name: "combination",
			args: []string{
				"--include=refs/heads",
				"--tags",
				"--exclude", "refs/heads/foo",
				"--include-regexp", ".*foo.*",
				"--exclude", "refs/foo",
				"--exclude-regexp", "refs/tags/release-.*",
			},
		}, // 16
	} {
		t.Run(
			p.name,
			func(t *testing.T) {
				args := []string{"--show-refs", "--no-progress", "--json", "--json-version=2"}
				args = append(args, p.args...)
				cmd := exec.Command(executable, args...)
				cmd.Dir = path
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

func TestBomb(t *testing.T) {
	t.Parallel()

	path, err := ioutil.TempDir("", "bomb")
	require.NoError(t, err)

	defer func() {
		os.RemoveAll(path)
	}()

	newGitBomb(t, path, 10, 10, "boom!\n")

	h, err := sizes.ScanRepositoryUsingGraph(
		newRepository(t, path), git.AllReferencesFilter, sizes.NameStyleFull, false,
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
	path, err := ioutil.TempDir("", "tagged-tags")
	require.NoError(t, err, "creating temporary directory")

	defer func() {
		os.RemoveAll(path)
	}()

	cmd := exec.Command("git", "init", path)
	require.NoError(t, cmd.Run(), "initializing repo")

	timestamp := time.Unix(1112911993, 0)

	cmd = gitCommand(t, path, "commit", "-m", "initial", "--allow-empty")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating commit")

	// The lexicographical order of these tags is important, hence
	// their strange names.
	cmd = gitCommand(t, path, "tag", "-m", "tag 1", "tag", "master")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 1")

	cmd = gitCommand(t, path, "tag", "-m", "tag 2", "bag", "tag")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 2")

	cmd = gitCommand(t, path, "tag", "-m", "tag 3", "wag", "bag")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating tag 3")

	h, err := sizes.ScanRepositoryUsingGraph(
		newRepository(t, path), git.AllReferencesFilter, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(3), h.MaxTagDepth, "tag depth")
}

func TestFromSubdir(t *testing.T) {
	t.Parallel()
	path, err := ioutil.TempDir("", "subdir")
	require.NoError(t, err, "creating temporary directory")

	defer func() {
		os.RemoveAll(path)
	}()

	cmd := exec.Command("git", "init", path)
	require.NoError(t, cmd.Run(), "initializing repo")

	timestamp := time.Unix(1112911993, 0)

	addFile(t, path, "subdir/file.txt", "Hello, world!\n")

	cmd = gitCommand(t, path, "commit", "-m", "initial")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating commit")

	h, err := sizes.ScanRepositoryUsingGraph(
		newRepository(t, filepath.Join(path, "subdir")),
		git.AllReferencesFilter, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(2), h.MaxPathDepth, "max path depth")
}

func TestSubmodule(t *testing.T) {
	t.Parallel()
	path, err := ioutil.TempDir("", "submodule")
	require.NoError(t, err, "creating temporary directory")

	defer func() {
		os.RemoveAll(path)
	}()

	timestamp := time.Unix(1112911993, 0)

	submPath := filepath.Join(path, "subm")
	cmd := exec.Command("git", "init", submPath)
	require.NoError(t, cmd.Run(), "initializing subm repo")
	addFile(t, submPath, "submfile1.txt", "Hello, submodule!\n")
	addFile(t, submPath, "submfile2.txt", "Hello again, submodule!\n")
	addFile(t, submPath, "submfile3.txt", "Hello again, submodule!\n")

	cmd = gitCommand(t, submPath, "commit", "-m", "subm initial")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating subm commit")

	mainPath := filepath.Join(path, "main")
	cmd = exec.Command("git", "init", mainPath)
	require.NoError(t, cmd.Run(), "initializing main repo")

	addFile(t, mainPath, "mainfile.txt", "Hello, main!\n")

	cmd = gitCommand(t, mainPath, "commit", "-m", "main initial")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "creating main commit")

	// Make subm a submodule of main:
	cmd = gitCommand(t, mainPath, "submodule", "add", submPath, "sub")
	cmd.Dir = mainPath
	require.NoError(t, cmd.Run(), "adding submodule")

	cmd = gitCommand(t, mainPath, "commit", "-m", "add submodule")
	addAuthorInfo(cmd, &timestamp)
	require.NoError(t, cmd.Run(), "committing submodule to main")

	// Analyze the main repo:
	h, err := sizes.ScanRepositoryUsingGraph(
		newRepository(t, mainPath), git.AllReferencesFilter, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(2), h.UniqueBlobCount, "unique blob count")
	assert.Equal(t, counts.Count32(2), h.MaxExpandedBlobCount, "max expanded blob count")
	assert.Equal(t, counts.Count32(1), h.MaxExpandedSubmoduleCount, "max expanded submodule count")

	// Analyze the submodule:
	h, err = sizes.ScanRepositoryUsingGraph(
		newRepository(t, filepath.Join(mainPath, "sub")),
		git.AllReferencesFilter, sizes.NameStyleNone, false,
	)
	require.NoError(t, err, "scanning repository")
	assert.Equal(t, counts.Count32(2), h.UniqueBlobCount, "unique blob count")
	assert.Equal(t, counts.Count32(3), h.MaxExpandedBlobCount, "max expanded blob count")
}
