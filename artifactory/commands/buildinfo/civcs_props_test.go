package buildinfo

import (
	"errors"
	"testing"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

			for i, path := range tt.artifactPaths {
				assert.Equal(t, path, specFiles.Files[i].Pattern)
			}
		})
	}
}

func TestConstructPresentArtifactPath(t *testing.T) {
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
			name: "without OriginalDeploymentRepo returns empty",
			artifact: buildinfo.Artifact{
				Path: "my-repo/path/to/file.jar",
				Name: "file.jar",
			},
			expected: "",
		},
		{
			name: "gradle extractor path without OriginalDeploymentRepo returns empty",
			artifact: buildinfo.Artifact{
				Path: "minimal-example/1.0/minimal-example-1.0.jar",
				Name: "minimal-example-1.0.jar",
			},
			expected: "",
		},
		{
			name: "Name-only without OriginalDeploymentRepo returns empty",
			artifact: buildinfo.Artifact{
				Name: "file.jar",
			},
			expected: "",
		},
		{
			name:     "empty artifact returns empty",
			artifact: buildinfo.Artifact{},
			expected: "",
		},
		{
			name: "virtual repo path with OriginalDeploymentRepo",
			artifact: buildinfo.Artifact{
				OriginalDeploymentRepo: "cli-pypi-virtual",
				Path:                   "jfrog-example/1.0/example-1.0.whl",
				Name:                   "example-1.0.whl",
			},
			expected: "cli-pypi-virtual/jfrog-example/1.0/example-1.0.whl",
		},
		{
			name: "leading slash in Path is trimmed",
			artifact: buildinfo.Artifact{
				OriginalDeploymentRepo: "my-repo",
				Path:                   "/path/to/file.jar",
			},
			expected: "my-repo/path/to/file.jar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructPresentArtifactPath(tt.artifact)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractArtifactPaths(t *testing.T) {
	tests := []struct {
		name                string
		buildInfo           *buildinfo.BuildInfo
		expectedPresent     []string
		expectedMissing     int
	}{
		{
			name:            "empty build info",
			buildInfo:       &buildinfo.BuildInfo{},
			expectedPresent: nil,
			expectedMissing: 0,
		},
		{
			name: "all artifacts have OriginalDeploymentRepo",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{OriginalDeploymentRepo: "repo1", Path: "path/file1.jar", Name: "file1.jar"},
							{OriginalDeploymentRepo: "repo2", Path: "path/file2.jar", Name: "file2.jar"},
						},
					},
				},
			},
			expectedPresent: []string{"repo1/path/file1.jar", "repo2/path/file2.jar"},
			expectedMissing: 0,
		},
		{
			name: "all artifacts missing OriginalDeploymentRepo",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{Path: "path/file1.jar", Name: "file1.jar"},
							{Name: "file2.jar"},
						},
					},
				},
			},
			expectedPresent: nil,
			expectedMissing: 2,
		},
		{
			name: "mixed: some present some missing",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{OriginalDeploymentRepo: "repo1", Path: "path/file1.jar", Name: "file1.jar"},
							{Path: "path/file2.jar", Name: "file2.jar"},
						},
					},
				},
			},
			expectedPresent: []string{"repo1/path/file1.jar"},
			expectedMissing: 1,
		},
		{
			name: "completely empty artifact is counted as missing",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{{}},
					},
				},
			},
			expectedPresent: nil,
			expectedMissing: 1,
		},
		{
			name: "virtual repo path",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{OriginalDeploymentRepo: "cli-pypi-virtual", Path: "jfrog-example/1.0/example-1.0.whl"},
							{OriginalDeploymentRepo: "cli-pypi-virtual", Path: "jfrog-example/1.0/example-1.0.tar.gz"},
						},
					},
				},
			},
			expectedPresent: []string{
				"cli-pypi-virtual/jfrog-example/1.0/example-1.0.whl",
				"cli-pypi-virtual/jfrog-example/1.0/example-1.0.tar.gz",
			},
			expectedMissing: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			present, missing := extractArtifactPaths(tt.buildInfo)
			assert.Equal(t, tt.expectedPresent, present)
			assert.Equal(t, tt.expectedMissing, missing)
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
			err:      assert.AnError,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := is404Error(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}

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

// zeroRetryDelay sets retryDelayBase to 0 for the duration of the test to avoid real-time waits.
func zeroRetryDelay(t *testing.T) {
	t.Helper()
	retryDelayBase = 0
	t.Cleanup(func() { retryDelayBase = time.Second })
}

// stubBuildArtifacts substitutes the build-artifacts API resolver for the duration of the test.
// readers are returned in order, one per call; errs[i] pairs with readers[i].
func stubBuildArtifacts(t *testing.T, results []buildArtifactsResult) *int {
	t.Helper()
	original := resolveBuildArtifacts
	calls := 0
	resolveBuildArtifacts = func(_ artifactory.ArtifactoryServicesManager, _, _, _ string) (*content.ContentReader, error) {
		i := calls
		calls++
		if i >= len(results) {
			return nil, errors.New("unexpected extra call to resolveBuildArtifacts")
		}
		return results[i].reader, results[i].err
	}
	t.Cleanup(func() { resolveBuildArtifacts = original })
	return &calls
}

type buildArtifactsResult struct {
	reader *content.ContentReader
	err    error
}

// TestSetPropsViaBuildSearch_Success: the API returns an artifact; SetProps is called once.
func TestSetPropsViaBuildSearch_Success(t *testing.T) {
	zeroRetryDelay(t)

	props := "vcs.provider=github;vcs.org=jfrog"
	mockSM := new(mockServicesManager)
	searchReader, cleanup := createTestSearchReader(t)
	defer cleanup()

	calls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: searchReader}})
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == props
	})).Return(1, nil)

	setPropsViaBuildSearch(mockSM, "mybuild", "42", "", props)

	mockSM.AssertExpectations(t)
	assert.Equal(t, 1, *calls, "build-artifacts API should be called once")
	mockSM.AssertNumberOfCalls(t, "SetProps", 1)
	// The AQL-based search path must not be used for build resolution.
	mockSM.AssertNotCalled(t, "SearchFiles")
}

// TestSetPropsViaBuildSearch_EmptyResults: the API returns no artifacts; SetProps is never called.
func TestSetPropsViaBuildSearch_EmptyResults(t *testing.T) {
	zeroRetryDelay(t)

	mockSM := new(mockServicesManager)
	emptyReader, cleanup := createEmptySearchReader(t)
	defer cleanup()

	stubBuildArtifacts(t, []buildArtifactsResult{{reader: emptyReader}})

	setPropsViaBuildSearch(mockSM, "mybuild", "42", "", "vcs.provider=github")

	mockSM.AssertNotCalled(t, "SetProps")
}

// TestSetPropsViaBuildSearch_MissingBuildInfo: empty build name/number → no calls at all.
func TestSetPropsViaBuildSearch_MissingBuildInfo(t *testing.T) {
	zeroRetryDelay(t)

	mockSM := new(mockServicesManager)
	calls := stubBuildArtifacts(t, nil)

	setPropsViaBuildSearch(mockSM, "", "", "", "vcs.provider=github")
	setPropsViaBuildSearch(mockSM, "mybuild", "", "", "vcs.provider=github")

	assert.Equal(t, 0, *calls, "build-artifacts API must not be called without build name/number")
	mockSM.AssertNotCalled(t, "SetProps")
}

// TestSetPropsViaBuildSearch_SearchRetries: first API call errors, second succeeds.
func TestSetPropsViaBuildSearch_SearchRetries(t *testing.T) {
	zeroRetryDelay(t)

	mockSM := new(mockServicesManager)
	searchReader, cleanup := createTestSearchReader(t)
	defer cleanup()

	calls := stubBuildArtifacts(t, []buildArtifactsResult{
		{err: errors.New("transient timeout")},
		{reader: searchReader},
	})
	mockSM.On("SetProps", mock.Anything).Return(1, nil)

	setPropsViaBuildSearch(mockSM, "mybuild", "42", "", "vcs.provider=github")

	assert.Equal(t, 2, *calls)
	mockSM.AssertCalled(t, "SetProps", mock.Anything)
}

// TestSetPropsViaBuildSearch_SetProps404NoRetry: 404 from SetProps stops immediately, no further retry.
func TestSetPropsViaBuildSearch_SetProps404NoRetry(t *testing.T) {
	zeroRetryDelay(t)

	mockSM := new(mockServicesManager)
	searchReader, cleanup := createTestSearchReader(t)
	defer cleanup()

	calls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: searchReader}})
	mockSM.On("SetProps", mock.Anything).Return(0, errors.New("server returned 404 Not Found"))

	setPropsViaBuildSearch(mockSM, "mybuild", "42", "", "vcs.provider=github")

	assert.Equal(t, 1, *calls)
	mockSM.AssertNumberOfCalls(t, "SetProps", 1)
}

// TestSetPropsViaBuildSearch_SearchError_Retries: a failing API call is retried maxRetries times.
func TestSetPropsViaBuildSearch_SearchError_Retries(t *testing.T) {
	zeroRetryDelay(t)

	for _, errMsg := range []string{
		"server returned 404 Not Found",
		"server returned 403 Forbidden",
		"transient network error",
	} {
		t.Run(errMsg, func(t *testing.T) {
			mockSM := new(mockServicesManager)
			var results []buildArtifactsResult
			for i := 0; i < maxRetries; i++ {
				results = append(results, buildArtifactsResult{err: errors.New(errMsg)})
			}
			calls := stubBuildArtifacts(t, results)

			setPropsViaBuildSearch(mockSM, "mybuild", "42", "", "vcs.provider=github")

			assert.Equal(t, maxRetries, *calls)
			mockSM.AssertNotCalled(t, "SetProps")
		})
	}
}
