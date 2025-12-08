package strategies

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateStrategy_Legacy(t *testing.T) {
	os.Unsetenv("JFROG_RUN_NATIVE")

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
	os.Setenv("JFROG_RUN_NATIVE", "true")
	defer os.Unsetenv("JFROG_RUN_NATIVE")

	options := DockerBuildOptions{
		DockerFilePath: "Dockerfile",
		ImageTag:       "test:latest",
	}

	strategy := CreateStrategy(options)
	assert.NotNil(t, strategy)

	_, isRunNative := strategy.(*RunNativeStrategy)
	assert.True(t, isRunNative, "Expected RunNativeStrategy when JFROG_RUN_NATIVE=true")
}
