package apmcommon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDirectAndRequestedBy_RejectsFlagShapedRepoURL(t *testing.T) {
	isDirect, requestedBy := resolveDirectAndRequestedBy(t.TempDir(), "--global")
	assert.True(t, isDirect)
	assert.Empty(t, requestedBy)
}

func TestFinalScope(t *testing.T) {
	tests := []struct {
		name      string
		isDirect  bool
		isDev     bool
		wantScope string
	}{
		{name: "direct, not dev -> prod", isDirect: true, isDev: false, wantScope: apmScopeProd},
		{name: "direct, dev -> dev", isDirect: true, isDev: true, wantScope: apmScopeDev},
		{name: "transitive, dev -> dev", isDirect: false, isDev: true, wantScope: apmScopeDev},
		{name: "transitive, not dev -> transitive", isDirect: false, isDev: false, wantScope: apmScopeTransitive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantScope, finalScope(tt.isDirect, tt.isDev))
		})
	}
}

func TestResolveDependencies_ScopeFollowsPriorityLadder(t *testing.T) {
	tempDir := t.TempDir()
	lockfilePath := filepath.Join(tempDir, ApmLockfileName)
	// Flag-shaped repo_urls deterministically resolve isDirect=true with no requestedBy (see
	// TestResolveDirectAndRequestedBy_RejectsFlagShapedRepoURL) without needing a real apm
	// subprocess to succeed - exactly what's needed here to isolate the scope computation.
	content := `
lockfile_version: "2"
dependencies:
  - repo_url: "--fake-dev-dep"
    version: 1.0.0
    source: registry
    resolved_hash: sha256:abc123
    is_dev: true
  - repo_url: "--fake-prod-dep"
    version: 1.0.0
    source: registry
    resolved_hash: sha256:def456
`
	require.NoError(t, os.WriteFile(lockfilePath, []byte(content), 0o644))

	deps, err := ResolveDependencies(lockfilePath)
	require.NoError(t, err)
	require.Len(t, deps, 2)

	byID := make(map[string][]string, len(deps))
	for _, dep := range deps {
		byID[dep.ID] = dep.Scopes
	}
	// Both are isDirect=true (the flag-shaped fallback), so the ladder distinguishes them
	// purely on is_dev: dev wins for the first, prod for the second.
	assert.Equal(t, []string{apmScopeDev}, byID["--fake-dev-dep:1.0.0"])
	assert.Equal(t, []string{apmScopeProd}, byID["--fake-prod-dep:1.0.0"])
}

func TestParseDepsWhyOutput_DirectDependency(t *testing.T) {
	out := []byte(`{
		"package": {"is_direct": true, "repo_url": "uday/pkg-consumer", "source": "registry", "version": "1.0.0"},
		"paths": [{"chain": [{"is_direct": true, "repo_url": "uday/pkg-consumer"}]}]
	}`)
	isDirect, requestedBy := parseDepsWhyOutput(out, "uday/pkg-consumer")
	assert.True(t, isDirect)
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
	isDirect, requestedBy := parseDepsWhyOutput(out, "uday/pkg-base")
	assert.False(t, isDirect)
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
	isDirect, requestedBy := parseDepsWhyOutput(out, "shared/lib")
	assert.False(t, isDirect)
	assert.Equal(t, [][]string{{"a/pkg"}, {"b/pkg"}}, requestedBy)
}

func TestParseDepsWhyOutput_MalformedJSONFallsBackToDirect(t *testing.T) {
	isDirect, requestedBy := parseDepsWhyOutput([]byte("not json"), "uday/pkg-base")
	assert.True(t, isDirect)
	assert.Empty(t, requestedBy)
}

// TestApmScopeConstantsAreStable guards the literal scope strings against accidental drift -
// they're part of the public build-info contract (consumed by Xray, the UI, etc.), and matching
// the naming the newer sibling FlexPack integrations converged on (Alpine's own
// TestAlpineScopeConstantsAreStable checks the identical pair) is the reason "prod" was chosen
// over the older, now-minority "runtime" convention.
func TestApmScopeConstantsAreStable(t *testing.T) {
	assert.Equal(t, "prod", apmScopeProd)
	assert.Equal(t, "dev", apmScopeDev)
	assert.Equal(t, "transitive", apmScopeTransitive)
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
