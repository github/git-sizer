package git_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/internal/testutils"
)

// hashObjectLiterally writes an object of the given type to `repo`
// using `git hash-object --literally`, which (unlike the normal path)
// does not reject "malformed" objects such as trees with excessively
// long path names. It returns the new object's OID.
func hashObjectLiterally(
	t *testing.T, repo *testutils.TestRepo, otype string, content []byte,
) git.OID {
	t.Helper()

	cmd := repo.GitCommand(
		t, "hash-object", "--literally", "-w", "-t", otype, "--stdin",
	)
	cmd.Stdin = bytes.NewReader(content)
	out, err := cmd.Output()
	require.NoError(t, err)

	oid, err := git.NewOID(string(bytes.TrimSpace(out)))
	require.NoError(t, err)
	return oid
}

// TestObjectIterLongPath checks that the object iterator can process a
// repository that contains an object whose path name is longer than
// the 64 KiB line length that `bufio.Scanner` accepts by default. Such
// a path makes `git rev-list --objects` emit a line longer than 64 KiB,
// which used to abort the walk with "bufio.Scanner: token too long".
// See https://github.com/github/git-sizer/issues/157.
func TestObjectIterLongPath(t *testing.T) {
	t.Parallel()

	repo := testutils.NewTestRepo(t, true, "long-path")
	defer repo.Remove(t)

	r := repo.Repository(t)

	blob := repo.CreateObject(t, "blob", func(w io.Writer) error {
		_, err := io.WriteString(w, "hello\n")
		return err
	})

	// A tree that references the blob under a file name that is far
	// longer than 64 KiB, so that the corresponding `git rev-list
	// --objects` line exceeds the default scanner limit:
	longName := strings.Repeat("a", 100*1024)
	var treeContent bytes.Buffer
	fmt.Fprintf(&treeContent, "100644 %s\x00", longName)
	treeContent.Write(blob.Bytes())
	tree := hashObjectLiterally(t, repo, "tree", treeContent.Bytes())

	commit := repo.CreateObject(t, "commit", func(w io.Writer) error {
		_, err := fmt.Fprintf(
			w,
			"tree %s\n"+
				"author Example <example@example.com> 1112911993 -0700\n"+
				"committer Example <example@example.com> 1112911993 -0700\n"+
				"\n"+
				"Commit with a very long path\n",
			tree,
		)
		return err
	})

	repo.UpdateRef(t, "refs/heads/main", commit)

	ctx := context.Background()
	iter, err := r.NewObjectIter(ctx)
	require.NoError(t, err)

	errChan := make(chan error, 1)
	go func() {
		defer iter.Close()
		errChan <- iter.AddRoot(commit)
	}()

	var oids []git.OID
	for {
		header, ok, err := iter.Next()
		require.NoError(t, err)
		if !ok {
			break
		}
		oids = append(oids, header.OID)
	}
	require.NoError(t, <-errChan)

	assert.Contains(t, oids, commit)
	assert.Contains(t, oids, tree)
	assert.Contains(t, oids, blob)
}
