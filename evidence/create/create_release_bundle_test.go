package create

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
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

func TestReleaseBundle(t *testing.T) {
	tests := []struct {
		name                 string
		project              string
		releaseBundle        string
		releaseBundleVersion string
		expectedPath         string
		expectedCheckSum     string
		expectedName         string
		expectError          bool
	}{
		{
			name:                 "Valid release bundle with project",
			project:              "myProject",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "myProject-release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectedName:         "bundleName 1.0.0",
			expectError:          false,
		},
		{
			name:                 "Valid release bundle default project",
			project:              "default",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectedName:         "bundleName 1.0.0",
			expectError:          false,
		},
		{
			name:                 "Valid release bundle empty project",
			project:              "default",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectedName:         "bundleName 1.0.0",
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := NewCreateEvidenceReleaseBundle(nil, "", "", "", "", "", tt.project, tt.releaseBundle, tt.releaseBundleVersion)
			c, ok := evidence.(*createEvidenceReleaseBundle)
			if !ok {
				t.Fatal("Failed to create createEvidenceReleaseBundle instance")
			}
			aa := &mockReleaseBundleArtifactoryServicesManager{}
			path, sha256, err := c.buildReleaseBundleSubjectPath(aa)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, sha256)
				assert.Empty(t, path)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
				assert.Equal(t, tt.expectedCheckSum, sha256)
				assert.Equal(t, tt.expectedName, c.displayName)
			}
		})
	}
}

func TestCreateEvidenceReleaseBundle_RecordSummary(t *testing.T) {
	tempDir, err := fileutils.CreateTempDir()
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, fileutils.RemoveTempDir(tempDir))
	}()

	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	assert.NoError(t, os.Setenv(coreutils.SummaryOutputDirPathEnv, tempDir))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
		assert.NoError(t, os.Unsetenv(coreutils.SummaryOutputDirPathEnv))
	}()

	serverDetails := &config.ServerDetails{
		Url:      "http://test.com",
		User:     "testuser",
		Password: "testpass",
	}

	evidence := NewCreateEvidenceReleaseBundle(
		serverDetails,
		"",
		"test-predicate-type",
		"",
		"test-key",
		"test-key-id",
		"myProject",
		"testBundle",
		"2.0.0",
	)
	c, ok := evidence.(*createEvidenceReleaseBundle)
	if !ok {
		t.Fatal("Failed to create createEvidenceReleaseBundle instance")
	}

	expectedResponse := &model.CreateResponse{
		PredicateSlug: "test-rb-slug",
		Verified:      true,
	}
	expectedSubject := "myProject-release-bundles-v2/testBundle/2.0.0/release-bundle.json.evd"
	expectedSha256 := "rb-sha256"

	c.recordSummary(expectedResponse, expectedSubject, expectedSha256)

	summaryFiles, err := fileutils.ListFiles(tempDir, true)
	assert.NoError(t, err)
	assert.True(t, len(summaryFiles) > 0, "Summary file should be created")

	for _, file := range summaryFiles {
		if strings.HasSuffix(file, "-data") {
			content, err := os.ReadFile(file)
			assert.NoError(t, err)

			var summaryData commandsummary.EvidenceSummaryData
			err = json.Unmarshal(content, &summaryData)
			assert.NoError(t, err)

			assert.Equal(t, expectedSubject, summaryData.Subject)
			assert.Equal(t, expectedSha256, summaryData.SubjectSha256)
			assert.Equal(t, "test-predicate-type", summaryData.PredicateType)
			assert.Equal(t, "test-rb-slug", summaryData.PredicateSlug)
			assert.True(t, summaryData.Verified)
			assert.Equal(t, "testBundle 2.0.0", summaryData.DisplayName)
			assert.Equal(t, commandsummary.SubjectTypeReleaseBundle, summaryData.SubjectType)
			assert.Equal(t, "testBundle", summaryData.ReleaseBundleName)
			assert.Equal(t, "2.0.0", summaryData.ReleaseBundleVersion)
			assert.Equal(t, "myProject-release-bundles-v2", summaryData.RepoKey)
			break
		}
	}
}
