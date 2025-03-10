package container

import (
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/container"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type BuildDockerCreateCommand struct {
	ContainerCommandBase
	manifestSha256 string
}

func NewBuildDockerCreateCommand() *BuildDockerCreateCommand {
	return &BuildDockerCreateCommand{}
}

// Set tag and manifest sha256 of an image in Artifactory.
// This file can be generated by Kaniko using the'--image-name-with-digest-file' flag
// or by buildx CLI using '--metadata-file' flag.
// Tag and Sha256 will be used later on to search the image in Artifactory.
func (bdc *BuildDockerCreateCommand) SetImageNameWithDigest(filePath string) (err error) {
	bdc.image, bdc.manifestSha256, err = container.GetImageTagWithDigest(filePath)
	return
}

func (bdc *BuildDockerCreateCommand) Run() error {
	if err := bdc.init(); err != nil {
		return err
	}
	serverDetails, err := bdc.ServerDetails()
	if err != nil {
		return err
	}
	buildName, err := bdc.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := bdc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}
	project := bdc.BuildConfiguration().GetProject()
	serviceManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return err
	}
	repo, err := bdc.GetRepo()
	if err != nil {
		return err
	}
	if err = build.SaveBuildGeneralDetails(buildName, buildNumber, project); err != nil {
		return err
	}
	builder, err := container.NewRemoteAgentBuildInfoBuilder(bdc.image, repo, buildName, buildNumber, project, serviceManager, bdc.manifestSha256)
	if err != nil {
		return err
	}
	buildInfo, err := builder.Build(bdc.BuildConfiguration().GetModule())
	if err != nil {
		return err
	}
	return build.SaveBuildInfo(buildName, buildNumber, project, buildInfo)
}

func (bdc *BuildDockerCreateCommand) CommandName() string {
	return "rt_build_docker_create"
}

func (bdc *BuildDockerCreateCommand) ServerDetails() (*config.ServerDetails, error) {
	return bdc.serverDetails, nil
}
