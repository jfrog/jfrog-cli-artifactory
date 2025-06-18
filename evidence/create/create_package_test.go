package create

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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

	cmd := NewCreateEvidencePackage(serverDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, packageName, packageVersion, packageRepoName)
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
