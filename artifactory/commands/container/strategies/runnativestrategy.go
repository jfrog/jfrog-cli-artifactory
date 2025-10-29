package strategies

import (
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// RunNativeStrategy runs docker build directly without JFrog enhancements
type RunNativeStrategy struct {
	containerManager container.ContainerManager
}

func NewRunNativeStrategy() *RunNativeStrategy {
	return &RunNativeStrategy{
		containerManager: container.NewManager(container.DockerClient),
	}
}

func (s *RunNativeStrategy) Execute(cmdParams []string, buildConfig *build.BuildConfiguration) error {
	log.Info("Running docker build in native mode (JFROG_RUN_NATIVE=true)")

	// Run native docker build directly
	err := s.containerManager.RunNativeCmd(cmdParams)
	if err != nil {
		return err
	}

	// Check if build-info collection is needed using existing BuildConfiguration method
	toCollect, err := buildConfig.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if toCollect {
		// In native mode, we acknowledge build-info flags but don't actually collect
		log.Info("Build-info collection: Not applicable (placeholder)")
	}
	return nil
}
