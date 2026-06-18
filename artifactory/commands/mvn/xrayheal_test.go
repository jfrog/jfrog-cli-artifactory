package mvn

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestRunXrayComponentHealing_RespectsDisabledEnv(t *testing.T) {
	t.Setenv(healcomponents.HealComponentsDisabledEnvVar, "true")
	mu := NewMvnUtils().SetGoals([]string{"install"}).SetConfig(viper.New())
	_, healed, err := mu.runXrayComponentHealing(t.Context(), t.TempDir(), nil)
	assert.NoError(t, err)
	assert.False(t, healed)
}
