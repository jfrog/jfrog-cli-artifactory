package apmcommon

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveScopeAndRequestedBy_RejectsFlagShapedRepoURL(t *testing.T) {
	scopes, requestedBy := resolveScopeAndRequestedBy(t.TempDir(), "--global")
	assert.Equal(t, []string{"runtime"}, scopes)
	assert.Empty(t, requestedBy)
}

func TestParseDepsWhyOutput_DirectDependency(t *testing.T) {
	out := []byte(`{
		"package": {"is_direct": true, "repo_url": "uday/pkg-consumer", "source": "registry", "version": "1.0.0"},
		"paths": [{"chain": [{"is_direct": true, "repo_url": "uday/pkg-consumer"}]}]
	}`)
	scopes, requestedBy := parseDepsWhyOutput(out, "uday/pkg-consumer")
	assert.Equal(t, []string{"runtime"}, scopes)
	assert.Empty(t, requestedBy)
}

func TestParseDepsWhyOutput_TransitiveDependency(t *testing.T) {
	out := []byte(`{
		"package": {"is_direct": false, "repo_url": "uday/pkg-base", "source": "registry", "version": "1.0.0"},
		"paths": [{"chain": [
			{"is_direct": true, "repo_url": "uday/pkg-consumer"},
			{"is_direct": false, "repo_url": "uday/pkg-base"}
		]}]
	}`)
	scopes, requestedBy := parseDepsWhyOutput(out, "uday/pkg-base")
	assert.Equal(t, []string{"transitive"}, scopes)
	assert.Equal(t, [][]string{{"uday/pkg-consumer"}}, requestedBy)
}

func TestParseDepsWhyOutput_MultipleParentPaths(t *testing.T) {
	out := []byte(`{
		"package": {"is_direct": false, "repo_url": "shared/lib", "source": "registry", "version": "1.0.0"},
		"paths": [
			{"chain": [{"is_direct": true, "repo_url": "a/pkg"}, {"is_direct": false, "repo_url": "shared/lib"}]},
			{"chain": [{"is_direct": true, "repo_url": "b/pkg"}, {"is_direct": false, "repo_url": "shared/lib"}]}
		]
	}`)
	scopes, requestedBy := parseDepsWhyOutput(out, "shared/lib")
	assert.Equal(t, []string{"transitive"}, scopes)
	assert.Equal(t, [][]string{{"a/pkg"}, {"b/pkg"}}, requestedBy)
}

func TestParseDepsWhyOutput_MalformedJSONFallsBackToRuntime(t *testing.T) {
	scopes, requestedBy := parseDepsWhyOutput([]byte("not json"), "uday/pkg-base")
	assert.Equal(t, []string{"runtime"}, scopes)
	assert.Empty(t, requestedBy)
}

// TestParseDepsWhyOutput_PathCountCappedAtMax verifies the fan-in cap: a widely-shared
// dependency (e.g. a diamond dependency's base, reachable through many parents) reports at
// most requestedByMaxPaths distinct paths, matching the same cap golang.go/yarn.go/
// uv_flexpack.go apply to len(dependency.RequestedBy) elsewhere in this codebase.
func TestParseDepsWhyOutput_PathCountCappedAtMax(t *testing.T) {
	var pathsJSON strings.Builder
	for i := range requestedByMaxPaths + 5 {
		if i > 0 {
			pathsJSON.WriteByte(',')
		}
		parent := "parent" + string(rune('a'+i%26))
		pathsJSON.WriteString(`{"chain": [{"is_direct": true, "repo_url": "` + parent + `"}, {"is_direct": false, "repo_url": "target"}]}`)
	}
	out := []byte(`{
		"package": {"is_direct": false, "repo_url": "target", "source": "registry", "version": "1.0.0"},
		"paths": [` + pathsJSON.String() + `]
	}`)
	_, requestedBy := parseDepsWhyOutput(out, "target")
	assert.Len(t, requestedBy, requestedByMaxPaths)
}

// TestParseDepsWhyOutput_SinglePathNotTruncatedByDepth verifies an individual chain's depth
// is reported in full - only the number of distinct paths is capped, not how deep one path goes.
func TestParseDepsWhyOutput_SinglePathNotTruncatedByDepth(t *testing.T) {
	chain := `{"is_direct": false, "repo_url": "target"}`
	var parents strings.Builder
	for i := range requestedByMaxPaths + 5 {
		parents.WriteString(`{"is_direct": false, "repo_url": "p` + string(rune('a'+i%26)) + `"},`)
	}
	out := []byte(`{
		"package": {"is_direct": false, "repo_url": "target", "source": "registry", "version": "1.0.0"},
		"paths": [{"chain": [` + parents.String() + chain + `]}]
	}`)
	_, requestedBy := parseDepsWhyOutput(out, "target")
	require := assert.New(t)
	require.Len(requestedBy, 1)
	require.Len(requestedBy[0], requestedByMaxPaths+5)
}
