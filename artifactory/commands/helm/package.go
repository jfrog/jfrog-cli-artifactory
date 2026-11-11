package helm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/jfrog-client-go/artifactory"
)

var ErrNoChartDirFound = errors.New("no valid Helm chart directory found in the provided paths")

func isChartDir(dir string) bool {
	chartPath := filepath.Join(dir, flexpack.ChartYaml)
	_, err := os.Stat(chartPath)
	return err == nil
}

func handlePackageCommand(buildInfoOld *entities.BuildInfo, args []string, serviceManager artifactory.ArtifactoryServicesManager, buildName, buildNumber, project string) error {
	packagePaths := getPaths(args)
	var chartPaths []string
	// helm package accepts one or more chart directories.
	// we need to ignore other paths such as archive destination or signing keyring.
	for _, path := range packagePaths {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		if isChartDir(absolutePath) {
			chartPaths = append(chartPaths, absolutePath)
		}
	}
	if len(chartPaths) == 0 {
		return ErrNoChartDirFound
	}
	for _, chartPath := range chartPaths {
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
	}
	removeDuplicateDependencies(buildInfoOld)
	if err := saveBuildInfo(buildInfoOld, buildName, buildNumber, project); err != nil {
		return fmt.Errorf("failed to save build info: %w", err)
	}
	return nil
}
