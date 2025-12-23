package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
)

func handleDependencyCommand(buildInfoOld *entities.BuildInfo, args []string, serviceManager artifactory.ArtifactoryServicesManager, workingDir, buildName, buildNumber, project string) error {
	chartPath := workingDir
	chartPaths := getPaths(args)
	if len(chartPaths) >= 2 {
		chartPath = chartPaths[1]
	}
	buildInfo, err := collectBuildInfoWithFlexPack(chartPath, buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect build info: %w", err)
	}
	if buildInfo == nil {
		return fmt.Errorf("no build info collected, skipping further processing")
	}
	updateDependencyOCILayersInBuildInfo(buildInfo, serviceManager)
	if len(buildInfo.Modules) > 0 {
		appendModuleInExistingBuildInfo(buildInfoOld, &buildInfo.Modules[0])
	}
	removeDuplicateDependencies(buildInfoOld)
	err = saveBuildInfo(buildInfoOld, buildName, buildNumber, project)
	if err != nil {
		return fmt.Errorf("failed to save build info")
	}
	return nil
}
