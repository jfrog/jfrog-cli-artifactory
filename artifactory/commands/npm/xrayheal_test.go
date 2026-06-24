package npm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
)

func TestRunComponentResolution_RespectsDisabledEnv(t *testing.T) {
	t.Setenv(healcomponents.HealComponentsDisabledEnvVar, "true")
	ca := &CommonArgs{}
	ca.SetRepo("npm-virtual").SetServerDetails(nil)
	_, healed, err := ca.runXrayComponentHealing(t.Context(), "install", t.TempDir(), nil)
	assert.NoError(t, err)
	assert.False(t, healed)
}

func TestEffectiveNpmCommandAfterHeal(t *testing.T) {
	nc := &NpmCommand{cmdName: "install", healedLockfile: true}
	assert.Equal(t, "ci", nc.effectiveNpmCommand())

	nc.healedLockfile = false
	assert.Equal(t, "install", nc.effectiveNpmCommand())

	nc.cmdName = "ci"
	nc.healedLockfile = true
	assert.Equal(t, "ci", nc.effectiveNpmCommand())
}

func TestIsSinglePackageInstall(t *testing.T) {
	assert.True(t, isSinglePackageInstall([]string{"lodash"}))
	assert.True(t, isSinglePackageInstall([]string{"--save", "lodash"}))
	assert.False(t, isSinglePackageInstall([]string{"--verbose"}))
	assert.False(t, isSinglePackageInstall(nil))
}
