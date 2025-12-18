package gradle

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestExtractBuildFilePath tests extraction of build file path from Gradle arguments
func TestExtractBuildFilePath(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []string
		expected string
	}{
		// -b flag tests
		{
			name:     "short flag with space",
			tasks:    []string{"clean", "build", "-b", "/path/to/build.gradle"},
			expected: "/path/to/build.gradle",
		},
		{
			name:     "short flag without space",
			tasks:    []string{"clean", "build", "-b/path/to/build.gradle"},
			expected: "/path/to/build.gradle",
		},
		{
			name:     "long flag with equals",
			tasks:    []string{"clean", "--build-file=/path/to/build.gradle", "build"},
			expected: "/path/to/build.gradle",
		},
		{
			name:     "long flag with space",
			tasks:    []string{"--build-file", "/path/to/build.gradle", "clean"},
			expected: "/path/to/build.gradle",
		},
		// -p flag tests (project directory)
		{
			name:     "project dir short flag with space",
			tasks:    []string{"clean", "build", "-p", "/path/to/project"},
			expected: filepath.Join("/path/to/project", "build.gradle"),
		},
		{
			name:     "project dir short flag without space",
			tasks:    []string{"clean", "build", "-p/path/to/project"},
			expected: filepath.Join("/path/to/project", "build.gradle"),
		},
		{
			name:     "project dir long flag with equals",
			tasks:    []string{"clean", "--project-dir=/path/to/project", "build"},
			expected: filepath.Join("/path/to/project", "build.gradle"),
		},
		{
			name:     "project dir long flag with space",
			tasks:    []string{"--project-dir", "/path/to/project", "clean"},
			expected: filepath.Join("/path/to/project", "build.gradle"),
		},
		// No flag tests
		{
			name:     "no build file flag",
			tasks:    []string{"clean", "build", "test"},
			expected: "",
		},
		{
			name:     "empty tasks",
			tasks:    []string{},
			expected: "",
		},
		// Edge cases
		{
			name:     "-b at end without value",
			tasks:    []string{"clean", "build", "-b"},
			expected: "",
		},
		{
			name:     "-p at end without value",
			tasks:    []string{"clean", "build", "-p"},
			expected: "",
		},
		{
			name:     "relative path with -b",
			tasks:    []string{"-b", "subdir/build.gradle", "clean"},
			expected: "subdir/build.gradle",
		},
		{
			name:     "relative path with -p",
			tasks:    []string{"-p", "subdir", "clean"},
			expected: filepath.Join("subdir", "build.gradle"),
		},
		{
			name:     "build file flag first",
			tasks:    []string{"-b/custom/build.gradle", "clean", "build"},
			expected: "/custom/build.gradle",
		},
		{
			name:     "-b flag should not match --build-cache",
			tasks:    []string{"clean", "--build-cache", "build"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBuildFilePath(tt.tasks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractBuildFilePathWindowsPaths tests Windows-style paths if on Windows
func TestExtractBuildFilePathWindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific path tests on non-Windows OS")
	}

	tests := []struct {
		name     string
		tasks    []string
		expected string
	}{
		{
			name:     "Windows absolute path with -b",
			tasks:    []string{"-b", "C:\\Users\\dev\\project\\build.gradle", "clean"},
			expected: "C:\\Users\\dev\\project\\build.gradle",
		},
		{
			name:     "Windows path with -p",
			tasks:    []string{"-p", "C:\\Users\\dev\\project", "clean"},
			expected: filepath.Join("C:\\Users\\dev\\project", "build.gradle"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBuildFilePath(tt.tasks)
			assert.Equal(t, tt.expected, result)
		})
	}
}
