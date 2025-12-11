package helm

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"strconv"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func handlePushCommand(buildInfo *entities.BuildInfo, helmArgs []string, serviceManager artifactory.ArtifactoryServicesManager, buildName, buildNumber, project string) error {
	filePath, registryURL := getPushChartPathAndRegistryURL(helmArgs)
	if filePath == "" || registryURL == "" {
		return fmt.Errorf("invalid helm chart path or registry url")
	}
	chartName, chartVersion, err := getChartDetails(filePath)
	if err != nil {
		return fmt.Errorf("could not extract chart name/version from artifact %s: %w", filePath, err)
	}
	appendModuleAndBuildAgentIfAbsent(buildInfo, chartName, chartVersion)
	log.Debug("Processing push command for chart: ", filePath, " to registry: ", registryURL)
	repoName := extractRepositoryNameFromURL(registryURL)
	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if project != "" {
		buildProps += fmt.Sprintf(";build.project=%s", project)
	}
	searchPattern := fmt.Sprintf("%s/%s/%s/", repoName, chartName, chartVersion)
	resultMap, err := searchDependencyOCIFilesByPath(serviceManager, searchPattern, buildProps)
	if err != nil {
		return fmt.Errorf("failed to search OCI artifacts for %s : %s: %w", chartName, chartVersion, err)
	}
	if len(resultMap) == 0 {
		return fmt.Errorf("no OCI artifacts found for chart: %s : %s", chartName, chartVersion)
	}
	artifactManifest, err := getManifest(resultMap, serviceManager, repoName)
	if err != nil {
		return fmt.Errorf("failed to get manifest")
	}
	if artifactManifest == nil {
		return fmt.Errorf("could not find image manifest in Artifactory")
	}
	layerDigests := make([]struct{ Digest, MediaType string }, len(artifactManifest.Layers))
	for i, layerItem := range artifactManifest.Layers {
		layerDigests[i] = struct{ Digest, MediaType string }{
			Digest:    layerItem.Digest,
			MediaType: layerItem.MediaType,
		}
	}
	artifactsLayers, err := ocicontainer.ExtractLayersFromManifestData(resultMap, artifactManifest.Config.Digest, layerDigests)
	if err != nil {
		return fmt.Errorf("failed to extract OCI artifacts for %s : %s: %w", chartName, chartVersion, err)
	}
	var artifacts []entities.Artifact
	for _, artLayer := range artifactsLayers {
		artifacts = append(artifacts, artLayer.ToArtifact())
	}
	addArtifactsInBuildInfo(buildInfo, artifacts, chartName, chartVersion)
	removeDuplicateArtifacts(buildInfo)
	return saveBuildInfo(buildInfo, buildName, buildNumber, project)
}
