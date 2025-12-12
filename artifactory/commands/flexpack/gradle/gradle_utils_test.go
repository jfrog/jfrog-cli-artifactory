package flexpack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDelimiter(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{'{', true},
		{'}', true},
		{'(', true},
		{')', true},
		{';', true},
		{',', true},
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'\r', true},
		{'a', false},
		{'1', false},
		{'_', false},
		{'.', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			assert.Equal(t, tt.expected, isDelimiter(tt.char))
		})
	}
}

func TestIsWhitespace(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'\r', true},
		{'a', false},
		{'0', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			assert.Equal(t, tt.expected, isWhitespace(tt.char))
		})
	}
}

func TestIsEscaped(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		index    int
		expected bool
	}{
		{
			name:     "Single backslash - escaped",
			content:  `\"hello`,
			index:    1,
			expected: true,
		},
		{
			name:     "Double backslash - not escaped",
			content:  `\\"hello`,
			index:    2,
			expected: false,
		},
		{
			name:     "Triple backslash - escaped",
			content:  `\\\"hello`,
			index:    3,
			expected: true,
		},
		{
			name:     "No backslash - not escaped",
			content:  `"hello`,
			index:    0,
			expected: false,
		},
		{
			name:     "Index at start",
			content:  `hello`,
			index:    0,
			expected: false,
		},
		{
			name:     "Backslash in middle",
			content:  `hel\"lo`,
			index:    4,
			expected: true,
		},
		{
			name:     "Four backslashes - not escaped",
			content:  `\\\\"hello`,
			index:    4,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEscaped(tt.content, tt.index)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetGradleUserHome(t *testing.T) {
	t.Run("With GRADLE_USER_HOME set", func(t *testing.T) {
		originalValue := os.Getenv(envGradleUserHome)
		defer func() {
			if originalValue != "" {
				_ = os.Setenv(envGradleUserHome, originalValue)
			} else {
				_ = os.Unsetenv(envGradleUserHome)
			}
		}()

		_ = os.Setenv(envGradleUserHome, "/custom/gradle/home")
		result := getGradleUserHome()
		assert.Equal(t, "/custom/gradle/home", result)
	})

	t.Run("Without GRADLE_USER_HOME - uses default", func(t *testing.T) {
		originalValue := os.Getenv(envGradleUserHome)
		defer func() {
			if originalValue != "" {
				_ = os.Setenv(envGradleUserHome, originalValue)
			} else {
				_ = os.Unsetenv(envGradleUserHome)
			}
		}()

		_ = os.Unsetenv(envGradleUserHome)
		result := getGradleUserHome()

		// Should return ~/.gradle
		homeDir, err := os.UserHomeDir()
		if err == nil {
			assert.Equal(t, filepath.Join(homeDir, ".gradle"), result)
		}
	})

	t.Run("Empty GRADLE_USER_HOME - uses default", func(t *testing.T) {
		originalValue := os.Getenv(envGradleUserHome)
		defer func() {
			if originalValue != "" {
				_ = os.Setenv(envGradleUserHome, originalValue)
			} else {
				_ = os.Unsetenv(envGradleUserHome)
			}
		}()

		_ = os.Setenv(envGradleUserHome, "")
		result := getGradleUserHome()

		homeDir, err := os.UserHomeDir()
		if err == nil {
			assert.Equal(t, filepath.Join(homeDir, ".gradle"), result)
		}
	})
}

func TestFindGradleFile(t *testing.T) {
	t.Run("Find Groovy build file", func(t *testing.T) {
		tmpDir := t.TempDir()
		groovyPath := filepath.Join(tmpDir, "build.gradle")
		err := os.WriteFile(groovyPath, []byte("// groovy"), 0644)
		assert.NoError(t, err)

		path, isKts, err := findGradleFile(tmpDir, "build")
		assert.NoError(t, err)
		assert.False(t, isKts)
		assert.Equal(t, groovyPath, path)
	})

	t.Run("Find Kotlin build file", func(t *testing.T) {
		tmpDir := t.TempDir()
		ktsPath := filepath.Join(tmpDir, "build.gradle.kts")
		err := os.WriteFile(ktsPath, []byte("// kotlin"), 0644)
		assert.NoError(t, err)

		path, isKts, err := findGradleFile(tmpDir, "build")
		assert.NoError(t, err)
		assert.True(t, isKts)
		assert.Equal(t, ktsPath, path)
	})

	t.Run("Groovy takes precedence over Kotlin", func(t *testing.T) {
		tmpDir := t.TempDir()
		groovyPath := filepath.Join(tmpDir, "build.gradle")
		ktsPath := filepath.Join(tmpDir, "build.gradle.kts")
		err := os.WriteFile(groovyPath, []byte("// groovy"), 0644)
		assert.NoError(t, err)
		err = os.WriteFile(ktsPath, []byte("// kotlin"), 0644)
		assert.NoError(t, err)

		path, isKts, err := findGradleFile(tmpDir, "build")
		assert.NoError(t, err)
		assert.False(t, isKts)
		assert.Equal(t, groovyPath, path)
	})

	t.Run("No file found", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, _, err := findGradleFile(tmpDir, "build")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no build.gradle")
	})

	t.Run("Find settings file", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "settings.gradle")
		err := os.WriteFile(settingsPath, []byte("// settings"), 0644)
		assert.NoError(t, err)

		path, isKts, err := findGradleFile(tmpDir, "settings")
		assert.NoError(t, err)
		assert.False(t, isKts)
		assert.Equal(t, settingsPath, path)
	})

	t.Run("Find init file", func(t *testing.T) {
		tmpDir := t.TempDir()
		initPath := filepath.Join(tmpDir, "init.gradle")
		err := os.WriteFile(initPath, []byte("// init"), 0644)
		assert.NoError(t, err)

		path, isKts, err := findGradleFile(tmpDir, "init")
		assert.NoError(t, err)
		assert.False(t, isKts)
		assert.Equal(t, initPath, path)
	})

	t.Run("Directory does not exist", func(t *testing.T) {
		_, _, err := findGradleFile("/nonexistent/path", "build")
		assert.Error(t, err)
	})
}

func TestValidateWorkingDirectory(t *testing.T) {
	t.Run("Empty working directory", func(t *testing.T) {
		err := validateWorkingDirectory("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
	})

	t.Run("Valid directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := validateWorkingDirectory(tmpDir)
		assert.NoError(t, err)
	})

	t.Run("Non-existent directory", func(t *testing.T) {
		err := validateWorkingDirectory("/nonexistent/path/xyz")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid working directory")
	})

	t.Run("File instead of directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "file.txt")
		err := os.WriteFile(filePath, []byte("content"), 0644)
		assert.NoError(t, err)

		err = validateWorkingDirectory(filePath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}
