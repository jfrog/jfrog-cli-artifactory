package strategies

import (
	"github.com/jfrog/build-info-go/flexpack"
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type DockerBuildOptions struct {
	DockerFilePath string
	ImageTag       string
	PushExpected   bool
}

// BuildStrategy defines the interface for different build execution strategies
type BuildStrategy interface {
	Execute(cmdParams []string, buildConfig *build.BuildConfiguration) error
	SetServerDetails(serverDetails *config.ServerDetails)
}

// DockerBuildStrategyBase contains common fields for all Docker build strategies
type DockerBuildStrategyBase struct {
	dockerBuildOptions DockerBuildOptions
	containerManager   container.ContainerManager
	serverDetails      *config.ServerDetails
}

// SetServerDetails sets the server details for the strategy
func (bs *DockerBuildStrategyBase) SetServerDetails(serverDetails *config.ServerDetails) {
	bs.serverDetails = serverDetails
}

// GetServerDetails returns the server details for the strategy
func (bs *DockerBuildStrategyBase) GetServerDetails() *config.ServerDetails {
	return bs.serverDetails
}

// GetContainerManager returns the container manager for the strategy
func (bs *DockerBuildStrategyBase) GetContainerManager() container.ContainerManager {
	return bs.containerManager
}

func CreateStrategy(options DockerBuildOptions) BuildStrategy {
	if flexpack.IsFlexPackEnabled() {
		log.Debug("Using RunNative Strategy (JFROG_RUN_NATIVE=true)")
		return NewRunNativeStrategy(options)
	}

	log.Debug("Using Legacy Strategy (traditional JFrog approach)")
	return NewLegacyStrategy(options)
}
