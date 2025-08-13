package create

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
)

func TestNewCreateEvidenceApplication(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "http://test.com", User: "testuser"}
	predicateFilePath := "/path/to/predicate.json"
	predicateType := "custom-predicate"
	markdownFilePath := "/path/to/markdown.md"
	key := "test-key"
	keyId := "test-key-id"
	project := "test-project"
	applicationName := "test-app"
	applicationVersion := "1.0.0"

	cmd := NewCreateEvidenceApplication(serverDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, project, applicationName, applicationVersion)
	createCmd, ok := cmd.(*createEvidenceApplication)
	assert.True(t, ok)

	assert.Equal(t, serverDetails, createCmd.serverDetails)
	assert.Equal(t, predicateFilePath, createCmd.predicateFilePath)
	assert.Equal(t, predicateType, createCmd.predicateType)
	assert.Equal(t, markdownFilePath, createCmd.markdownFilePath)
	assert.Equal(t, key, createCmd.key)
	assert.Equal(t, keyId, createCmd.keyId)

	assert.Equal(t, project, createCmd.project)
	assert.Equal(t, applicationName, createCmd.application)
	assert.Equal(t, applicationVersion, createCmd.applicationVersion)
}

func TestCreateEvidenceApplication_CommandName(t *testing.T) {
	cmd := &createEvidenceApplication{}
	assert.Equal(t, "create-application-evidence", cmd.CommandName())
}

func TestCreateEvidenceApplication_ServerDetails(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "http://test.com", User: "testuser"}
	cmd := &createEvidenceApplication{
		createEvidenceBase: createEvidenceBase{serverDetails: serverDetails},
	}

	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestBuildManifestPathApplication(t *testing.T) {
	tests := []struct {
		name     string
		repoKey  string
		appName  string
		version  string
		expected string
	}{
		{
			name:     "Valid_Basic_Path",
			repoKey:  "test-repo",
			appName:  "my-app",
			version:  "1.0.0",
			expected: "test-repo/my-app/1.0.0/release-bundle.json.evd",
		},
		{
			name:     "With_Special_Characters",
			repoKey:  "test-repo-dev",
			appName:  "my-app-v2",
			version:  "1.0.0-beta",
			expected: "test-repo-dev/my-app-v2/1.0.0-beta/release-bundle.json.evd",
		},
		{
			name:     "With_Numbers",
			repoKey:  "repo123",
			appName:  "app123",
			version:  "2.1.0",
			expected: "repo123/app123/2.1.0/release-bundle.json.evd",
		},
		{
			name:     "Empty_Values",
			repoKey:  "",
			appName:  "",
			version:  "",
			expected: "///release-bundle.json.evd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildManifestPathApplication(tt.repoKey, tt.appName, tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateEvidenceApplication_BuildApplicationSubjectPath(t *testing.T) {
	tests := []struct {
		name        string
		project     string
		application string
		version     string
		expectPath  string
	}{
		{
			name:        "Valid_Application_Path",
			project:     "test-project",
			application: "my-app",
			version:     "1.0.0",
			expectPath:  "test-project-release-bundles-v2/my-app/1.0.0/release-bundle.json.evd",
		},
		{
			name:        "Different_Project",
			project:     "another-project",
			application: "another-app",
			version:     "2.1.0",
			expectPath:  "another-project-release-bundles-v2/another-app/2.1.0/release-bundle.json.evd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createTestApplicationCommand()
			cmd.project = tt.project
			cmd.application = tt.application
			cmd.applicationVersion = tt.version
			
			repoKey := tt.project + "-release-bundles-v2" 
			path := buildManifestPathApplication(repoKey, cmd.application, cmd.applicationVersion)
			assert.Equal(t, tt.expectPath, path)
		})
	}
}

func createTestApplicationCommand() *createEvidenceApplication {
	return &createEvidenceApplication{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     &config.ServerDetails{Url: "http://test.com"},
			predicateFilePath: "/test/predicate.json",
			predicateType:     "test-type",
			key:               "test-key",
			keyId:             "test-key-id",
		},
		project:            "test-project",
		application:        "test-app",
		applicationVersion: "1.0.0",
	}
} 