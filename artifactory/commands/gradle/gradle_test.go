package gradle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateInitScript(t *testing.T) {
	config := InitScriptAuthConfig{
		ArtifactoryURL:         "http://example.com/artifactory",
		GradleRepoName:         "example-repo",
		ArtifactoryUsername:    "user",
		ArtifactoryAccessToken: "token",
	}
	script, err := GenerateInitScript(config)
	assert.NoError(t, err)
	assert.Contains(t, script, "http://example.com/artifactory")
	assert.Contains(t, script, "example-repo")
	assert.Contains(t, script, "user")
	assert.Contains(t, script, "token")
	// Verify publishing configuration is included
	assert.Contains(t, script, "maven-publish")
	assert.Contains(t, script, "publishing {")

	// Verify Maven repository configuration
	assert.Contains(t, script, "repositories {")
	assert.Contains(t, script, "maven {")

	// Verify repository names are included for better logging
	assert.Contains(t, script, `name = "Artifactory"`)

	// Verify modern uri() function usage
	assert.Contains(t, script, "url = uri(")
	assert.Contains(t, script, "url uri(")

	// Verify exclusive publishing with clear()
	assert.Contains(t, script, "clear()")
	assert.Contains(t, script, "Clear any existing repositories")

	// Verify metadataSources is not included (uses Gradle defaults)
	assert.NotContains(t, script, "metadataSources")
	assert.NotContains(t, script, "artifact()")
	assert.NotContains(t, script, "mavenPom()")

	// Verify credentials and security configuration
	assert.Contains(t, script, "credentials {")
	assert.Contains(t, script, "allowInsecureProtocol")
	assert.Contains(t, script, "gradleVersion >= GradleVersion.version")
}

func TestWriteInitScript(t *testing.T) {
	// Set up a temporary directory for testing
	tempDir := t.TempDir()
	t.Setenv(UserHomeEnv, tempDir)

	initScript := "test init script content"

	err := WriteInitScript(initScript)
	assert.NoError(t, err)

	// Verify the init script was written to the correct location
	expectedPath := filepath.Join(tempDir, "init.d", InitScriptName)
	content, err := os.ReadFile(expectedPath)
	assert.NoError(t, err)
	assert.Equal(t, initScript, string(content))
}
