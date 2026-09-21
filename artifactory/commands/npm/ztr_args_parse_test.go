package npm

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNpmCLIArgs_Prefix(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"--prefix", "sub/pkg"})
	assert.Equal(t, "sub/pkg", opts.prefixDir)
}

func TestParseNpmCLIArgs_CShort(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"-C", "services/api"})
	assert.Equal(t, "services/api", opts.prefixDir)
}

func TestParseNpmCLIArgs_PrefixEquals(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"--prefix=frontend"})
	assert.Equal(t, "frontend", opts.prefixDir)
}

func TestParseNpmCLIArgs_WorkspaceBootstrap(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"--workspace", "@scope/pkg", "-w", "app"})
	assert.Equal(t, []string{"--workspace", "@scope/pkg", "-w", "app"}, opts.bootstrapArgs)
}

func TestParseNpmCLIArgs_ShortWorkspaceConsumesValue(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"-w", "app"})
	assert.Equal(t, []string{"-w", "app"}, opts.bootstrapArgs)
	assert.Empty(t, opts.packageOperands)
}

func TestParseNpmCLIArgs_ShortWorkspaceEquals(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"-w=app"})
	assert.Equal(t, []string{"-w=app"}, opts.bootstrapArgs)
}

func TestParseNpmCLIArgs_WorkspacesBooleanUnchanged(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"--workspaces"})
	assert.Equal(t, []string{"--workspaces"}, opts.bootstrapArgs)
}

func TestParseNpmCLIArgs_RegistryOverride(t *testing.T) {
	opts := parseNpmCLIArgs([]string{"--registry", "https://acme.jfrog.io/artifactory/api/npm/libs-npm/"})
	assert.Equal(t, "https://acme.jfrog.io/artifactory/api/npm/libs-npm/", opts.registryURL)
	assert.Equal(t, []string{"--registry", "https://acme.jfrog.io/artifactory/api/npm/libs-npm/"}, opts.bootstrapArgs)

	opts = parseNpmCLIArgs([]string{"--registry=https://acme.jfrog.io/artifactory/api/npm/libs-npm/"})
	assert.Equal(t, "https://acme.jfrog.io/artifactory/api/npm/libs-npm/", opts.registryURL)
	assert.Equal(t, []string{"--registry=https://acme.jfrog.io/artifactory/api/npm/libs-npm/"}, opts.bootstrapArgs)
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

func TestParseNpmCLIArgs_BootstrapArgs(t *testing.T) {
	assert.Equal(t, []string{"-w", "app"}, parseNpmCLIArgs([]string{"-w", "app"}).bootstrapArgs)
	assert.Empty(t, parseNpmCLIArgs([]string{"-w"}).bootstrapArgs)
}

func TestParseNpmCLIArgs_PackageOperands(t *testing.T) {
	hasPackageOperands := func(args ...string) bool {
		return len(parseNpmCLIArgs(args).packageOperands) > 0
	}
	assert.True(t, hasPackageOperands("lodash"))
	assert.True(t, hasPackageOperands("--save", "lodash"))
	assert.False(t, hasPackageOperands("--verbose"))
	assert.False(t, hasPackageOperands("-w", "app"))
	assert.False(t, hasPackageOperands("--workspace", "@scope/pkg"))
	assert.False(t, hasPackageOperands())
	assert.False(t, hasPackageOperands("--registry", "https://registry.example"))
	assert.False(t, hasPackageOperands("--registry=https://registry.example"))
	assert.False(t, hasPackageOperands("--tag", "next"))
	assert.False(t, hasPackageOperands("--tag=next"))
	assert.False(t, hasPackageOperands("--omit", "dev"))
	assert.False(t, hasPackageOperands("--omit=dev"))
	assert.False(t, hasPackageOperands("--include", "dev"))
	assert.False(t, hasPackageOperands("--save-prefix", "~"))
	assert.True(t, hasPackageOperands("--registry", "https://registry.example", "lodash"))
	assert.True(t, hasPackageOperands("--omit", "dev", "lodash"))
}

func TestStripNpmInstallOnlyArgs_PreservesPositionalBooleanName(t *testing.T) {
	assert.Equal(t, []string{"false"}, stripNpmInstallOnlyArgs([]string{"--save", "false"}))
}
