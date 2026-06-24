package npm

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNpmCLIArgs_Prefix(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"install", "--prefix", "sub/pkg"})
	assert.Equal(t, "sub/pkg", opts.prefixDir)
}

func TestParseNpmCLIArgs_CShort(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"-C", "services/api", "ci"})
	assert.Equal(t, "services/api", opts.prefixDir)
}

func TestParseNpmCLIArgs_PrefixEquals(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"install", "--prefix=frontend"})
	assert.Equal(t, "frontend", opts.prefixDir)
}

func TestParseNpmCLIArgs_WorkspaceBootstrap(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"install", "--workspace", "@scope/pkg", "-w"})
	assert.Equal(t, []string{"--workspace", "@scope/pkg", "-w"}, opts.bootstrapArgs)
}

func TestEffectiveStartDir_PublishPathOverridesCwd(t *testing.T) {
	root := t.TempDir()
	publishPath := filepath.Join(root, "packages", "foo")
	got, err := effectiveStartDir(root, discoveryOptions{publishPath: publishPath})
	assert.NoError(t, err)
	assert.Equal(t, publishPath, got)
}

func TestEffectiveStartDir_PublishPathUnixAbsolute(t *testing.T) {
	got, err := effectiveStartDir("/repo", discoveryOptions{publishPath: "/repo/packages/foo"})
	assert.NoError(t, err)
	want, err := filepath.Abs("/repo/packages/foo")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestEffectiveStartDir_PrefixFromArgs(t *testing.T) {
	root := t.TempDir()
	got, err := effectiveStartDir(root, discoveryOptions{prefixDir: "sub"})
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "sub"), got)
}

func TestEffectiveStartDir_PrefixFromArgsUnixRoot(t *testing.T) {
	got, err := effectiveStartDir("/repo", discoveryOptions{prefixDir: "sub"})
	assert.NoError(t, err)
	root, err := filepath.Abs("/repo")
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "sub"), got)
}

func TestBootstrapArgsFrom(t *testing.T) {
	assert.Equal(t, []string{"-w"}, BootstrapArgsFrom([]string{"install", "-w"}))
}
