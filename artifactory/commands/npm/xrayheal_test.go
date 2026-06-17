package npm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
)


func TestRunComponentResolution_RespectsDisabledEnv(t *testing.T) {
	t.Setenv(healcomponents.HealComponentsDisabledEnvVar, "true")
	ca := &CommonArgs{}
	ca.SetRepo("npm-virtual").SetServerDetails(nil)
	_, healed, err := ca.runXrayComponentHealing(t.Context(), "install", t.TempDir(), nil)
	assert.NoError(t, err)
	assert.False(t, healed)
}

func TestEffectiveNpmCommandAfterHeal(t *testing.T) {
	nc := &NpmCommand{cmdName: "install", healedLockfile: true}
	assert.Equal(t, "ci", nc.effectiveNpmCommand())

	nc.healedLockfile = false
	assert.Equal(t, "install", nc.effectiveNpmCommand())

	nc.cmdName = "ci"
	nc.healedLockfile = true
	assert.Equal(t, "ci", nc.effectiveNpmCommand())
}

func TestIsSinglePackageInstall(t *testing.T) {
	assert.True(t, isSinglePackageInstall([]string{"lodash"}))
	assert.True(t, isSinglePackageInstall([]string{"--save", "lodash"}))
	assert.False(t, isSinglePackageInstall([]string{"--verbose"}))
	assert.False(t, isSinglePackageInstall(nil))
}


func TestNpmBuildTool_DiscoverLockfiles_SingleRootLock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"lock":true}`), 0644))

	tool := NewNpmBuildTool()
	files, err := tool.DiscoverLockfiles(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "package-lock.json", files[0].Path)
	assert.JSONEq(t, `{"lock":true}`, string(files[0].Content))
}

func TestDiscoverProjectRoot_FromWorkspacePackage(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "packages", "app")
	require.NoError(t, os.MkdirAll(pkgDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"workspaces":["packages/*"]}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"name":"app"}`), 0644))

	got, err := discoverProjectRoot(pkgDir)
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

func TestNpmBuildTool_EnsureLockfiles_CiRequiresExistingLock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))

	tool := NewNpmBuildTool()
	_, err := tool.EnsureLockfiles(context.Background(), dir, "ci", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package-lock.json")
}

func TestNpmBuildTool_EnsureLockfiles_InstallBootstrapsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))

	var ran []string
	runner := func(_ context.Context, root string, args ...string) error {
		ran = append(ran, root+":"+strings.Join(args, " "))
		require.NoError(t, os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{"bootstrapped":true}`), 0644))
		return nil
	}

	tool := NewNpmBuildTool()
	_, err := tool.EnsureLockfiles(context.Background(), dir, "install", runner)
	require.NoError(t, err)
	assert.Contains(t, ran[0], "install --package-lock-only")
	_, err = os.Stat(filepath.Join(dir, "package-lock.json"))
	require.NoError(t, err)
}

func TestNpmBuildTool_EnsureLockfiles_SkipsBootstrapWhenLockExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{}`), 0644))

	called := false
	runner := func(context.Context, string, ...string) error {
		called = true
		return nil
	}

	tool := NewNpmBuildTool()
	_, err := tool.EnsureLockfiles(context.Background(), dir, "install", runner)
	require.NoError(t, err)
	assert.False(t, called)
}

func TestNpmBuildTool_EnsureLockfiles_PassesWorkspaceFlags(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))

	var ran []string
	runner := func(_ context.Context, root string, args ...string) error {
		ran = append(ran, strings.Join(args, " "))
		require.NoError(t, os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{}`), 0644))
		return nil
	}

	tool := NewNpmBuildTool()
	_, err := tool.EnsureLockfiles(context.Background(), dir, "publish", runner, "--workspaces")
	require.NoError(t, err)
	assert.Contains(t, ran[0], "--workspaces")
}
