package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractRepositoryNameFromURL(t *testing.T) {
	tests := []struct {
		name         string
		repository   string
		expectedRepo string
	}{
		{
			name:         "OCI URL with artifactory",
			repository:   "oci://example.com/artifactory/my-repo",
			expectedRepo: "artifactory",
		},
		{
			name:         "OCI URL without artifactory",
			repository:   "oci://example.com/my-repo",
			expectedRepo: "my-repo",
		},
		{
			name:         "HTTPS URL",
			repository:   "https://charts.example.com/repo",
			expectedRepo: "repo",
		},
		{
			name:         "Non-URL string",
			repository:   "my-repo",
			expectedRepo: "my-repo",
		},
		{
			name:         "Empty string",
			repository:   "",
			expectedRepo: "",
		},
		// Subdomain Docker access method
		{
			name:         "OCI subdomain - SaaS style",
			repository:   "oci://demo-helm-local.jfrog.io",
			expectedRepo: "demo-helm-local",
		},
		{
			name:         "OCI subdomain - on-prem multi-label domain",
			repository:   "oci://abadoc-helmoci-dev-idesuite.hlb.helaba.de",
			expectedRepo: "abadoc-helmoci-dev-idesuite",
		},
		{
			name:         "OCI subdomain with port",
			repository:   "oci://my-helm-repo.registry.example.com:8443",
			expectedRepo: "my-helm-repo",
		},
		{
			name:         "HTTPS subdomain - no path",
			repository:   "https://helm-local.artifactory.example.com",
			expectedRepo: "helm-local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRepositoryNameFromURL(tt.repository)
			assert.Equal(t, tt.expectedRepo, result)
		})
	}
}

func TestExtractRepositoryFromHostSubdomain(t *testing.T) {
	tests := []struct {
		name         string
		host         string
		expectedRepo string
	}{
		{
			name:         "Three-label hostname",
			host:         "demo-helm-local.jfrog.io",
			expectedRepo: "demo-helm-local",
		},
		{
			name:         "Four-label hostname",
			host:         "abadoc-helmoci-dev-idesuite.hlb.helaba.de",
			expectedRepo: "abadoc-helmoci-dev-idesuite",
		},
		{
			name:         "Hostname with port",
			host:         "my-repo.registry.com:8443",
			expectedRepo: "my-repo",
		},
		{
			name:         "Two-label hostname",
			host:         "example.com",
			expectedRepo: "example",
		},
		{
			name:         "Single-label hostname",
			host:         "localhost",
			expectedRepo: "",
		},
		{
			name:         "Single-label hostname with port",
			host:         "localhost:8080",
			expectedRepo: "",
		},
		{
			name:         "Empty string",
			host:         "",
			expectedRepo: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRepositoryFromHostSubdomain(tt.host)
			assert.Equal(t, tt.expectedRepo, result)
		})
	}
}

// TestExtractDependencyPathInRepository tests the extractDependencyPath function
// Note: This test is also in layers_test.go, keeping this version for repository.go specific testing
func TestExtractDependencyPathInRepository(t *testing.T) {
	tests := []struct {
		name         string
		depId        string
		expectedPath string
	}{
		{
			name:         "Valid dependency ID",
			depId:        "nginx:1.2.3",
			expectedPath: "nginx/1.2.3",
		},
		{
			name:         "Invalid format - no colon",
			depId:        "nginx",
			expectedPath: "",
		},
		{
			name:         "Invalid format - multiple colons",
			depId:        "nginx:1.2.3:extra",
			expectedPath: "",
		},
		{
			name:         "Empty string",
			depId:        "",
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDependencyPath(tt.depId)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

func TestExtractRepoAndSubPath(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		expectedRepo    string
		expectedSubPath string
	}{
		{
			name:            "No subpath - simple repo",
			url:             "oci://example.com/my-repo",
			expectedRepo:    "my-repo",
			expectedSubPath: "",
		},
		{
			name:            "Single-level subpath",
			url:             "oci://example.com/my-repo/team",
			expectedRepo:    "my-repo",
			expectedSubPath: "team",
		},
		{
			name:            "Multi-level subpath",
			url:             "oci://example.com/my-repo/team/app/env",
			expectedRepo:    "my-repo",
			expectedSubPath: "team/app/env",
		},
		{
			name:            "OCI URL with artifactory prefix",
			url:             "oci://example.com/artifactory/my-repo",
			expectedRepo:    "artifactory",
			expectedSubPath: "my-repo",
		},
		{
			name:            "HTTPS URL with subpath",
			url:             "https://example.com/my-repo/subdir",
			expectedRepo:    "my-repo",
			expectedSubPath: "subdir",
		},
		{
			name:            "Empty string",
			url:             "",
			expectedRepo:    "",
			expectedSubPath: "",
		},
		{
			name:            "No protocol",
			url:             "my-repo",
			expectedRepo:    "my-repo",
			expectedSubPath: "",
		},
		{
			name:            "Subdomain only - no path segments",
			url:             "oci://demo-helm-local.jfrog.io",
			expectedRepo:    "demo-helm-local",
			expectedSubPath: "",
		},
		{
			name:            "OCI subdomain with port",
			url:             "oci://my-helm-repo.registry.example.com:8443",
			expectedRepo:    "my-helm-repo",
			expectedSubPath: "",
		},
		{
			name:            "Trailing slash is trimmed",
			url:             "oci://example.com/my-repo/team/",
			expectedRepo:    "my-repo",
			expectedSubPath: "team",
		},
		{
			name:            "Deep nesting",
			url:             "oci://registry.example.com/helm-local/org/team/project/charts",
			expectedRepo:    "helm-local",
			expectedSubPath: "org/team/project/charts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, subPath := extractRepoAndSubPath(tt.url)
			assert.Equal(t, tt.expectedRepo, repo)
			assert.Equal(t, tt.expectedSubPath, subPath)
		})
	}
}

func TestGenerateRepoCandidates(t *testing.T) {
	tests := []struct {
		name       string
		registry   string
		repository string
		expected   []ociRepoCandidate
	}{
		{
			name:       "Path-based: host with single-segment repo",
			registry:   "art.com",
			repository: "helm-repo",
			expected: []ociRepoCandidate{
				{repoKey: "helm-repo", subpath: ""},
				{repoKey: "art", subpath: "helm-repo"},
			},
		},
		{
			name:       "Path-based: host with multi-segment repo",
			registry:   "art.com",
			repository: "helm-repo/team/charts",
			expected: []ociRepoCandidate{
				{repoKey: "helm-repo", subpath: "team/charts"},
				{repoKey: "art", subpath: "helm-repo/team/charts"},
			},
		},
		{
			name:       "Subdomain: repo-named host with path segments",
			registry:   "helm.art.com",
			repository: "team/charts",
			expected: []ociRepoCandidate{
				{repoKey: "team", subpath: "charts"},
				{repoKey: "helm", subpath: "team/charts"},
			},
		},
		{
			name:       "Subdomain: SaaS-style host with no path",
			registry:   "demo-helm-local.jfrog.io",
			repository: "",
			expected: []ociRepoCandidate{
				{repoKey: "demo-helm-local", subpath: ""},
			},
		},
		{
			name:       "Single-label host (no subdomain), single repo segment",
			registry:   "localhost",
			repository: "my-repo",
			expected: []ociRepoCandidate{
				{repoKey: "my-repo", subpath: ""},
			},
		},
		{
			name:       "Empty registry and repository",
			registry:   "",
			repository: "",
			expected:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := generateRepoCandidates(tt.registry, tt.repository)
			assert.Equal(t, tt.expected, candidates)
		})
	}
}

func TestGenerateOCIRepoCandidates(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected []ociRepoCandidate
	}{
		{
			name: "Reviewer case 1: path-based works - oci://art.com/helm-repo/team/charts",
			url:  "oci://art.com/helm-repo/team/charts",
			expected: []ociRepoCandidate{
				{repoKey: "helm-repo", subpath: "team/charts"},
				{repoKey: "art", subpath: "helm-repo/team/charts"},
			},
		},
		{
			name: "Reviewer case 2: subdomain-based - oci://helm.art.com/team/charts",
			url:  "oci://helm.art.com/team/charts",
			expected: []ociRepoCandidate{
				{repoKey: "team", subpath: "charts"},
				{repoKey: "helm", subpath: "team/charts"},
			},
		},
		{
			name: "Reviewer case 3: ambiguous - oci://helm-prod.example/team/charts",
			url:  "oci://helm-prod.example/team/charts",
			expected: []ociRepoCandidate{
				{repoKey: "team", subpath: "charts"},
				{repoKey: "helm-prod", subpath: "team/charts"},
			},
		},
		{
			name: "Ambiguous: oci://helm-repo.art.com/team-a/charts - could be path or subdomain",
			url:  "oci://helm-repo.art.com/team-a/charts",
			expected: []ociRepoCandidate{
				{repoKey: "team-a", subpath: "charts"},
				{repoKey: "helm-repo", subpath: "team-a/charts"},
			},
		},
		{
			name: "Subdomain-only: oci://demo-helm-local.jfrog.io",
			url:  "oci://demo-helm-local.jfrog.io",
			expected: []ociRepoCandidate{
				{repoKey: "demo-helm-local", subpath: ""},
			},
		},
		{
			name: "Subdomain with port: oci://my-repo.registry.com:8443",
			url:  "oci://my-repo.registry.com:8443",
			expected: []ociRepoCandidate{
				{repoKey: "my-repo", subpath: ""},
			},
		},
		{
			name: "Deep nesting: oci://registry.example.com/helm-local/org/team/project",
			url:  "oci://registry.example.com/helm-local/org/team/project",
			expected: []ociRepoCandidate{
				{repoKey: "helm-local", subpath: "org/team/project"},
				{repoKey: "registry", subpath: "helm-local/org/team/project"},
			},
		},
		{
			name:     "Empty URL",
			url:      "",
			expected: nil,
		},
		{
			name:     "Only protocol",
			url:      "oci://",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := generateOCIRepoCandidates(tt.url)
			assert.Equal(t, tt.expected, candidates)
		})
	}
}

// TestIsOCIRepository tests the isOCIRepository function
func TestIsOCIRepository(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		expected   bool
	}{
		{
			name:       "OCI URL",
			repository: "oci://example.com/repo",
			expected:   true,
		},
		{
			name:       "HTTPS URL",
			repository: "https://charts.example.com/repo",
			expected:   false,
		},
		{
			name:       "Empty string",
			repository: "",
			expected:   false,
		},
		{
			name:       "Non-URL string",
			repository: "my-repo",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOCIRepository(tt.repository)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAmbiguousOCIURL_ResolutionScenarios exercises the full candidate-generation
// + path-building chain for the ambiguous URL oci://helm-repo.art.com/team-a/charts.
// The URL is ambiguous because the OCI reference can be interpreted as either:
//
//	path-based:     repo="team-a",     subpath="charts"
//	subdomain-based: repo="helm-repo", subpath="team-a/charts"
//
// resolveOCIPushArtifacts iterates candidates in order and returns the first
// one whose AQL search produces results, so we verify the ordering and the
// exact Artifactory paths that would be queried for each interpretation.
func TestAmbiguousOCIURL_ResolutionScenarios(t *testing.T) {
	const ambiguousURL = "oci://helm-repo.art.com/team-a/charts"
	const chartName = "mychart"
	const chartVersion = "1.0.0"

	candidates := generateOCIRepoCandidates(ambiguousURL)
	assert.Len(t, candidates, 2, "ambiguous URL must produce exactly two candidates")

	t.Run("Candidate ordering: path-based is tried first", func(t *testing.T) {
		assert.Equal(t, "team-a", candidates[0].repoKey,
			"first candidate should be path-based (repo=team-a)")
		assert.Equal(t, "charts", candidates[0].subpath)

		assert.Equal(t, "helm-repo", candidates[1].repoKey,
			"second candidate should be subdomain-based (repo=helm-repo)")
		assert.Equal(t, "team-a/charts", candidates[1].subpath)
	})

	t.Run("Search paths match each interpretation", func(t *testing.T) {
		path0 := buildOCIChartPath(candidates[0].subpath, chartName, chartVersion)
		assert.Equal(t, "charts/mychart/1.0.0", path0,
			"path-based candidate should search <subpath>/<chart>/<version>")

		path1 := buildOCIChartPath(candidates[1].subpath, chartName, chartVersion)
		assert.Equal(t, "team-a/charts/mychart/1.0.0", path1,
			"subdomain candidate should search <subpath>/<chart>/<version>")
	})

	t.Run("Assumed repo does not exist: falls through to subdomain candidate", func(t *testing.T) {
		// When the first candidate's repo (team-a) does not exist in Artifactory,
		// the AQL search returns zero results and the resolver moves on to the
		// next candidate (helm-repo). Verify the second candidate's search path.
		path := buildOCIChartPath(candidates[1].subpath, chartName, chartVersion)
		assert.Equal(t, "team-a/charts/mychart/1.0.0", path,
			"fallback candidate must search under subdomain-derived repo")
		assert.Equal(t, "helm-repo", candidates[1].repoKey)
	})

	t.Run("Artifact is in subdomain repo, not path-based repo", func(t *testing.T) {
		// When the chart actually lives under the subdomain-derived repository
		// (helm-repo) the first candidate (team-a) produces no hits; the
		// resolver falls through to the second candidate and succeeds.
		// This scenario verifies that the correct repo key and full path are
		// available after the fallthrough.
		secondCandidate := candidates[1]
		assert.Equal(t, "helm-repo", secondCandidate.repoKey)
		fullPath := buildOCIChartPath(secondCandidate.subpath, chartName, chartVersion)
		assert.Equal(t, "team-a/charts/mychart/1.0.0", fullPath)
	})
}

// TestAmbiguousOCIURL_VariousPatterns verifies candidate generation for several
// ambiguous OCI URLs where the first path segment and the hostname subdomain
// both look like plausible Artifactory repo keys.
func TestAmbiguousOCIURL_VariousPatterns(t *testing.T) {
	tests := []struct {
		name              string
		url               string
		chartName         string
		chartVersion      string
		expectedCandidate [2]struct {
			repoKey string
			path    string
		}
	}{
		{
			name:         "helm-repo.art.com/team-a/charts",
			url:          "oci://helm-repo.art.com/team-a/charts",
			chartName:    "nginx",
			chartVersion: "2.0.0",
			expectedCandidate: [2]struct {
				repoKey string
				path    string
			}{
				{repoKey: "team-a", path: "charts/nginx/2.0.0"},
				{repoKey: "helm-repo", path: "team-a/charts/nginx/2.0.0"},
			},
		},
		{
			name:         "prod-charts.registry.io/platform/services",
			url:          "oci://prod-charts.registry.io/platform/services",
			chartName:    "api-gateway",
			chartVersion: "3.1.0",
			expectedCandidate: [2]struct {
				repoKey string
				path    string
			}{
				{repoKey: "platform", path: "services/api-gateway/3.1.0"},
				{repoKey: "prod-charts", path: "platform/services/api-gateway/3.1.0"},
			},
		},
		{
			name:         "Single path segment: host/repo",
			url:          "oci://my-charts.artifactory.io/shared",
			chartName:    "common",
			chartVersion: "0.1.0",
			expectedCandidate: [2]struct {
				repoKey string
				path    string
			}{
				{repoKey: "shared", path: "common/0.1.0"},
				{repoKey: "my-charts", path: "shared/common/0.1.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := generateOCIRepoCandidates(tt.url)
			assert.Len(t, candidates, 2, "ambiguous URL must produce exactly two candidates")

			for i, expected := range tt.expectedCandidate {
				assert.Equal(t, expected.repoKey, candidates[i].repoKey)
				path := buildOCIChartPath(candidates[i].subpath, tt.chartName, tt.chartVersion)
				assert.Equal(t, expected.path, path)
			}
		})
	}
}

// TestRemoveProtocolPrefix tests the removeProtocolPrefix function
func TestRemoveProtocolPrefix(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "OCI URL",
			url:      "oci://example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "HTTPS URL",
			url:      "https://example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "HTTP URL",
			url:      "http://example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "URL with custom scheme",
			url:      "custom://example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "No protocol",
			url:      "example.com/repo",
			expected: "example.com/repo",
		},
		{
			name:     "Empty string",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeProtocolPrefix(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
