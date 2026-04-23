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

func TestGenerateOCIRepoCandidates(t *testing.T) {
	tests := []struct {
		name       string
		registry   string
		repository string
		expected   []ociRepoCandidate
	}{
		{
			name:       "host-only registry falls back to host repo",
			registry:   "helm-repo.company.example",
			repository: "",
			expected:   []ociRepoCandidate{{repoKey: "helm-repo"}},
		},
		{
			name:       "generic multi-label host yields host-first and path fallback candidates",
			registry:   "art.company.example",
			repository: "helm-repo/staging/libs",
			expected: []ociRepoCandidate{
				{repoKey: "art", subpath: "helm-repo/staging/libs"},
				{repoKey: "helm-repo", subpath: "staging/libs"},
			},
		},
		{
			name:       "virtual host prefers host-based candidate before path fallback",
			registry:   "helm-repo.company.example",
			repository: "team-a/charts",
			expected: []ociRepoCandidate{
				{repoKey: "helm-repo", subpath: "team-a/charts"},
				{repoKey: "team-a", subpath: "charts"},
			},
		},
		{
			name:       "single-segment virtual host subpath prefers host-based candidate before path fallback",
			registry:   "helm-repo.art.com",
			repository: "team-a",
			expected: []ociRepoCandidate{
				{repoKey: "helm-repo", subpath: "team-a"},
				{repoKey: "team-a", subpath: ""},
			},
		},
		{
			name:       "single-segment path repo does not add duplicate host fallback",
			registry:   "helm-repo.company.example",
			repository: "helm-repo",
			expected:   []ociRepoCandidate{{repoKey: "helm-repo", subpath: ""}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, generateRepoCandidates(tt.registry, tt.repository))
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
