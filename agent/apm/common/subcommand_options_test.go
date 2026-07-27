package apmcommon

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveServerDetails_DirectCredentials(t *testing.T) {
	testutil.WithJfrogHome(t) // isolate from any real jf config on this machine

	sd, err := resolveServerDetails("", "https://acme.jfrog.io", "uday", "", "my-token")
	require.NoError(t, err)
	assert.Equal(t, "uday", sd.User)
	assert.Equal(t, "my-token", sd.AccessToken)
	assert.Equal(t, "https://acme.jfrog.io/artifactory/", sd.ArtifactoryUrl)
}

func TestResolveServerDetails_ServerIDWinsOverDirectCredentials(t *testing.T) {
	testutil.WithJfrogHome(t)
	// At least one real server must exist so GetAllServersConfigs() is non-empty - otherwise
	// GetSpecificConfig short-circuits to an empty ServerDetails regardless of serverID,
	// and this test couldn't tell "server-id consulted and not found" from "no configs at all".
	require.NoError(t, config.SaveServersConf([]*config.ServerDetails{
		{ServerId: "known-server", Url: "https://known.jfrog.io/"},
	}))

	// server-id present but not the one configured: must error rather than silently fall
	// through to the direct-credential branch below it.
	_, err := resolveServerDetails("does-not-exist", "https://acme.jfrog.io", "", "", "ignored-token")
	assert.Error(t, err)

	// server-id present AND matching a real config: must use that config, not the passed
	// direct credentials, proving server-id truly takes precedence.
	sd, err := resolveServerDetails("known-server", "https://acme.jfrog.io", "", "", "ignored-token")
	require.NoError(t, err)
	assert.Equal(t, "https://known.jfrog.io/", sd.Url)
	assert.Empty(t, sd.AccessToken)
}

func TestResolveServerDetails_NoFlagsAtAll(t *testing.T) {
	testutil.WithJfrogHome(t) // no servers configured

	sd, err := resolveServerDetails("", "", "", "", "")
	require.NoError(t, err)
	assert.Empty(t, sd.ArtifactoryUrl)
}

func TestExtractApmSubcommandOptions_PreservesApmNativeFlags(t *testing.T) {
	testutil.WithJfrogHome(t)

	// install/publish/update don't declare --repo as one of jf's own flags (unlike
	// passthrough) - a registry must already be declared, so --repo isn't extracted here and
	// flows through untouched like any other apm-native flag.
	opts, err := ExtractApmSubcommandOptions([]string{
		"--repo", "buk-apm",
		"--registry", "buk-apm",
		"--frozen",
		"uday/pkg-base#1.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"--repo", "buk-apm", "--registry", "buk-apm", "--frozen", "uday/pkg-base#1.0.0"}, opts.RemainingArgs)
}

func TestExtractApmSubcommandOptions_DirectCredentialsFlowThrough(t *testing.T) {
	testutil.WithJfrogHome(t)

	opts, err := ExtractApmSubcommandOptions([]string{
		"--url", "https://acme.jfrog.io",
		"--access-token", "my-token",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-token", opts.ServerDetails.AccessToken)
	assert.Equal(t, "https://acme.jfrog.io/artifactory/", opts.ServerDetails.ArtifactoryUrl)
	assert.Empty(t, opts.RemainingArgs)
}

func TestExtractApmSubcommandOptions_BuildNameWithoutNumberErrors(t *testing.T) {
	testutil.WithJfrogHome(t)

	_, err := ExtractApmSubcommandOptions([]string{"--build-name", "my-build"})
	assert.Error(t, err)
}
