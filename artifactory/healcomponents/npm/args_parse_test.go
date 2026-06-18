package npm

import (
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
	got, err := effectiveStartDir("/repo", discoveryOptions{publishPath: "/repo/packages/foo"})
	assert.NoError(t, err)
	assert.Equal(t, "/repo/packages/foo", got)
}

func TestEffectiveStartDir_PrefixFromArgs(t *testing.T) {
	got, err := effectiveStartDir("/repo", discoveryOptions{prefixDir: "sub"})
	assert.NoError(t, err)
	assert.Equal(t, "/repo/sub", got)
}

func TestBootstrapArgsFrom(t *testing.T) {
	assert.Equal(t, []string{"-w"}, BootstrapArgsFrom([]string{"install", "-w"}))
}
