package common

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadInstalledPluginVersion_PrefersManifest(t *testing.T) {
	dir := makePluginDir(t, `{"name":"web","version":"1.0.0"}`)
	require.NoError(t, agentcommon.WriteInstallInfoManifest(dir, PluginInfoManifestFile, PluginInfoManifest{
		Repo:             "r",
		Slug:             "web",
		InstalledVersion: "2.0.0",
		Scope:            "project",
		Agent:            "claude",
	}))
	v, err := ReadInstalledPluginVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", v)
}

func TestReadInstalledPluginVersion_FallsBackToPluginJSON(t *testing.T) {
	dir := makePluginDir(t, `{"name":"web","version":"1.2.3"}`)
	v, err := ReadInstalledPluginVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", v)
}

func TestReadInstalledPluginVersion_ManifestEmptyUsesPluginJSON(t *testing.T) {
	dir := makePluginDir(t, `{"name":"web","version":"1.0.0"}`)
	require.NoError(t, agentcommon.WriteInstallInfoManifest(dir, PluginInfoManifestFile, PluginInfoManifest{
		Repo:             "r",
		Slug:             "web",
		InstalledVersion: "",
		Scope:            "project",
		Agent:            "claude",
	}))
	v, err := ReadInstalledPluginVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", v)
}

func TestReadInstalledPluginVersion_NoVersionField(t *testing.T) {
	dir := makePluginDir(t, `{"name":"web"}`)
	v, err := ReadInstalledPluginVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "", v)
}

func TestReadInstalledPluginVersion_NotInstalled(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-plugin")
	_, err := ReadInstalledPluginVersion(missing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "expected fs.ErrNotExist, got %v", err)
}

func TestReadInstalledPluginVersion_DirWithoutManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	_, err := ReadInstalledPluginVersion(dir)
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "expected fs.ErrNotExist, got %v", err)
}

func TestReadInstalledPluginVersion_CorruptManifestFallsBackToPluginJSON(t *testing.T) {
	dir := makePluginDir(t, `{"name":"web","version":"3.4.5"}`)
	manifestDir := filepath.Join(dir, ".jfrog")
	require.NoError(t, os.MkdirAll(manifestDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, PluginInfoManifestFile), []byte("not json"), 0o644))
	v, err := ReadInstalledPluginVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "3.4.5", v)
}

func TestDiscoverInstalledPluginSlugs_MatchesReadInstalledPluginVersion(t *testing.T) {
	base := t.TempDir()
	_ = makePluginDirInRoot(t, base, "alpha", `{"name":"alpha","version":"1.0.0"}`)

	betaDir := filepath.Join(base, "beta")
	manifestDir := filepath.Join(betaDir, ".jfrog")
	require.NoError(t, os.MkdirAll(manifestDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, PluginInfoManifestFile), []byte(`{"installedVersion":"1.0.0"}`), 0o644))

	empty := filepath.Join(base, "gamma")
	require.NoError(t, os.MkdirAll(empty, 0o755))

	got, err := DiscoverInstalledPluginSlugs(base)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, got)
}

func TestDiscoverInstalledPluginSlugs_SortsOutput(t *testing.T) {
	base := t.TempDir()
	for _, slug := range []string{"zeta", "alpha"} {
		_ = makePluginDirInRoot(t, base, slug, fmt.Sprintf(`{"name":%q,"version":"1.0.0"}`, slug))
	}
	got, err := DiscoverInstalledPluginSlugs(base)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "zeta"}, got)
}

func makePluginDirInRoot(t *testing.T, root, slug, manifest string) string {
	t.Helper()
	dir := filepath.Join(root, slug)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644))
	return dir
}

func makePluginDir(t *testing.T, manifest string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "web")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644))
	return dir
}
