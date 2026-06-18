package maven

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMavenBuildTool_DiscoverLockfiles_MultiModule(t *testing.T) {
	root := t.TempDir()
	modA := filepath.Join(root, "module-a")
	require.NoError(t, os.MkdirAll(modA, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>module-a</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(modA, "pom.xml"), []byte(`<project><artifactId>mod</artifactId></project>`), 0644))

	tool := NewBuildTool()
	files, err := tool.DiscoverLockfiles(root)
	require.NoError(t, err)
	require.Len(t, files, 2)
}

func TestMavenBuildTool_EnsureLockfiles_ErrorsWhenNoPom(t *testing.T) {
	dir := t.TempDir()
	tool := NewBuildTool()
	_, err := tool.EnsureLockfiles(context.Background(), dir, "resolve", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pom.xml")
}

func TestMavenBuildTool_DiscoverLockfiles_RespectsFileFlag(t *testing.T) {
	repoRoot := t.TempDir()
	sub := filepath.Join(repoRoot, "child")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "pom.xml"), []byte(`<project><artifactId>child</artifactId></project>`), 0644))

	cwd := t.TempDir()
	tool := NewBuildToolWithGoals([]string{"-f", filepath.Join(sub, "pom.xml"), "install"})
	files, err := tool.DiscoverLockfiles(cwd)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "pom.xml", files[0].Path)
}

func TestMavenBuildTool_EnsureLockfiles_NoOpWhenPomsExist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`<project/>`), 0644))
	tool := NewBuildTool()
	bootstrapped, err := tool.EnsureLockfiles(context.Background(), dir, "resolve", nil)
	require.NoError(t, err)
	assert.Empty(t, bootstrapped)
}
