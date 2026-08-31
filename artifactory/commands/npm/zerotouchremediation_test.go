package npm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jfrog/jfrog-client-go/xray/services"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/zerotouchremediation"
)

type countingZTRClient struct {
	calls int
}

func (c *countingZTRClient) GetVersion() (string, error) {
	return zerotouchremediation.ZeroTouchRemediationMinVersion, nil
}

func (c *countingZTRClient) ZeroTouchRemediation(_ services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, bool, error) {
	c.calls++
	return &services.ComponentResolutionResponse{
		Changes: []services.Change{{Package: "lodash"}},
	}, false, nil
}

func TestRunZeroTouchRemediation_DisabledByDefault(t *testing.T) {
	t.Setenv(zerotouchremediation.ZtrComponentsEnabledEnvVar, "")
	ca := &CommonArgs{}
	ca.SetRepo("npm-virtual").SetServerDetails(nil)
	_, remediated, err := ca.runZeroTouchRemediation(t.Context(), "install", t.TempDir(), nil)
	assert.NoError(t, err)
	assert.False(t, remediated)
}

func TestEffectiveNpmCommandAfterRemediation(t *testing.T) {
	nc := &NpmCommand{cmdName: "install", remediatedLockfile: true}
	assert.Equal(t, "ci", nc.effectiveNpmCommand())

	nc.remediatedLockfile = false
	assert.Equal(t, "install", nc.effectiveNpmCommand())

	nc.cmdName = "ci"
	nc.remediatedLockfile = true
	assert.Equal(t, "ci", nc.effectiveNpmCommand())
}

func TestIsSinglePackageInstall(t *testing.T) {
	assert.True(t, isSinglePackageInstall([]string{"lodash"}))
	assert.True(t, isSinglePackageInstall([]string{"--save", "lodash"}))
	assert.False(t, isSinglePackageInstall([]string{"--verbose"}))
	assert.False(t, isSinglePackageInstall([]string{"-w", "app"}))
	assert.False(t, isSinglePackageInstall([]string{"--workspace", "@scope/pkg"}))
	assert.False(t, isSinglePackageInstall([]string{"--prefix", "packages/app"}))
	assert.False(t, isSinglePackageInstall([]string{"--registry", "https://registry.example"}))
	assert.False(t, isSinglePackageInstall([]string{"--tag", "next"}))
	assert.False(t, isSinglePackageInstall([]string{"--omit", "dev"}))
	assert.False(t, isSinglePackageInstall(nil))
}

func TestRunIfEnabled_SkipsSymlinkedLockfile(t *testing.T) {
	t.Setenv(zerotouchremediation.ZtrComponentsEnabledEnvVar, "true")
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.lock")
	require.NoError(t, os.WriteFile(outside, []byte("secret-from-outside"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "package-lock.json")))

	client := &countingZTRClient{}
	_, remediated, err := zerotouchremediation.RunIfEnabled(context.Background(), client, "npm-virtual", NewBuildTool(), "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 0, client.calls)
}
