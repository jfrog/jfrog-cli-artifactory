package strategies

import (
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// BuildXStrategy runs docker buildx build for advanced build capabilities
type BuildXStrategy struct {
	containerManager container.ContainerManager
}

func NewBuildXStrategy() *BuildXStrategy {
	return &BuildXStrategy{
		containerManager: container.NewManager(container.DockerClient),
	}
}

func (s *BuildXStrategy) Execute(cmdParams []string, buildConfig *build.BuildConfiguration) error {
	log.Info("Running docker buildx build command (JFROG_RUN_NATIVE=true with buildx)")

	// The cmdParams should already contain "buildx build" or similar
	// Just pass them directly to the container manager
	err := s.containerManager.RunNativeCmd(cmdParams)
	if err != nil {
		return err
	}

	// Check if build-info collection is needed
	toCollect, err := buildConfig.IsCollectBuildInfo()
	if err != nil {
		return err
	}

	if toCollect {
		// BuildX can generate metadata file that could be used for build-info
		// For now, just acknowledge the request
		log.Info("Build-info collection: Not applicable (placeholder)")
		// Future: Parse buildx metadata file and create build-info
		// Could use --metadata-file flag with buildx
	}

	return nil
}
