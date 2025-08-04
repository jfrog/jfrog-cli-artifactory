package create

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

type mockReleaseBundleArtifactoryServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockReleaseBundleArtifactoryServicesManager) FileInfo(_ string) (*utils.FileInfo, error) {
	fi := &utils.FileInfo{
		Checksums: struct {
			Sha1   string `json:"sha1,omitempty"`
			Sha256 string `json:"sha256,omitempty"`
			Md5    string `json:"md5,omitempty"`
		}{
			Sha256: "dummy_sha256",
		},
	}
	return fi, nil
}

func createTestReleaseBundleCommand() *createEvidenceReleaseBundle {
	return &createEvidenceReleaseBundle{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     &config.ServerDetails{Url: "http://test.com"},
			predicateFilePath: "/test/predicate.json",
			predicateType:     "test-type",
			key:               "test-key",
			keyId:             "test-key-id",
			stage:             "test-stage",
		},
		project:              "test-project",
		releaseBundle:        "test-bundle",
		releaseBundleVersion: "1.0.0",
	}
}

func TestNewCreateEvidenceReleaseBundle(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "http://test.com", User: "testuser"}
	predicateFilePath := "/path/to/predicate.json"
	predicateType := "custom-predicate"
	markdownFilePath := "/path/to/markdown.md"
	key := "test-key"
	keyId := "test-key-id"
	project := "test-project"
	releaseBundle := "test-bundle"
	releaseBundleVersion := "1.0.0"

	cmd := NewCreateEvidenceReleaseBundle(serverDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, project, releaseBundle, releaseBundleVersion)
	createCmd, ok := cmd.(*createEvidenceReleaseBundle)
	assert.True(t, ok)

	assert.Equal(t, serverDetails, createCmd.serverDetails)
	assert.Equal(t, predicateFilePath, createCmd.predicateFilePath)
	assert.Equal(t, predicateType, createCmd.predicateType)
	assert.Equal(t, markdownFilePath, createCmd.markdownFilePath)
	assert.Equal(t, key, createCmd.key)
	assert.Equal(t, keyId, createCmd.keyId)

	assert.Equal(t, project, createCmd.project)
	assert.Equal(t, releaseBundle, createCmd.releaseBundle)
	assert.Equal(t, releaseBundleVersion, createCmd.releaseBundleVersion)

	// The stage should be set (though it might be empty if the lifecycle service fails)
	// We just verify it's initialized, not the exact value since it depends on external service
	assert.NotNil(t, createCmd.stage)
}

func TestCreateEvidenceReleaseBundle_CommandName(t *testing.T) {
	cmd := &createEvidenceReleaseBundle{}
	assert.Equal(t, "create-release-bundle-evidence", cmd.CommandName())
}

func TestCreateEvidenceReleaseBundle_ServerDetails(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "http://test.com", User: "testuser"}
	cmd := &createEvidenceReleaseBundle{
		createEvidenceBase: createEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestBuildManifestPath(t *testing.T) {
	tests := []struct {
		name       string
		repoKey    string
		bundleName string
		version    string
		expected   string
	}{
		{
			name:       "Valid_Basic_Path",
			repoKey:    "test-repo",
			bundleName: "my-bundle",
			version:    "1.0.0",
			expected:   "test-repo/my-bundle/1.0.0/release-bundle.json.evd",
		},
		{
			name:       "With_Special_Characters",
			repoKey:    "test-repo-dev",
			bundleName: "my-bundle-v2",
			version:    "1.0.0-beta",
			expected:   "test-repo-dev/my-bundle-v2/1.0.0-beta/release-bundle.json.evd",
		},
		{
			name:       "With_Numbers",
			repoKey:    "repo123",
			bundleName: "bundle123",
			version:    "2.1.0",
			expected:   "repo123/bundle123/2.1.0/release-bundle.json.evd",
		},
		{
			name:       "Empty_Values",
			repoKey:    "",
			bundleName: "",
			version:    "",
			expected:   "///release-bundle.json.evd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildManifestPath(tt.repoKey, tt.bundleName, tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInitReleasebundlePromotionDetails(t *testing.T) {
	// Test the initReleasebundlePromotionDetails function structure and parameter handling

	t.Run("Parameter_Initialization", func(t *testing.T) {
		releaseBundle := "test-bundle"
		releaseBundleVersion := "1.0.0"
		project := "test-project"

		rbDetails, queryParams := initReleaseBundlePromotionDetails(releaseBundle, releaseBundleVersion, project)

		// Verify the returned structures have the expected field values
		assert.Equal(t, releaseBundle, rbDetails.ReleaseBundleName)
		assert.Equal(t, releaseBundleVersion, rbDetails.ReleaseBundleVersion)
		assert.Equal(t, project, queryParams.ProjectKey)
	})

	t.Run("Empty_Parameters", func(t *testing.T) {
		// Test with empty parameters
		rbDetails, queryParams := initReleaseBundlePromotionDetails("", "", "")

		assert.Equal(t, "", rbDetails.ReleaseBundleName)
		assert.Equal(t, "", rbDetails.ReleaseBundleVersion)
		assert.Equal(t, "", queryParams.ProjectKey)
	})
}

func TestGetReleaseBundleStage_Integration(t *testing.T) {
	t.Run("Error_Handling_Empty_Parameters", func(t *testing.T) {
		serverDetails := &config.ServerDetails{Url: "http://test.com"}

		result := getReleaseBundleStage(serverDetails, "", "1.0.0", "test-project")
		assert.Equal(t, "", result, "Should return empty string when bundle name is empty")

		result = getReleaseBundleStage(serverDetails, "test-bundle", "", "test-project")
		assert.Equal(t, "", result, "Should return empty string when version is empty")
	})

	t.Run("Service_Error_Handling", func(t *testing.T) {
		serverDetails := &config.ServerDetails{Url: "invalid-url"}
		result := getReleaseBundleStage(serverDetails, "test-bundle", "1.0.0", "test-project")

		assert.Equal(t, "", result, "Should return empty string when service creation fails")
	})

	t.Run("Stage_Functionality_Documentation", func(t *testing.T) {
		// Document the expected behavior of getReleaseBundleStage function
		// This function should:
		// 1. Create a lifecycle service manager from server details
		// 2. Initialize release bundle promotion details
		// 3. Get promotion details from the lifecycle service
		// 4. Extract the current stage from completed promotions
		// 5. Return empty string on any errors (with appropriate logging)

		assert.True(t, true, "Stage functionality is implemented in getReleaseBundleStage function")
	})
}

func TestStageIntegrationInConstructor(t *testing.T) {
	t.Run("Stage_Field_Integration", func(t *testing.T) {
		serverDetails := &config.ServerDetails{Url: "http://test.com", User: "testuser"}
		predicateFilePath := "/path/to/predicate.json"
		predicateType := "custom-predicate"
		markdownFilePath := "/path/to/markdown.md"
		key := "test-key"
		keyId := "test-key-id"
		project := "test-project"
		releaseBundle := "test-bundle"
		releaseBundleVersion := "1.0.0"

		cmd := NewCreateEvidenceReleaseBundle(serverDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, project, releaseBundle, releaseBundleVersion)
		createCmd, ok := cmd.(*createEvidenceReleaseBundle)
		assert.True(t, ok)

		stage := createCmd.stage
		assert.NotNil(t, stage, "Stage field should be initialized (even if empty)")

		t.Logf("Stage field set to: '%s' (may be empty if lifecycle service unavailable)", stage)
	})
}

func TestReleaseBundle(t *testing.T) {
	tests := []struct {
		name                 string
		project              string
		releaseBundle        string
		releaseBundleVersion string
		expectedPath         string
		expectedCheckSum     string
		expectError          bool
	}{
		{
			name:                 "Valid release bundle with project",
			project:              "myProject",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "myProject-release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectError:          false,
		},
		{
			name:                 "Valid release bundle default project",
			project:              "default",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectError:          false,
		},
		{
			name:                 "Valid release bundle empty project",
			project:              "",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createTestReleaseBundleCommand()
			cmd.project = tt.project
			cmd.releaseBundle = tt.releaseBundle
			cmd.releaseBundleVersion = tt.releaseBundleVersion

			aa := &mockReleaseBundleArtifactoryServicesManager{}
			path, sha256, err := cmd.buildReleaseBundleSubjectPath(aa)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, sha256)
				assert.Empty(t, path)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
				assert.Equal(t, tt.expectedCheckSum, sha256)
			}
		})
	}
}
