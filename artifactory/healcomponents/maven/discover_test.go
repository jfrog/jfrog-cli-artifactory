package maven

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverProjectRoot_FromSubmodule(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "module-a")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>module-a</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "pom.xml"), []byte(`<project><artifactId>module-a</artifactId></project>`), 0644))

	got, err := discoverProjectRoot(sub)
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

func TestDiscoverPomPaths_MultiModule(t *testing.T) {
	root := t.TempDir()
	modA := filepath.Join(root, "module-a")
	require.NoError(t, os.MkdirAll(modA, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>module-a</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(modA, "pom.xml"), []byte(`<project/>`), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(modA, "target"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(modA, "target", "pom.xml"), []byte(`<project/>`), 0644))

	paths, err := discoverPomPaths(root)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pom.xml", filepath.Join("module-a", "pom.xml")}, paths)
}

func TestDiscoverPomPaths_ExcludesStrayPomNotInReactor(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "mod-a"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "test", "resources"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>mod-a</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "mod-a", "pom.xml"), []byte(`<project/>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "test", "resources", "pom.xml"), []byte(`<project/>`), 0644))

	paths, err := discoverPomPaths(root)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pom.xml", filepath.Join("mod-a", "pom.xml")}, paths)
}

func TestDiscoverPomPaths_NestedAggregator(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "services", "api"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>services</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "services", "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>api</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "services", "api", "pom.xml"), []byte(`<project/>`), 0644))

	paths, err := discoverPomPaths(root)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"pom.xml",
		filepath.Join("services", "pom.xml"),
		filepath.Join("services", "api", "pom.xml"),
	}, paths)
}

func TestDiscoverProjectRoot_FromFileFlag(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub", "mod")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>sub</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>mod</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "pom.xml"), []byte(`<project/>`), 0644))

	cwd := t.TempDir()
	got, err := discoverProjectRootWithOptions(cwd, discoveryOptions{pomFile: filepath.Join(root, "sub", "pom.xml")})
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

func TestDiscoverPomPaths_ProjectListFilter(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "mod-a"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "mod-b"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<project><packaging>pom</packaging><modules><module>mod-a</module><module>mod-b</module></modules></project>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "mod-a", "pom.xml"), []byte(`<project/>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "mod-b", "pom.xml"), []byte(`<project/>`), 0644))

	paths, err := discoverPomPathsWithOptions(root, discoveryOptions{projectList: []string{"mod-a"}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{filepath.Join("mod-a", "pom.xml")}, paths)
}

func TestDiscoverPomPaths_MavenExampleTestdata(t *testing.T) {
	root := filepath.Join("..", "..", "..", "tests", "testdata", "maven-example")
	if _, err := os.Stat(root); err != nil {
		t.Skip("testdata not available")
	}
	paths, err := discoverPomPaths(root)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"pom.xml",
		filepath.Join("multi1", "pom.xml"),
		filepath.Join("multi2", "pom.xml"),
		filepath.Join("multi3", "pom.xml"),
	}, paths)
}
