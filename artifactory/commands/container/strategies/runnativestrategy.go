package strategies

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/container/dockerfileutils"
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// RunNativeStrategy runs docker build directly without JFrog enhancements
type RunNativeStrategy struct {
	DockerBuildStrategyBase
}

func NewRunNativeStrategy(options DockerBuildOptions) *RunNativeStrategy {
	return &RunNativeStrategy{
		DockerBuildStrategyBase: DockerBuildStrategyBase{
			containerManager:   container.NewManager(container.DockerClient),
			dockerBuildOptions: options,
		},
	}
}

func (s *RunNativeStrategy) Execute(cmdParams []string, buildConfig *build.BuildConfiguration) error {
	log.Info("Running docker build in native mode (JFROG_RUN_NATIVE=true)")

	// Run native docker build directly
	err := s.GetContainerManager().RunNativeCmd(cmdParams)
	if err != nil {
		return err
	}

	// Check if build-info collection is needed using existing BuildConfiguration method
	toCollect, err := buildConfig.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if toCollect {
		// Collect build-info for docker build
		err = s.collectBuildInfo(cmdParams, buildConfig)
		if err != nil {
			// just warn, no need to fail the build if build info collection fails
			log.Warn("Failed to collect build info. Error:", err)
		}
	}
	return nil
}

func (s *RunNativeStrategy) collectBuildInfo(cmdParams []string, buildConfig *build.BuildConfiguration) error {
	log.Info("Collecting build info...")
	if s.dockerBuildOptions.ImageTag == "" {
		log.Warn("Could not extract image tag from build command")
		return nil
	}

	// Parse Dockerfile to get base images
	dockerfilePath := s.dockerBuildOptions.DockerFilePath
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	baseImageInfos, err := dockerfileutils.ParseDockerfileBaseImages(dockerfilePath)
	if err != nil {
		return errorutils.CheckErrorf("Failed to parse Dockerfile: %s", err.Error())
	}

	if len(baseImageInfos) == 0 {
		log.Info("No base images found in Dockerfile")
		return nil
	}

	// Use server details from the strategy (set by BuildCommand)
	serverDetails := s.GetServerDetails()
	if serverDetails == nil {
		log.Warn("Server configuration not available")
		return nil
	}

	// Create Artifactory service manager
	serviceManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return errorutils.CheckErrorf("Failed to create Artifactory service manager: %s", err.Error())
	}

	// Get build configuration details
	buildName, err := buildConfig.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := buildConfig.GetBuildNumber()
	if err != nil {
		return err
	}
	project := buildConfig.GetProject()

	// Create simplified DockerBuildInfoBuilder (no image or repository needed)
	builder := container.NewDockerBuildInfoBuilder(
		buildName,
		buildNumber,
		project,
		buildConfig.GetModule(),
		serviceManager,
		s.dockerBuildOptions.ImageTag,
		baseImageInfos,
		s.dockerBuildOptions.PushExpected,
		cmdParams,
	)

	// Build the build-info (just pass the image tag for module ID)
	err = builder.Build()
	if err != nil {
		return errorutils.CheckErrorf("Failed to build build-info: %s", err.Error())
	}

	log.Info(fmt.Sprintf("Build-info collected successfully for image: %s", s.dockerBuildOptions.ImageTag))
	return nil
}
