package apmcommon

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractApmSubcommandOptions_PreservesApmNativeFlags(t *testing.T) {
	testutil.WithJfrogHome(t)

	// install/publish/update don't declare --repo, --server-id, or direct-credential flags as
	// jf's own - a registry and server must already be declared/configured, so none of these
	// are extracted here; they flow through untouched like any other apm-native flag.
	opts, err := ExtractApmSubcommandOptions([]string{
		"--repo", "buk-apm",
		"--server-id", "my-server",
		"--registry", "buk-apm",
		"--frozen",
		"uday/pkg-base#1.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"--repo", "buk-apm", "--server-id", "my-server", "--registry", "buk-apm", "--frozen", "uday/pkg-base#1.0.0"},
		opts.RemainingArgs)
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
	assert.Equal(t, []string{"uday/pkg-base#1.0.0"}, opts.RemainingArgs)

	buildName, err := opts.BuildConfig.GetBuildName()
	require.NoError(t, err)
	assert.Equal(t, "my-build", buildName)
}

func TestExtractApmSubcommandOptions_BuildNameWithoutNumberErrors(t *testing.T) {
	testutil.WithJfrogHome(t)

	_, err := ExtractApmSubcommandOptions([]string{"--build-name", "my-build"})
	assert.Error(t, err)
}
