package apmcommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, ApmManifestName)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadManifest_NoRegistries(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `
name: my-project
version: 1.0.0
`)
	manifest, err := LoadManifest(path)
	require.NoError(t, err)
	assert.Empty(t, manifest.Registries.Entries)
	assert.Empty(t, manifest.Registries.Default)
}

// TestLoadManifest_RegistriesWithDefault is a regression test: a registries: block with a
// sibling "default: <name>" key - the schema-correct, common shape per
// https://microsoft.github.io/apm/reference/manifest-schema/ (a registries: block without a
// default has no effect on plain owner/repo dependency resolution at all) - used to fail
// yaml.Unmarshal entirely, silently discarding every registry in the block.
func TestLoadManifest_RegistriesWithDefault(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `
name: my-project
version: 1.0.0
registries:
  jf-skills:
    url: https://artifactory.example.com/artifactory/api/agentpackages/jf-skills-local
  default: jf-skills
`)
	manifest, err := LoadManifest(path)
	require.NoError(t, err)
	require.Len(t, manifest.Registries.Entries, 1)
	assert.Equal(t, "https://artifactory.example.com/artifactory/api/agentpackages/jf-skills-local", manifest.Registries.Entries["jf-skills"].URL)
	assert.Equal(t, "jf-skills", manifest.Registries.Default)
}

func TestLoadManifest_RegistriesWithoutDefault(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `
name: my-project
version: 1.0.0
registries:
  jf-skills:
    url: https://artifactory.example.com/artifactory/api/agentpackages/jf-skills-local
`)
	manifest, err := LoadManifest(path)
	require.NoError(t, err)
	require.Len(t, manifest.Registries.Entries, 1)
	assert.Equal(t, "https://artifactory.example.com/artifactory/api/agentpackages/jf-skills-local", manifest.Registries.Entries["jf-skills"].URL)
	assert.Empty(t, manifest.Registries.Default)
}

func TestLoadManifest_MultipleRegistriesWithDefault(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `
name: my-project
version: 1.0.0
registries:
  registry-a:
    url: https://a.example.com/artifactory/api/agentpackages/a-local
  registry-b:
    url: https://b.example.com/artifactory/api/agentpackages/b-local
  default: registry-b
`)
	manifest, err := LoadManifest(path)
	require.NoError(t, err)
	require.Len(t, manifest.Registries.Entries, 2)
	assert.Equal(t, "https://a.example.com/artifactory/api/agentpackages/a-local", manifest.Registries.Entries["registry-a"].URL)
	assert.Equal(t, "https://b.example.com/artifactory/api/agentpackages/b-local", manifest.Registries.Entries["registry-b"].URL)
	assert.Equal(t, "registry-b", manifest.Registries.Default)
}

func TestLoadManifest_MissingFileReturnsEmptyManifest(t *testing.T) {
	manifest, err := LoadManifest(filepath.Join(t.TempDir(), ApmManifestName))
	require.NoError(t, err)
	assert.Empty(t, manifest.Name)
	assert.Empty(t, manifest.Registries.Entries)
}

func TestLoadManifest_MalformedYAMLErrors(t *testing.T) {
	path := writeManifest(t, t.TempDir(), `name: [this is not valid yaml`)
	_, err := LoadManifest(path)
	assert.Error(t, err)
}

func TestApmHostMatches(t *testing.T) {
	tests := []struct {
		name           string
		registryURL    string
		artifactoryURL string
		want           bool
	}{
		{name: "matching host", registryURL: "https://acme.jfrog.io/artifactory/api/agentpackages/my-repo/", artifactoryURL: "https://acme.jfrog.io/artifactory/", want: true},
		{name: "different host", registryURL: "https://other.jfrog.io/artifactory/api/agentpackages/my-repo/", artifactoryURL: "https://acme.jfrog.io/artifactory/", want: false},
		{name: "case-insensitive host", registryURL: "https://ACME.jfrog.io/artifactory/api/agentpackages/my-repo/", artifactoryURL: "https://acme.jfrog.io/artifactory/", want: true},
		{name: "empty registry URL", registryURL: "", artifactoryURL: "https://acme.jfrog.io/artifactory/", want: false},
		{name: "empty artifactory URL", registryURL: "https://acme.jfrog.io/artifactory/api/agentpackages/my-repo/", artifactoryURL: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, apmHostMatches(tt.registryURL, tt.artifactoryURL))
		})
	}
}
