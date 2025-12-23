package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildtool "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// CollectHelmBuildInfoWithFlexPack collects Helm build info using FlexPack
func CollectHelmBuildInfoWithFlexPack(workingDir, buildName, buildNumber, project, commandName string, helmArgs []string, serverDetails *config.ServerDetails) error {
	serviceManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}
	buildInfoService := buildtool.CreateBuildInfoService()
	build, err := buildInfoService.GetOrCreateBuildWithProject(buildName, buildNumber, project)
	if err != nil {
		return fmt.Errorf("failed to create build info: %w", err)
	}
	buildInfo, err := build.ToBuildInfo()
	if err != nil {
		return fmt.Errorf("failed to build info: %w", err)
	}
	switch commandName {
	case "push":
		return handlePushCommand(buildInfo, helmArgs, serviceManager, buildName, buildNumber, project)
	case "package":
		return handlePackageCommand(buildInfo, helmArgs, serviceManager, buildName, buildNumber, project)
	case "dependency":
		return handleDependencyCommand(buildInfo, helmArgs, serviceManager, workingDir, buildName, buildNumber, project)
	}
	log.Info("Skipping helm build info because", commandName, "command is not collecting build info")
	return nil
}

// collectBuildInfoWithFlexPack collects build info using FlexPack
func collectBuildInfoWithFlexPack(workingDir, buildName, buildNumber string) (*entities.BuildInfo, error) {
	helmConfig := flexpack.HelmConfig{
		WorkingDirectory: workingDir,
	}
	helmFlex, err := flexpack.NewHelmFlexPack(helmConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Helm FlexPack: %w", err)
	}
	buildInfo, err := helmFlex.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to collect build info with FlexPack: %w", err)
	}
	return buildInfo, nil
}

// saveBuildInfo saves build info to the build instance
func saveBuildInfo(buildInfo *entities.BuildInfo, buildName, buildNumber, project string) error {
	if err := buildtool.SaveBuildInfo(buildName, buildNumber, project, buildInfo); err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: ", err.Error())
		return err
	}
	log.Info("Build info saved locally. Use 'jf rt bp", buildName, buildNumber, "' to publish it to Artifactory.")
	return nil
}
