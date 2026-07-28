package apmcommon

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractApmSubcommandOptions_ServerIDSelectsConfiguredServer(t *testing.T) {
	testutil.WithJfrogHome(t)
	require.NoError(t, config.SaveServersConf([]*config.ServerDetails{
		{ServerId: "known-server", Url: "https://known.jfrog.io/"},
	}))

	// server-id present but not the one configured: must error rather than silently falling
	// back to an empty/default server.
	_, err := ExtractApmSubcommandOptions([]string{"--server-id", "does-not-exist"})
	assert.Error(t, err)

	// server-id present and matching a real config: must resolve to that config.
	opts, err := ExtractApmSubcommandOptions([]string{"--server-id", "known-server"})
	require.NoError(t, err)
	assert.Equal(t, "https://known.jfrog.io/", opts.ServerDetails.Url)
}

func TestExtractApmSubcommandOptions_NoServerIDFallsBackToDefaultConfig(t *testing.T) {
	testutil.WithJfrogHome(t) // no servers configured

	opts, err := ExtractApmSubcommandOptions(nil)
	require.NoError(t, err)
	assert.Empty(t, opts.ServerDetails.ArtifactoryUrl)
}

func TestExtractApmSubcommandOptions_PreservesApmNativeFlags(t *testing.T) {
	testutil.WithJfrogHome(t)

	// install/publish/update don't declare --repo (or direct-credential flags) as jf's own -
	// a registry must already be declared, so --repo isn't extracted here and flows through
	// untouched like any other apm-native flag.
	opts, err := ExtractApmSubcommandOptions([]string{
		"--repo", "buk-apm",
		"--registry", "buk-apm",
		"--frozen",
		"uday/pkg-base#1.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"--repo", "buk-apm", "--registry", "buk-apm", "--frozen", "uday/pkg-base#1.0.0"}, opts.RemainingArgs)
}

func TestExtractApmSubcommandOptions_DirectCredentialFlagsFlowThrough(t *testing.T) {
	testutil.WithJfrogHome(t)

	// --url/--user/--password/--access-token are no longer jf's own flags for install/
	// publish/update (matching pnpm/npm/yarn/nuget - auth comes from --server-id or the
	// default configured server only), so they pass straight through like any other
	// apm-native flag, and ServerDetails resolves from the default config.
	opts, err := ExtractApmSubcommandOptions([]string{
		"--url", "https://acme.jfrog.io",
		"--access-token", "my-token",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"--url", "https://acme.jfrog.io", "--access-token", "my-token"}, opts.RemainingArgs)
	assert.Empty(t, opts.ServerDetails.ArtifactoryUrl)
}

func TestExtractApmSubcommandOptions_BuildNameWithoutNumberErrors(t *testing.T) {
	testutil.WithJfrogHome(t)

	_, err := ExtractApmSubcommandOptions([]string{"--build-name", "my-build"})
	assert.Error(t, err)
}
