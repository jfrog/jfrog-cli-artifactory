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

func isChartDir(dir string) bool {
	chartPath := filepath.Join(dir, flexpack.ChartYaml)
	_, err := os.Stat(chartPath)
	return !errors.Is(err, os.ErrNotExist)
}

func handlePackageCommand(buildInfoOld *entities.BuildInfo, args []string, serviceManager artifactory.ArtifactoryServicesManager, buildName, buildNumber, project string) error {
	packagePaths := getPaths(args)
	var chartPath string
	// helm package command has only 1 chart path directory.
	// we need to ignore other paths such as archive destination or signing keyring.
	for _, path := range packagePaths {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		if isChartDir(absolutePath) {
			chartPath = absolutePath
			break
		}
	}
	if chartPath == "" {
		return fmt.Errorf("no valid Helm chart directory found in the provided paths")
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
