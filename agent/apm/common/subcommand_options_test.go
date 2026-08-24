package apmcommon

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractApmSubcommandOptions_PreservesApmNativeFlags(t *testing.T) {
	testutil.WithJfrogHome(t)

	// install/publish/update don't declare --repo or direct-credential flags as jf's own - a
	// registry must already be declared/configured, so those flow through untouched like any
	// other apm-native flag. --server-id IS jf's own flag (selects which configured JFrog
	// server to use) and is extracted here, not forwarded.
	opts, err := ExtractApmSubcommandOptions([]string{
		"--repo", "buk-apm",
		"--server-id", "my-server",
		"--registry", "buk-apm",
		"--frozen",
		"uday/pkg-base#1.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"--repo", "buk-apm", "--registry", "buk-apm", "--frozen", "uday/pkg-base#1.0.0"},
		opts.ApmNativeArgs)
	assert.Equal(t, "my-server", opts.ServerID)
}

func TestExtractApmSubcommandOptions_ExtractsBuildInfoFlags(t *testing.T) {
	testutil.WithJfrogHome(t)

	opts, err := ExtractApmSubcommandOptions([]string{
		"--build-name", "my-build",
		"--build-number", "1",
		"--module", "my-module",
		"--project", "my-project",
		"uday/pkg-base#1.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"uday/pkg-base#1.0.0"}, opts.ApmNativeArgs)

	buildName, err := opts.BuildConfig.GetBuildName()
	require.NoError(t, err)
	assert.Equal(t, "my-build", buildName)
}

func TestExtractApmSubcommandOptions_BuildNameWithoutNumberErrors(t *testing.T) {
	testutil.WithJfrogHome(t)

	_, err := ExtractApmSubcommandOptions([]string{"--build-name", "my-build"})
	assert.Error(t, err)
}
