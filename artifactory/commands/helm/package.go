package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"path/filepath"
)

func handlePackageCommand(buildInfoOld *entities.BuildInfo, args []string, serviceManager artifactory.ArtifactoryServicesManager, buildName, buildNumber, project string) error {
	packagePaths := getPaths(args)
	for _, path := range packagePaths {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		buildInfo, err := collectBuildInfoWithFlexPack(absolutePath, buildName, buildNumber)
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
	}
	return nil
}
