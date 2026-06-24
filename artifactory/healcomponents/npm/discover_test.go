package npm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestDiscoverProjectRoot_PrefixFlag(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "frontend")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "package.json"), []byte(`{"name":"fe"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "package-lock.json"), []byte(`{}`), 0644))

	got, err := discoverProjectRootWithOptions(root, discoveryOptions{prefixDir: "frontend"})
	require.NoError(t, err)
	assert.Equal(t, sub, got)
}

func TestDiscoverProjectRoot_ShrinkwrapPrecedence(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"lock":1}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "npm-shrinkwrap.json"), []byte(`{"shrink":1}`), 0644))

	name, err := lockfileNameInDir(dir)
	require.NoError(t, err)
	assert.Equal(t, shrinkwrapFileName, name)
}

func TestDiscoverProjectRoot_IndependentPackageLock(t *testing.T) {
	root := t.TempDir()
	svc := filepath.Join(root, "svc-a")
	require.NoError(t, os.MkdirAll(svc, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"root-no-ws"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(svc, "package.json"), []byte(`{"name":"svc-a"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(svc, "package-lock.json"), []byte(`{}`), 0644))

	got, err := discoverProjectRoot(svc)
	require.NoError(t, err)
	assert.Equal(t, svc, got)
}

func TestDiscoverProjectRoot_NoProjectFound(t *testing.T) {
	dir := t.TempDir()
	_, err := discoverProjectRoot(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package.json")
}

func TestDiscoverProjectRoot_IgnoresStrayPackageJSONWhenLockAbove(t *testing.T) {
	root := t.TempDir()
	svc := filepath.Join(root, "svc")
	stray := filepath.Join(svc, "samples")
	require.NoError(t, os.MkdirAll(stray, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(svc, "package.json"), []byte(`{"name":"svc"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(svc, "package-lock.json"), []byte(`{}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(stray, "package.json"), []byte(`{"name":"sample"}`), 0644))

	got, err := discoverProjectRoot(stray)
	require.NoError(t, err)
	assert.Equal(t, svc, got)
}

func TestDiscoverProjectRoot_PublishPath(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "packages", "lib")
	require.NoError(t, os.MkdirAll(pkg, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"lib"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkg, "package-lock.json"), []byte(`{}`), 0644))

	got, err := discoverProjectRootWithOptions(root, discoveryOptions{publishPath: filepath.Join("packages", "lib")})
	require.NoError(t, err)
	assert.Equal(t, pkg, got)
}
