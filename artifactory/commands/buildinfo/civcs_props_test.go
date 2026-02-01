package buildinfo

import (
	"testing"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/stretchr/testify/assert"
)

func TestBuildSpecFromPaths(t *testing.T) {
	tests := []struct {
		name          string
		artifactPaths []string
		expectedCount int
	}{
		{
			name:          "empty paths",
			artifactPaths: []string{},
			expectedCount: 0,
		},
		{
			name:          "single path",
			artifactPaths: []string{"repo/path/to/file.jar"},
			expectedCount: 1,
		},
		{
			name: "multiple paths",
			artifactPaths: []string{
				"repo1/path/to/file1.jar",
				"repo2/path/to/file2.jar",
				"repo3/path/to/file3.jar",
			},
			expectedCount: 3,
		},
		{
			name: "paths with virtual repo prefix",
			artifactPaths: []string{
				"cli-pypi-virtual/jfrog-example/1.0/example-1.0.whl",
				"cli-pypi-virtual/jfrog-example/1.0/example-1.0.tar.gz",
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specFiles := buildSpecFromPaths(tt.artifactPaths)
			assert.NotNil(t, specFiles)
			assert.Len(t, specFiles.Files, tt.expectedCount)

			// Verify each path is correctly set as a pattern
			for i, path := range tt.artifactPaths {
				assert.Equal(t, path, specFiles.Files[i].Pattern)
			}
		})
	}
}

func TestConstructArtifactPathWithFallback(t *testing.T) {
	tests := []struct {
		name     string
		artifact buildinfo.Artifact
		expected string
	}{
		{
			name: "with OriginalDeploymentRepo and Path",
			artifact: buildinfo.Artifact{
				OriginalDeploymentRepo: "my-repo",
				Path:                   "path/to/file.jar",
				Name:                   "file.jar",
			},
			expected: "my-repo/path/to/file.jar",
		},
		{
			name: "with OriginalDeploymentRepo and Name only",
			artifact: buildinfo.Artifact{
				OriginalDeploymentRepo: "my-repo",
				Name:                   "file.jar",
			},
			expected: "my-repo/file.jar",
		},
		{
			name: "without OriginalDeploymentRepo - fallback to Path",
			artifact: buildinfo.Artifact{
				Path: "my-repo/path/to/file.jar",
				Name: "file.jar",
			},
			expected: "my-repo/path/to/file.jar",
		},
		{
			name: "without OriginalDeploymentRepo or Path - fallback to Name",
			artifact: buildinfo.Artifact{
				Name: "file.jar",
			},
			expected: "file.jar",
		},
		{
			name:     "empty artifact",
			artifact: buildinfo.Artifact{},
			expected: "",
		},
		{
			name: "virtual repo path",
			artifact: buildinfo.Artifact{
				OriginalDeploymentRepo: "cli-pypi-virtual",
				Path:                   "jfrog-example/1.0/example-1.0.whl",
				Name:                   "example-1.0.whl",
			},
			expected: "cli-pypi-virtual/jfrog-example/1.0/example-1.0.whl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructArtifactPathWithFallback(tt.artifact)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCivcsExtractArtifactPathsWithWarnings(t *testing.T) {
	tests := []struct {
		name             string
		buildInfo        *buildinfo.BuildInfo
		expectedPaths    int
		expectedSkipped  int
	}{
		{
			name:             "empty build info",
			buildInfo:        &buildinfo.BuildInfo{},
			expectedPaths:    0,
			expectedSkipped:  0,
		},
		{
			name: "build info with artifacts",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{
								OriginalDeploymentRepo: "repo1",
								Path:                   "path/file1.jar",
								Name:                   "file1.jar",
							},
							{
								OriginalDeploymentRepo: "repo2",
								Path:                   "path/file2.jar",
								Name:                   "file2.jar",
							},
						},
					},
				},
			},
			expectedPaths:   2,
			expectedSkipped: 0,
		},
		{
			name: "build info with some empty artifacts",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{
								OriginalDeploymentRepo: "repo1",
								Path:                   "path/file1.jar",
								Name:                   "file1.jar",
							},
							{}, // Empty artifact - should be skipped
							{
								Path: "path/file3.jar", // No repo but has path - should use fallback
								Name: "file3.jar",
							},
						},
					},
				},
			},
			expectedPaths:   2,
			expectedSkipped: 1,
		},
		{
			name: "build info with virtual repo paths",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{
								OriginalDeploymentRepo: "cli-pypi-virtual",
								Path:                   "jfrog-example/1.0/example-1.0.whl",
								Name:                   "example-1.0.whl",
							},
							{
								OriginalDeploymentRepo: "cli-pypi-virtual",
								Path:                   "jfrog-example/1.0/example-1.0.tar.gz",
								Name:                   "example-1.0.tar.gz",
							},
						},
					},
				},
			},
			expectedPaths:   2,
			expectedSkipped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths, skipped := extractArtifactPathsWithWarnings(tt.buildInfo)
			assert.Len(t, paths, tt.expectedPaths)
			assert.Equal(t, tt.expectedSkipped, skipped)
		})
	}
}

func TestCivcsIs404Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "404 error",
			err:      assert.AnError, // Will check manually
			expected: false,          // AnError doesn't contain 404
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := is404Error(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Test with actual 404 message
	t.Run("error containing 404", func(t *testing.T) {
		err := &mockError{msg: "server response: 404 Not Found"}
		assert.True(t, is404Error(err))
	})

	t.Run("error containing not found", func(t *testing.T) {
		err := &mockError{msg: "artifact not found"}
		assert.True(t, is404Error(err))
	})
}

func TestCivcsIs403Error(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		assert.False(t, is403Error(nil))
	})

	t.Run("error containing 403", func(t *testing.T) {
		err := &mockError{msg: "server response: 403 Forbidden"}
		assert.True(t, is403Error(err))
	})

	t.Run("error containing forbidden", func(t *testing.T) {
		err := &mockError{msg: "access forbidden"}
		assert.True(t, is403Error(err))
	})

	t.Run("other error", func(t *testing.T) {
		err := &mockError{msg: "some other error"}
		assert.False(t, is403Error(err))
	})
}

// mockError is a simple error implementation for testing
type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}
