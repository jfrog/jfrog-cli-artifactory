package npm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/zerotouchremediation"
)

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
	assert.False(t, isSinglePackageInstall(nil))
}
