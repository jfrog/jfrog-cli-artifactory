package nix

import (
	"strings"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/stretchr/testify/assert"
)

// TestBuildClosureAql_SingleDir locks in that the batched query shape for a
// single directory is the same as what the previous per-directory helpers
// (findNarFile / setBuildPropertiesInDir) emitted, so we don't break any
// Artifactory side that relied on the exact AQL form.
func TestBuildClosureAql_SingleDir(t *testing.T) {
	q := buildClosureAql("my-nix-local", []string{"binary-cache/abcdef"}, "*.nar.xz")
	expected := `{"repo":"my-nix-local","$or":[{"$and":[{"path":"binary-cache/abcdef"},{"name":{"$match":"*.nar.xz"}}]}]}`
	assert.Equal(t, expected, q)
}

func TestBuildClosureAql_MultipleDirs(t *testing.T) {
	q := buildClosureAql("repo1", []string{"binary-cache/aaa", "binary-cache/bbb"}, "*")
	expected := `{"repo":"repo1","$or":[{"$and":[{"path":"binary-cache/aaa"},{"name":{"$match":"*"}}]},{"$and":[{"path":"binary-cache/bbb"},{"name":{"$match":"*"}}]}]}`
	assert.Equal(t, expected, q)
}

// TestBuildClosureAql_DedupeDirs ensures repeated dirs are collapsed so the
// query stays compact even when callers (e.g. a closure with two roots
// resolving to the same store path) hand us duplicates.
func TestBuildClosureAql_DedupeDirs(t *testing.T) {
	q := buildClosureAql("repo1", []string{"binary-cache/aaa", "binary-cache/bbb", "binary-cache/aaa"}, "*.nar.xz")
	// Should contain exactly two $and clauses, not three.
	assert.Equal(t, 2, strings.Count(q, "$and"))
	assert.Contains(t, q, `"path":"binary-cache/aaa"`)
	assert.Contains(t, q, `"path":"binary-cache/bbb"`)
}

// TestFirstByDir_KeepsFirstMatch mirrors the previous findNarFile semantics —
// when AQL returns multiple files in the same directory we keep the first one,
// not the last.
func TestFirstByDir_KeepsFirstMatch(t *testing.T) {
	files := []aqlFile{
		{Name: "a.nar.xz", Dir: "binary-cache/hash1", Path: "binary-cache/hash1/a.nar.xz", Checksum: entities.Checksum{Sha1: "first"}},
		{Name: "b.nar.xz", Dir: "binary-cache/hash1", Path: "binary-cache/hash1/b.nar.xz", Checksum: entities.Checksum{Sha1: "second"}},
		{Name: "c.nar.xz", Dir: "binary-cache/hash2", Path: "binary-cache/hash2/c.nar.xz", Checksum: entities.Checksum{Sha1: "third"}},
	}
	idx := firstByDir(files)
	assert.Len(t, idx, 2)
	assert.Equal(t, "first", idx["binary-cache/hash1"].Checksum.Sha1, "should keep first match in dir")
	assert.Equal(t, "third", idx["binary-cache/hash2"].Checksum.Sha1)
}

func TestFirstByDir_EmptyInput(t *testing.T) {
	assert.Empty(t, firstByDir(nil))
	assert.Empty(t, firstByDir([]aqlFile{}))
}
