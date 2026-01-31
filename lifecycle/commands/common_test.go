package commands

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/stretchr/testify/assert"
)

func TestGetPrerequisites_Success(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	rbCmd := &releaseBundleCmd{
		serverDetails:        serverDetails,
		releaseBundleName:    "testRelease",
		releaseBundleVersion: "1.0.0",
		sync:                 true,
		rbProjectKey:         "project1",
	}

	expectedQueryParams := services.CommonOptionalQueryParams{
		ProjectKey: rbCmd.rbProjectKey,
		Async:      false,
	}

	expectedRbDetails := services.ReleaseBundleDetails{
		ReleaseBundleName:    rbCmd.releaseBundleName,
		ReleaseBundleVersion: rbCmd.releaseBundleVersion,
	}

	servicesManager, rbDetails, queryParams, err := rbCmd.getPrerequisites()

	assert.NoError(t, err)
	assert.NotNil(t, servicesManager, "Expected servicesManager to be initialized")
	assert.Equal(t, expectedRbDetails, rbDetails, "ReleaseBundleDetails does not match expected values")
	assert.Equal(t, expectedQueryParams, queryParams, "QueryParams do not match expected values")

}

func TestGetPromotionPrerequisites_Success(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	rbp := &ReleaseBundlePromoteCommand{
		promotionType: "move",
		releaseBundleCmd: releaseBundleCmd{
			serverDetails:        serverDetails,
			releaseBundleName:    "testRelease",
			releaseBundleVersion: "1.0.0",
			sync:                 true,
			rbProjectKey:         "project1",
		},
	}

	expectedQueryParams := services.CommonOptionalQueryParams{
		ProjectKey:    rbp.rbProjectKey,
		Async:         false,
		PromotionType: rbp.promotionType,
	}

	expectedRbDetails := services.ReleaseBundleDetails{
		ReleaseBundleName:    rbp.releaseBundleName,
		ReleaseBundleVersion: rbp.releaseBundleVersion,
	}

	servicesManager, rbDetails, queryParams, err := rbp.getPromotionPrerequisites()

	assert.NoError(t, err)
	assert.NotNil(t, servicesManager, "Expected servicesManager to be initialized")
	assert.Equal(t, expectedRbDetails, rbDetails, "ReleaseBundleDetails do not match expected values") // Replace _ with appropriate variable.
	assert.Equal(t, expectedQueryParams, queryParams, "QueryParams do not match expected values")
}

func TestBuildRepoKey(t *testing.T) {
	repoKey := buildRepoKey("example-project")
	assert.Equal(t, "example-project-release-bundles-v2", repoKey)

	repoKey = buildRepoKey("")
	assert.Equal(t, releaseBundlesV2, repoKey)

	repoKey = buildRepoKey("default")
	assert.Equal(t, releaseBundlesV2, repoKey)
}

func TestGetArtifactFilesFromSpec(t *testing.T) {
	testCases := []struct {
		name          string
		inputFiles    []spec.File
		expectedCount int
		expectedFiles []string
	}{
		{
			name: "filters only files with Pattern",
			inputFiles: []spec.File{
				{Pattern: "repo/path/*.jar"},
				{Build: "my-build/123"},
				{Pattern: "repo/another/*.txt"},
				{Bundle: "my-bundle/1.0"},
			},
			expectedCount: 2,
			expectedFiles: []string{"repo/path/*.jar", "repo/another/*.txt"},
		},
		{
			name: "returns empty when no Pattern files",
			inputFiles: []spec.File{
				{Build: "my-build/123"},
				{Bundle: "my-bundle/1.0"},
			},
			expectedCount: 0,
			expectedFiles: []string{},
		},
		{
			name:          "handles empty input",
			inputFiles:    []spec.File{},
			expectedCount: 0,
			expectedFiles: []string{},
		},
		{
			name: "returns all when all have Pattern",
			inputFiles: []spec.File{
				{Pattern: "repo1/*.jar"},
				{Pattern: "repo2/*.war"},
				{Pattern: "repo3/*.zip"},
			},
			expectedCount: 3,
			expectedFiles: []string{"repo1/*.jar", "repo2/*.war", "repo3/*.zip"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getArtifactFilesFromSpec(tc.inputFiles)

			assert.Len(t, result, tc.expectedCount)
			for i, expectedPattern := range tc.expectedFiles {
				assert.Equal(t, expectedPattern, result[i].Pattern)
			}
		})
	}
}

func TestConvertSpecToReleaseBundlesSource(t *testing.T) {
	testCases := []struct {
		name            string
		inputFiles      []spec.File
		expectedCount   int
		expectedBundles []services.ReleaseBundleSource
		expectError     bool
	}{
		{
			// Valid bundle format with name/version
			name: "parses_valid_bundle_format",
			inputFiles: []spec.File{
				{Bundle: "my-bundle/1.0.0"},
			},
			expectedCount: 1,
			expectedBundles: []services.ReleaseBundleSource{
				{ReleaseBundleName: "my-bundle", ReleaseBundleVersion: "1.0.0"},
			},
			expectError: false,
		},
		{
			// Multiple bundles with project key
			name: "parses_multiple_bundles_with_project",
			inputFiles: []spec.File{
				{Bundle: "bundle-a/1.0", Project: "proj-a"},
				{Bundle: "bundle-b/2.0", Project: "proj-b"},
			},
			expectedCount: 2,
			expectedBundles: []services.ReleaseBundleSource{
				{ReleaseBundleName: "bundle-a", ReleaseBundleVersion: "1.0", ProjectKey: "proj-a"},
				{ReleaseBundleName: "bundle-b", ReleaseBundleVersion: "2.0", ProjectKey: "proj-b"},
			},
			expectError: false,
		},
		{
			// Mixed input - filters only files with Bundle field
			name: "filters_only_bundle_files",
			inputFiles: []spec.File{
				{Pattern: "repo/*.jar"},
				{Bundle: "my-bundle/1.0"},
				{Build: "my-build/123"},
			},
			expectedCount: 1,
			expectedBundles: []services.ReleaseBundleSource{
				{ReleaseBundleName: "my-bundle", ReleaseBundleVersion: "1.0"},
			},
			expectError: false,
		},
		{
			// Empty input returns empty result
			name:            "handles_empty_input",
			inputFiles:      []spec.File{},
			expectedCount:   0,
			expectedBundles: nil,
			expectError:     false,
		},
		{
			// Missing version in bundle format
			name: "error_on_missing_version",
			inputFiles: []spec.File{
				{Bundle: "my-bundle"},
			},
			expectError: true,
		},
		{
			// Empty bundle name (only slash)
			name: "error_on_empty_name",
			inputFiles: []spec.File{
				{Bundle: "/1.0.0"},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := convertSpecToReleaseBundlesSource(tc.inputFiles)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, result.ReleaseBundles, tc.expectedCount)
			for i, expected := range tc.expectedBundles {
				assert.Equal(t, expected.ReleaseBundleName, result.ReleaseBundles[i].ReleaseBundleName)
				assert.Equal(t, expected.ReleaseBundleVersion, result.ReleaseBundles[i].ReleaseBundleVersion)
				assert.Equal(t, expected.ProjectKey, result.ReleaseBundles[i].ProjectKey)
			}
		})
	}
}
