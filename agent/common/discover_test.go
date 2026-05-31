package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverInstalledSlugs_MissingDir(t *testing.T) {
	got, err := DiscoverInstalledSlugs(filepath.Join(t.TempDir(), "nope"), "plugin-info.json")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestDiscoverInstalledSlugs_FiltersToInstalled(t *testing.T) {
	base := t.TempDir()

	installed := filepath.Join(base, "alpha", jfrogInstallDirName)
	require.NoError(t, os.MkdirAll(installed, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(installed, "plugin-info.json"), []byte(`{}`), 0o644))

	uninstalled := filepath.Join(base, "beta")
	require.NoError(t, os.MkdirAll(uninstalled, 0o755))

	wrongManifest := filepath.Join(base, "gamma", jfrogInstallDirName)
	require.NoError(t, os.MkdirAll(wrongManifest, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wrongManifest, "skill-info.json"), []byte(`{}`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(base, "loose-file"), []byte("not a dir"), 0o644))

	got, err := DiscoverInstalledSlugs(base, "plugin-info.json")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha"}, got)
}

func TestDiscoverInstalledSlugs_SortsOutput(t *testing.T) {
	base := t.TempDir()
	for _, slug := range []string{"zeta", "alpha", "mu"} {
		dir := filepath.Join(base, slug, jfrogInstallDirName)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin-info.json"), []byte(`{}`), 0o644))
	}
	got, err := DiscoverInstalledSlugs(base, "plugin-info.json")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "mu", "zeta"}, got)
}
