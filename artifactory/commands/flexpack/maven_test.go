package flexpack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateMavenCoordinate verifies that the path-traversal sanitizer accepts legitimate Maven
// coordinates/packaging types and rejects crafted values that could escape the target directory.
func TestValidateMavenCoordinate(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		// Legitimate values - dots are valid separators, hyphens/underscores allowed.
		{name: "simple groupId", value: "com.example", wantErr: false},
		{name: "nested groupId", value: "org.apache.maven.plugins", wantErr: false},
		{name: "artifactId with hyphen", value: "my-app", wantErr: false},
		{name: "release version", value: "1.0.0", wantErr: false},
		{name: "snapshot version", value: "1.2.3-SNAPSHOT", wantErr: false},
		{name: "packaging jar", value: "jar", wantErr: false},
		{name: "packaging bundle", value: "bundle", wantErr: false},

		// Empty string is rejected by the allowlist guard.
		{name: "empty string", value: "", wantErr: true},

		// Path traversal sequences.
		{name: "double dot", value: "..", wantErr: true},
		{name: "parent traversal", value: "../../etc", wantErr: true},
		{name: "embedded double dot", value: "1.0..0", wantErr: true},

		// Path separators.
		{name: "forward slash", value: "com/example", wantErr: true},
		{name: "backslash", value: "com\\example", wantErr: true},
		{name: "absolute path", value: "/etc/passwd", wantErr: true},

		// Null byte and other characters outside the allowlist.
		{name: "null byte", value: "jar\x00", wantErr: true},
		{name: "newline", value: "jar\nwar", wantErr: true},
		{name: "space", value: "my app", wantErr: true},
		{name: "semicolon", value: "jar;rm", wantErr: true},
		{name: "colon", value: "C:jar", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMavenCoordinate(tt.value)
			if tt.wantErr {
				assert.Error(t, err, "expected %q to be rejected", tt.value)
			} else {
				assert.NoError(t, err, "expected %q to be accepted", tt.value)
			}
		})
	}
}

// writePom is a test helper that creates a pom.xml inside a fresh temp dir and returns the dir.
func writePom(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(contents), 0600))
	return dir
}

// TestGetMavenArtifactCoordinates_Valid verifies coordinates are extracted from a well-formed pom.xml,
// including the parent fallback for groupId/version.
func TestGetMavenArtifactCoordinates_Valid(t *testing.T) {
	t.Run("explicit coordinates", func(t *testing.T) {
		dir := writePom(t, `<?xml version="1.0"?>
<project>
  <groupId>com.example</groupId>
  <artifactId>my-app</artifactId>
  <version>1.2.3</version>
</project>`)
		groupId, artifactId, version, err := getMavenArtifactCoordinates(dir)
		require.NoError(t, err)
		assert.Equal(t, "com.example", groupId)
		assert.Equal(t, "my-app", artifactId)
		assert.Equal(t, "1.2.3", version)
	})

	t.Run("parent fallback", func(t *testing.T) {
		dir := writePom(t, `<?xml version="1.0"?>
<project>
  <parent>
    <groupId>com.parent</groupId>
    <version>9.9.9</version>
  </parent>
  <artifactId>child-app</artifactId>
</project>`)
		groupId, artifactId, version, err := getMavenArtifactCoordinates(dir)
		require.NoError(t, err)
		assert.Equal(t, "com.parent", groupId)
		assert.Equal(t, "child-app", artifactId)
		assert.Equal(t, "9.9.9", version)
	})
}

// TestGetMavenArtifactCoordinates_PathTraversal ensures a crafted pom.xml cannot inject traversal
// sequences through any coordinate field.
func TestGetMavenArtifactCoordinates_PathTraversal(t *testing.T) {
	tests := []struct {
		name string
		pom  string
	}{
		{
			name: "malicious groupId",
			pom: `<project>
  <groupId>../../../../etc</groupId>
  <artifactId>app</artifactId>
  <version>1.0.0</version>
</project>`,
		},
		{
			name: "malicious artifactId",
			pom: `<project>
  <groupId>com.example</groupId>
  <artifactId>../evil</artifactId>
  <version>1.0.0</version>
</project>`,
		},
		{
			name: "malicious version",
			pom: `<project>
  <groupId>com.example</groupId>
  <artifactId>app</artifactId>
  <version>../../1.0</version>
</project>`,
		},
		{
			name: "separator in artifactId",
			pom: `<project>
  <groupId>com.example</groupId>
  <artifactId>a/b</artifactId>
  <version>1.0.0</version>
</project>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writePom(t, tt.pom)
			_, _, _, err := getMavenArtifactCoordinates(dir)
			assert.Error(t, err, "crafted pom.xml should be rejected")
		})
	}
}

// TestGetPackagingType covers the default, valid, and sanitized-fallback paths.
func TestGetPackagingType(t *testing.T) {
	t.Run("explicit packaging", func(t *testing.T) {
		dir := writePom(t, `<project><packaging>war</packaging></project>`)
		assert.Equal(t, "war", getPackagingType(dir))
	})

	t.Run("defaults to jar when missing", func(t *testing.T) {
		dir := writePom(t, `<project></project>`)
		assert.Equal(t, "jar", getPackagingType(dir))
	})

	t.Run("defaults to jar when no pom", func(t *testing.T) {
		assert.Equal(t, "jar", getPackagingType(t.TempDir()))
	})

	t.Run("falls back to jar on path traversal", func(t *testing.T) {
		dir := writePom(t, `<project><packaging>../../bin/sh</packaging></project>`)
		assert.Equal(t, "jar", getPackagingType(dir))
	})
}
