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

// TestParseUserHomeFromJavaOutput tests the parsing of user.home from Java output
func TestParseUserHomeFromJavaOutput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name: "valid output with user.home",
			input: `Property settings:
    file.encoding = UTF-8
    java.home = /opt/java
    user.dir = /home/user/project
    user.home = /home/user
    user.name = testuser`,
			expected:    "/home/user",
			expectError: false,
		},
		{
			name: "valid output with spaces around equals",
			input: `Property settings:
    user.home = /Users/developer
    user.name = dev`,
			expected:    "/Users/developer",
			expectError: false,
		},
		{
			name:        "valid output with Windows path",
			input:       "Property settings:\n    user.home = C:\\Users\\Developer\n    user.name = dev",
			expected:    "C:\\Users\\Developer",
			expectError: false,
		},
		{
			name: "valid output with root user (container scenario)",
			input: `Property settings:
    user.home = /root
    user.name = root`,
			expected:    "/root",
			expectError: false,
		},
		{
			name:        "empty output",
			input:       "",
			expected:    "",
			expectError: true,
		},
		{
			name: "output without user.home",
			input: `Property settings:
    java.home = /opt/java
    user.name = testuser`,
			expected:    "",
			expectError: true,
		},
		{
			name:        "malformed line without equals",
			input:       "user.home /home/user",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseUserHomeFromJavaOutput(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestWriteInitScriptUsesJavaUserHome specifically tests the new case where
// Java's user.home is used instead of $HOME when GRADLE_USER_HOME is not set.
// This is the fix for container environments where $HOME differs from Java's user.home.
func TestWriteInitScriptUsesJavaUserHome(t *testing.T) {
	// Get Java's user.home - skip if Java is not available
	javaHome, err := GetJavaUserHome()
	if err != nil {
		t.Skip("Java not available, skipping test")
	}

	// Ensure GRADLE_USER_HOME is NOT set so we exercise the Java user.home path
	t.Setenv(UserHomeEnv, "")

	// Set $HOME to a DIFFERENT temp directory to simulate container environment
	// where $HOME differs from Java's user.home
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	initScript := "test init script for java user.home case"

	err = WriteInitScript(initScript)
	assert.NoError(t, err)

	// Verify: init script should be in Java's user.home, NOT in fake $HOME
	expectedPath := filepath.Join(javaHome, ".gradle", "init.d", InitScriptName)
	wrongPath := filepath.Join(fakeHome, ".gradle", "init.d", InitScriptName)

	// Should exist in Java user.home (the correct location)
	content, err := os.ReadFile(expectedPath)
	assert.NoError(t, err, "Init script should be written to Java user.home: %s", expectedPath)
	assert.Equal(t, initScript, string(content))

	// Should NOT exist in fake $HOME (this was the bug!)
	_, err = os.Stat(wrongPath)
	assert.True(t, os.IsNotExist(err), "Init script should NOT be written to $HOME: %s", wrongPath)

	// Cleanup - use t.Cleanup for deferred cleanup
	t.Cleanup(func() {
		_ = os.Remove(expectedPath)
	})
}

// TestWriteInitScriptFallsBackToHome tests that WriteInitScript falls back to $HOME
// when both GRADLE_USER_HOME is not set AND Java is not available
func TestWriteInitScriptFallsBackToHome(t *testing.T) {
	// Ensure GRADLE_USER_HOME is NOT set
	t.Setenv(UserHomeEnv, "")

	// Set a known HOME for the fallback case
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Temporarily modify PATH to ensure Java is not found
	// This simulates an environment where Java is not installed
	// Note: t.Setenv automatically restores the original value after the test
	t.Setenv("PATH", "/nonexistent")

	initScript := "test init script for HOME fallback case"

	err := WriteInitScript(initScript)
	assert.NoError(t, err)

	// Verify the init script was written to $HOME fallback location
	fallbackPath := filepath.Join(tempDir, ".gradle", "init.d", InitScriptName)
	content, err := os.ReadFile(fallbackPath)
	assert.NoError(t, err, "Init script should be written to $HOME fallback: %s", fallbackPath)
	assert.Equal(t, initScript, string(content))
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
