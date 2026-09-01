package apmcommon

import (
	"strings"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

func TestSelectCachedAndUncached(t *testing.T) {
	deps := []ResolvedDep{
		{ID: "a/b:1.0.0"},
		{ID: "c/d:2.0.0"},
		{ID: "e/f:3.0.0"},
	}
	cachedChecksums := map[string]entities.Checksum{
		"a/b:1.0.0": {Sha256: "cached-sha256"},
	}

	cached, uncached := selectCachedAndUncached(deps, cachedChecksums)

	assert.Equal(t, map[string]entities.Checksum{"a/b:1.0.0": {Sha256: "cached-sha256"}}, cached)
	assert.Equal(t, []ResolvedDep{{ID: "c/d:2.0.0"}, {ID: "e/f:3.0.0"}}, uncached)
}

func TestSelectCachedAndUncached_NoneCached(t *testing.T) {
	deps := []ResolvedDep{{ID: "a/b:1.0.0"}, {ID: "c/d:2.0.0"}}

	cached, uncached := selectCachedAndUncached(deps, map[string]entities.Checksum{})

	assert.Empty(t, cached)
	assert.Equal(t, deps, uncached)
}

func TestSelectCachedAndUncached_AllCached(t *testing.T) {
	deps := []ResolvedDep{{ID: "a/b:1.0.0"}, {ID: "c/d:2.0.0"}}
	cachedChecksums := map[string]entities.Checksum{
		"a/b:1.0.0": {Sha256: "s1"},
		"c/d:2.0.0": {Sha256: "s2"},
	}

	cached, uncached := selectCachedAndUncached(deps, cachedChecksums)

	assert.Len(t, cached, 2)
	assert.Empty(t, uncached)
}

func TestHasAnyChecksum(t *testing.T) {
	tests := []struct {
		name     string
		checksum entities.Checksum
		expected bool
	}{
		{"All empty", entities.Checksum{}, false},
		{"SHA1 only", entities.Checksum{Sha1: "abc"}, true},
		{"SHA256 only", entities.Checksum{Sha256: "def"}, true},
		{"MD5 only", entities.Checksum{Md5: "ghi"}, true},
		{"All present", entities.Checksum{Sha1: "a", Sha256: "b", Md5: "c"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasAnyChecksum(tt.checksum))
		})
	}
}

func TestExtractArtifactFilename(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "APM URL format",
			url:      "https://artifactory.test/artifactory/api/agentpackages/test-repo/v1/packages/owner/my-skill/versions/1.0.0/download",
			expected: "my-skill-1.0.0.zip",
		},
		{
			name:     "APM URL with different package",
			url:      "https://artifactory.test/artifactory/api/agentpackages/repo/v1/packages/acme/tool/versions/2.0.1/download",
			expected: "tool-2.0.1.zip",
		},
		{
			name:     "Empty URL",
			url:      "",
			expected: "",
		},
		{
			name:     "Invalid APM URL format",
			url:      "https://artifactory.test/invalid",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractArtifactFilename(tt.url)
			assert.Equal(t, tt.expected, result, "extractArtifactFilename(%q)", tt.url)
		})
	}
}

func TestExtractRepoFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "APM URL format",
			url:      "https://artifactory.test/artifactory/api/agentpackages/apm-local/v1/packages/owner/skill/versions/1.0.0/download",
			expected: "apm-local",
		},
		{
			name:     "Classic repo URL",
			url:      "https://artifactory.test/artifactory/my-repo/path/to/artifact.jar",
			expected: "my-repo",
		},
		{
			name:     "Empty URL",
			url:      "",
			expected: "",
		},
		{
			name:     "Invalid URL format",
			url:      "https://artifactory.test/invalid",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRepoFromURL(tt.url)
			assert.Equal(t, tt.expected, result, "extractRepoFromURL(%q)", tt.url)
		})
	}
}

func TestParseAQLResults_IncludesPath(t *testing.T) {
	// Test that parseAQLResults correctly parses and includes the path field from AQL JSON.
	rawJSON := `{
		"results": [
			{
				"path": "owner-a/my-tool",
				"name": "my-tool-1.0.0.zip",
				"actual_sha1": "sha1-value-a",
				"sha256": "sha256-value-a",
				"actual_md5": "md5-value-a"
			},
			{
				"path": "owner-b/my-tool",
				"name": "my-tool-1.0.0.zip",
				"actual_sha1": "sha1-value-b",
				"sha256": "sha256-value-b",
				"actual_md5": "md5-value-b"
			}
		]
	}`

	reader := strings.NewReader(rawJSON)
	results, err := parseAQLResults(reader)

	assert.NoError(t, err, "parseAQLResults should not error")
	assert.Len(t, results, 2, "should parse 2 results")

	// Verify first result has correct path and all checksums
	assert.Equal(t, "owner-a/my-tool", results[0].Path, "first result should have path owner-a/my-tool")
	assert.Equal(t, "my-tool-1.0.0.zip", results[0].Name)
	assert.Equal(t, "sha1-value-a", results[0].Actual_Sha1)
	assert.Equal(t, "sha256-value-a", results[0].Sha256)
	assert.Equal(t, "md5-value-a", results[0].Actual_Md5)

	// Verify second result has correct path and all checksums
	assert.Equal(t, "owner-b/my-tool", results[1].Path, "second result should have path owner-b/my-tool")
	assert.Equal(t, "my-tool-1.0.0.zip", results[1].Name)
	assert.Equal(t, "sha1-value-b", results[1].Actual_Sha1)
	assert.Equal(t, "sha256-value-b", results[1].Sha256)
	assert.Equal(t, "md5-value-b", results[1].Actual_Md5)
}

func TestMapAQLResults_OwnerCollision(t *testing.T) {
	// Test that different owners with the same package name are handled correctly
	// without one overwriting the other's checksum.
	deps := []ResolvedDep{
		{
			ID:          "owner-a/my-tool:1.0.0",
			ResolvedURL: "https://artifactory.test/artifactory/api/agentpackages/apm-local/v1/packages/owner-a/my-tool/versions/1.0.0/download",
		},
		{
			ID:          "owner-b/my-tool:1.0.0",
			ResolvedURL: "https://artifactory.test/artifactory/api/agentpackages/apm-local/v1/packages/owner-b/my-tool/versions/1.0.0/download",
		},
	}

	// Simulated AQL results: both packages have the same filename but different paths.
	// Without proper deduplication, the second result would overwrite the first in a
	// filename-only map. The Path field in AQL results can include a leading slash
	// (e.g., "/owner-a/my-tool"), which we normalize by removing it.
	results := []servicesUtils.ResultItem{
		{
			Name:        "my-tool-1.0.0.zip",
			Path:        "/owner-a/my-tool", // AQL paths may have leading slash
			Actual_Sha1: "sha1-owner-a",
			Actual_Md5:  "md5-owner-a",
			Sha256:      "sha256-owner-a",
		},
		{
			Name:        "my-tool-1.0.0.zip",
			Path:        "/owner-b/my-tool", // AQL paths may have leading slash
			Actual_Sha1: "sha1-owner-b",
			Actual_Md5:  "md5-owner-b",
			Sha256:      "sha256-owner-b",
		},
	}

	checksumMap := make(map[string]entities.Checksum)
	mapAQLResults(deps, results, checksumMap)

	// Verify both dependencies got their correct checksums (not overwritten by collision).
	// Each dependency should have its unique set of checksums despite sharing the filename.
	assert.Equal(t, "sha256-owner-a", checksumMap["owner-a/my-tool:1.0.0"].Sha256, "owner-a should have sha256-owner-a")
	assert.Equal(t, "sha256-owner-b", checksumMap["owner-b/my-tool:1.0.0"].Sha256, "owner-b should have sha256-owner-b")
	assert.Equal(t, "sha1-owner-a", checksumMap["owner-a/my-tool:1.0.0"].Sha1, "owner-a should have sha1-owner-a")
	assert.Equal(t, "sha1-owner-b", checksumMap["owner-b/my-tool:1.0.0"].Sha1, "owner-b should have sha1-owner-b")
	assert.Equal(t, "md5-owner-a", checksumMap["owner-a/my-tool:1.0.0"].Md5, "owner-a should have md5-owner-a")
	assert.Equal(t, "md5-owner-b", checksumMap["owner-b/my-tool:1.0.0"].Md5, "owner-b should have md5-owner-b")
}
