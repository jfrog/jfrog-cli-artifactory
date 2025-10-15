package strategies

import (
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// LegacyStrategy runs docker build with traditional JFrog approach
type LegacyStrategy struct {
	DockerBuildStrategyBase
}

func NewLegacyStrategy(options DockerBuildOptions) *LegacyStrategy {
	return &LegacyStrategy{
		DockerBuildStrategyBase: DockerBuildStrategyBase{
			containerManager:   container.NewManager(container.DockerClient),
			dockerBuildOptions: options,
		},
	}
}

func (s *LegacyStrategy) Execute(cmdParams []string, buildConfig *build.BuildConfiguration) error {
	log.Debug("Running docker build in legacy mode (traditional JFrog approach)")

	// Legacy mode currently just runs the native docker command
	// Future enhancements would include:
	// - Proxying through Artifactory
	// - Layer caching
	// - Security scanning
	// - NO build-info collection (legacy mode does not support build-info for docker build)

	err := s.GetContainerManager().RunNativeCmd(cmdParams)
	if err != nil {
		return err
	}

	// Legacy strategy does NOT collect build-info for docker build
	// Build-info is only collected for docker push/pull in legacy mode
	return nil
}
