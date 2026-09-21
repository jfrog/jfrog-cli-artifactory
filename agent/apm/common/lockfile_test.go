package apmcommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSHA256Hex(t *testing.T) {
	tests := []struct {
		name         string
		resolvedHash string
		want         string
	}{
		{name: "sha256 prefixed", resolvedHash: "sha256:abc123", want: "abc123"},
		{name: "different algo prefix", resolvedHash: "md5:abc123", want: ""},
		{name: "empty", resolvedHash: "", want: ""},
		{name: "no prefix", resolvedHash: "abc123", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SHA256Hex(tt.resolvedHash))
		})
	}
}

func TestApmLockedPackage_DepID(t *testing.T) {
	pkg := ApmLockedPackage{RepoURL: "acme/skills-pack", Version: "2.0.0"}
	assert.Equal(t, "acme/skills-pack:2.0.0", pkg.DepID())
}

func TestApmLockFile_RegistryPackages(t *testing.T) {
	lockfile := &ApmLockFile{
		Dependencies: []ApmLockedPackage{
			{RepoURL: "acme/skills-pack", Source: "registry"},
			{RepoURL: "someone/github-direct", Source: "github"},
			{RepoURL: "acme/prompt-pack", Source: "registry"},
		},
	}
	registryPkgs := lockfile.RegistryPackages()
	require.Len(t, registryPkgs, 2)
	assert.Equal(t, "acme/skills-pack", registryPkgs[0].RepoURL)
	assert.Equal(t, "acme/prompt-pack", registryPkgs[1].RepoURL)
}

func TestLoadLockFile(t *testing.T) {
	tempDir := t.TempDir()
	lockfilePath := filepath.Join(tempDir, ApmLockfileName)
	content := `
lockfile_version: "1"
dependencies:
  - repo_url: acme/skills-pack
    version: 2.0.0
    source: registry
    resolved_hash: sha256:abc123
`
	require.NoError(t, os.WriteFile(lockfilePath, []byte(content), 0644))

	lockfile, err := LoadLockFile(lockfilePath)
	require.NoError(t, err)
	require.Len(t, lockfile.Dependencies, 1)
	assert.Equal(t, "acme/skills-pack", lockfile.Dependencies[0].RepoURL)
}

func TestLoadLockFile_MissingFile(t *testing.T) {
	_, err := LoadLockFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	assert.Error(t, err)
}
