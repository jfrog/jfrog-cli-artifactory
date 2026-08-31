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
	opts := parseNpmCLIArgs([]string{"install", "--workspace", "@scope/pkg", "-w", "app"})
	assert.Equal(t, []string{"--workspace", "@scope/pkg", "-w", "app"}, opts.bootstrapArgs)
}

func TestParseNpmCLIArgs_ShortWorkspaceConsumesValue(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"-w", "app"})
	assert.Equal(t, []string{"-w", "app"}, opts.bootstrapArgs)
	assert.Empty(t, opts.packageOperands)
}

func TestParseNpmCLIArgs_ShortWorkspaceEquals(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"install", "-w=app"})
	assert.Equal(t, []string{"-w=app"}, opts.bootstrapArgs)
}

func TestParseNpmCLIArgs_WorkspacesBooleanUnchanged(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"install", "--workspaces"})
	assert.Equal(t, []string{"--workspaces"}, opts.bootstrapArgs)
}

func TestParseNpmCLIArgs_PackageOperandsSkipOptionValues(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"--save", "lodash"})
	assert.Equal(t, []string{"lodash"}, opts.packageOperands)

	opts = parseNpmCLIArgs([]string{"--prefix", "packages/app", "lodash"})
	assert.Equal(t, []string{"lodash"}, opts.packageOperands)
	assert.Equal(t, "packages/app", opts.prefixDir)

	opts = parseNpmCLIArgs([]string{"--verbose"})
	assert.Empty(t, opts.packageOperands)
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
	assert.Equal(t, []string{"-w", "app"}, BootstrapArgsFrom([]string{"install", "-w", "app"}))
	assert.Empty(t, BootstrapArgsFrom([]string{"install", "-w"}))
}

func TestHasPackageOperands(t *testing.T) {
	assert.True(t, HasPackageOperands([]string{"lodash"}))
	assert.True(t, HasPackageOperands([]string{"--save", "lodash"}))
	assert.False(t, HasPackageOperands([]string{"--verbose"}))
	assert.False(t, HasPackageOperands([]string{"-w", "app"}))
	assert.False(t, HasPackageOperands([]string{"--workspace", "@scope/pkg"}))
	assert.False(t, HasPackageOperands(nil))
	assert.False(t, HasPackageOperands([]string{"--registry", "https://registry.example"}))
	assert.False(t, HasPackageOperands([]string{"--registry=https://registry.example"}))
	assert.False(t, HasPackageOperands([]string{"--tag", "next"}))
	assert.False(t, HasPackageOperands([]string{"--tag=next"}))
	assert.False(t, HasPackageOperands([]string{"--omit", "dev"}))
	assert.False(t, HasPackageOperands([]string{"--omit=dev"}))
	assert.True(t, HasPackageOperands([]string{"--registry", "https://registry.example", "lodash"}))
	assert.True(t, HasPackageOperands([]string{"--omit", "dev", "lodash"}))
}
