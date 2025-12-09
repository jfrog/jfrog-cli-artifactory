package strategies

import (
	"github.com/jfrog/jfrog-client-go/utils/log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateStrategy_Legacy(t *testing.T) {
	_ = os.Unsetenv("JFROG_RUN_NATIVE")

	options := DockerBuildOptions{
		DockerFilePath: "Dockerfile",
		ImageTag:       "test:latest",
	}

	strategy := CreateStrategy(options)
	assert.NotNil(t, strategy)

	_, isLegacy := strategy.(*LegacyStrategy)
	assert.True(t, isLegacy, "Expected LegacyStrategy when JFROG_RUN_NATIVE is not set")
}

func TestCreateStrategy_RunNative(t *testing.T) {
	_ = os.Setenv("JFROG_RUN_NATIVE", "true")
	defer func() {
		err := os.Unsetenv("JFROG_RUN_NATIVE")
		if err != nil {
			log.Warn(err)
		}
	}()

	options := DockerBuildOptions{
		DockerFilePath: "Dockerfile",
		ImageTag:       "test:latest",
	}

	strategy := CreateStrategy(options)
	assert.NotNil(t, strategy)

	_, isRunNative := strategy.(*RunNativeStrategy)
	assert.True(t, isRunNative, "Expected RunNativeStrategy when JFROG_RUN_NATIVE=true")
}
