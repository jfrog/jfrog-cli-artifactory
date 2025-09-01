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
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/stretchr/testify/assert"
)

func TestNewCreateEvidencePackage(t *testing.T) {
	serverDetails := &config.ServerDetails{}
	predicateFilePath := "/path/to/predicate.json"
	predicateType := "custom-predicate"
	markdownFilePath := "/path/to/markdown.md"
	key := "test-key"
	keyId := "test-key-id"
	packageName := "test-package"
	packageVersion := "1.0.0"
	packageRepoName := "test-repo"

	cmd := NewCreateEvidencePackage(serverDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, packageName, packageVersion, packageRepoName, "", false)
	createCmd, ok := cmd.(*createEvidencePackage)
	assert.True(t, ok)

	// Test createEvidenceBase fields
	assert.Equal(t, serverDetails, createCmd.serverDetails)
	assert.Equal(t, predicateFilePath, createCmd.predicateFilePath)
	assert.Equal(t, predicateType, createCmd.predicateType)
	assert.Equal(t, markdownFilePath, createCmd.markdownFilePath)
	assert.Equal(t, key, createCmd.key)
	assert.Equal(t, keyId, createCmd.keyId)

	// Test packageService fields
	assert.Equal(t, packageName, createCmd.packageService.GetPackageName())
	assert.Equal(t, packageVersion, createCmd.packageService.GetPackageVersion())
	assert.Equal(t, packageRepoName, createCmd.packageService.GetPackageRepoName())
}

func TestCreateEvidencePackage_CommandName(t *testing.T) {
	cmd := &createEvidencePackage{}
	assert.Equal(t, "create-package-evidence", cmd.CommandName())
}

func TestCreateEvidencePackage_ServerDetails(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "http://test.com"}
	cmd := &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestCreateEvidencePackage_RecordSummary(t *testing.T) {
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

	packageName := "test-package"
	packageVersion := "1.0.0"
	repoName := "maven-local"

	evidence := NewCreateEvidencePackage(
		serverDetails,
		"",
		"test-predicate-type",
		"",
		"test-key",
		"test-key-id",
		packageName,
		packageVersion,
		repoName,
		"",
		false,
	)
	c, ok := evidence.(*createEvidencePackage)
	if !ok {
		t.Fatal("Failed to create createEvidencePackage instance")
	}

	expectedResponse := &model.CreateResponse{
		PredicateSlug: "test-slug",
		Verified:      false,
	}
	expectedArtifactPath := "maven-local/test-package/1.0.0/test-package-1.0.0.jar"
	expectedSha256 := "package-sha256"

	c.recordSummary(expectedResponse, expectedArtifactPath, expectedSha256)

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

			assert.Equal(t, expectedArtifactPath, summaryData.Subject)
			assert.Equal(t, expectedSha256, summaryData.SubjectSha256)
			assert.Equal(t, "test-predicate-type", summaryData.PredicateType)
			assert.Equal(t, "test-slug", summaryData.PredicateSlug)
			assert.False(t, summaryData.Verified)
			assert.Equal(t, "test-package 1.0.0", summaryData.DisplayName)
			assert.Equal(t, commandsummary.SubjectTypePackage, summaryData.SubjectType)
			assert.Equal(t, repoName, summaryData.RepoKey)
			break
		}
	}
}

func TestCreateEvidencePackage_ProviderId(t *testing.T) {
	tests := []struct {
		name               string
		providerId         string
		expectedProviderId string
	}{
		{
			name:               "With custom provider ID",
			providerId:         "custom-provider",
			expectedProviderId: "custom-provider",
		},
		{
			name:               "With empty provider ID",
			providerId:         "",
			expectedProviderId: "",
		},
		{
			name:               "With sonar provider ID",
			providerId:         "sonar",
			expectedProviderId: "sonar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverDetails := &config.ServerDetails{Url: "http://test.com"}

			cmd := NewCreateEvidencePackage(
				serverDetails,
				"",
				"test-predicate-type",
				"",
				"test-key",
				"test-key-id",
				"test-package",
				"1.0.0",
				"test-repo",
				tt.providerId,
				false,
			)

			createCmd, ok := cmd.(*createEvidencePackage)
			assert.True(t, ok)

			// Verify that the provider ID is correctly set in the base struct
			assert.Equal(t, tt.expectedProviderId, createCmd.providerId)
		})
	}
}
