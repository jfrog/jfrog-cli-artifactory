package npm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNpmBuildTool_DiscoverLockfiles_SingleRootLock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"lock":true}`), 0644))

	tool := NewBuildTool()
	files, err := tool.DiscoverLockfiles(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "package-lock.json", files[0].Path)
	assert.JSONEq(t, `{"lock":true}`, string(files[0].Content))
}

func TestNpmBuildTool_DiscoverLockfiles_Shrinkwrap(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"a"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "npm-shrinkwrap.json"), []byte(`{"s":1}`), 0644))

	tool := NewBuildTool()
	files, err := tool.DiscoverLockfiles(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "npm-shrinkwrap.json", files[0].Path)
}

func TestNpmBuildTool_DiscoverLockfiles_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.lock")
	require.NoError(t, os.WriteFile(outside, []byte("secret-from-outside"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "package-lock.json")))

	files, err := NewBuildTool().DiscoverLockfiles(dir)
	require.Error(t, err)
	assert.Empty(t, files)
}

func TestNpmBuildTool_EnsureLockfiles_SkipsWhenShrinkwrapExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"a"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "npm-shrinkwrap.json"), []byte(`{}`), 0644))

	called := false
	runner := func(context.Context, string, ...string) error { called = true; return nil }

	tool := NewBuildTool()
	_, err := tool.EnsureLockfiles(context.Background(), dir, "install", runner)
	require.NoError(t, err)
	assert.False(t, called)
}

func TestNpmBuildTool_EnsureLockfiles_CiRequiresExistingLock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))

	tool := NewBuildTool()
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

	tool := NewBuildTool()
	bootstrapped, err := tool.EnsureLockfiles(context.Background(), dir, "install", runner)
	require.NoError(t, err)
	assert.Equal(t, []string{"package-lock.json"}, bootstrapped)
	assert.Contains(t, ran[0], "install --package-lock-only")
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

	tool := NewBuildTool()
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

	tool := NewBuildTool()
	_, err := tool.EnsureLockfiles(context.Background(), dir, "install", runner, "--workspaces")
	require.NoError(t, err)
	assert.Contains(t, ran[0], "--workspaces")
}

func TestNpmBuildTool_RelevantCommands(t *testing.T) {
	assert.Equal(t, []string{"install", "ci"}, NewBuildTool().RelevantCommands())
}
