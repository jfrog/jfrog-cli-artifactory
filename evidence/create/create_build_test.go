package create

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
)

type mockArtifactoryServicesManagerBuild struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockArtifactoryServicesManagerBuild) FileInfo(_ string) (*utils.FileInfo, error) {
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

func (m *mockArtifactoryServicesManagerBuild) GetBuildInfo(services.BuildInfoParams) (*buildinfo.PublishedBuildInfo, bool, error) {
	buildInfo := &buildinfo.PublishedBuildInfo{
		BuildInfo: buildinfo.BuildInfo{
			Started: "2024-01-17T15:04:05.000-0700",
		},
	}
	return buildInfo, true, nil
}

func TestBuildInfo(t *testing.T) {
	tests := []struct {
		name             string
		project          string
		buildName        string
		buildNumber      string
		expectedPath     string
		expectedChecksum string
		expectError      bool
	}{
		{
			name:             "Valid buildName with project",
			project:          "myProject",
			buildName:        "buildName",
			buildNumber:      "1",
			expectedPath:     "myProject-build-info/buildName/1-1705529045000.json",
			expectedChecksum: "dummy_sha256",
			expectError:      false,
		},
		{
			name:             "Valid buildName default project",
			project:          "default",
			buildName:        "buildName",
			buildNumber:      "1",
			expectedPath:     "artifactory-build-info/buildName/1-1705529045000.json",
			expectedChecksum: "dummy_sha256",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok := NewCreateEvidenceBuild(nil, "", "", "", "", "", tt.project, tt.buildName, tt.buildNumber).(*createEvidenceBuild)
			if !ok {
				t.Fatal("Failed to create createEvidenceBuild instance")
			}
			aa := &mockArtifactoryServicesManagerBuild{}
			timestamp, err := getBuildLatestTimestamp(tt.buildName, tt.buildNumber, tt.project, aa)
			assert.NoError(t, err)
			path, sha256, err := c.buildBuildInfoSubjectPath(aa, timestamp)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, path)
				assert.Empty(t, sha256)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
				assert.Equal(t, tt.expectedChecksum, sha256)
			}
		})
	}
}

func TestCreateEvidenceBuild_RecordSummary(t *testing.T) {
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

	predicateFile := filepath.Join(tempDir, "predicate.json")
	predicateContent := `{"test": "predicate"}`
	assert.NoError(t, os.WriteFile(predicateFile, []byte(predicateContent), 0644))

	serverDetails := &config.ServerDetails{
		Url:      "http://test.com",
		User:     "testuser",
		Password: "testpass",
	}

	evidence := NewCreateEvidenceBuild(
		serverDetails,
		predicateFile,
		"test-predicate-type",
		"",
		"test-key",
		"test-key-id",
		"myProject",
		"testBuild",
		"123",
	)
	c, ok := evidence.(*createEvidenceBuild)
	if !ok {
		t.Fatal("Failed to create createEvidenceBuild instance")
	}

	expectedResponse := &model.CreateResponse{
		PredicateSlug: "test-slug",
		Verified:      true,
	}
	expectedTimestamp := "1705529045000"
	expectedSubject := "myProject-build-info/testBuild/123-1705529045000.json"
	expectedSha256 := "test-sha256"

	c.recordSummary(expectedSubject, expectedSha256, expectedResponse, expectedTimestamp)

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
			assert.Equal(t, "test-slug", summaryData.PredicateSlug)
			assert.True(t, summaryData.Verified)
			assert.Equal(t, "testBuild 123", summaryData.DisplayName)
			assert.Equal(t, commandsummary.SubjectTypeBuild, summaryData.SubjectType)
			assert.Equal(t, "testBuild", summaryData.BuildName)
			assert.Equal(t, "123", summaryData.BuildNumber)
			assert.Equal(t, expectedTimestamp, summaryData.BuildTimestamp)
			assert.Equal(t, "myProject-build-info", summaryData.RepoKey)
			break
		}
	}
}
