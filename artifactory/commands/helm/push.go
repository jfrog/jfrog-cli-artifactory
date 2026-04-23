package helm

import (
	"encoding/json"
	"fmt"
	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"os"
	"strconv"
	"strings"
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
	repoName, subPath := extractRepoAndSubPath(registryURL)
	ociChartPath := buildOCIChartPath(subPath, chartName, chartVersion)
	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if project != "" {
		buildProps += fmt.Sprintf(";build.project=%s", project)
	}
	resultMap, err := searchPushedArtifacts(serviceManager, repoName, ociChartPath, buildProps)
	if err != nil {
		return fmt.Errorf("failed to search oci layers for %s: %w", ociChartPath, err)
	}
	if len(resultMap) == 0 {
		return fmt.Errorf("no oci layers found for chart path: %s", ociChartPath)
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

// searchPushedArtifacts searches for pushed OCI artifacts using a search pattern.
// ociChartPath is the full path under the repository, e.g. "subdir1/subdir2/chart-name/1.0.0".
func searchPushedArtifacts(serviceManager artifactory.ArtifactoryServicesManager, repoName, ociChartPath string, buildProperties string) (map[string]*servicesUtils.ResultItem, error) {
	aqlQuery := fmt.Sprintf(`{
	  "repo": "%s",
	  "path": "%s"
	}`, repoName, ociChartPath)
	searchParams := services.SearchParams{
		CommonParams: &servicesUtils.CommonParams{
			Aql: servicesUtils.Aql{ItemsFind: aqlQuery},
		},
	}
	searchParams.Recursive = false
	reader, err := serviceManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to search for pushed OCI artifacts: %w", err)
	}
	var closeErr error
	defer func() {
		ioutils.Close(reader, &closeErr)
		if closeErr != nil {
			log.Debug("Failed to close search reader: ", closeErr)
		}
	}()
	artifacts := make(map[string]*servicesUtils.ResultItem)
	for item := new(servicesUtils.ResultItem); reader.NextRecord(item) == nil; item = new(servicesUtils.ResultItem) {
		if item.Type != "folder" && (item.Name == "manifest.json" || strings.HasPrefix(item.Name, "sha256__")) {
			itemCopy := *item
			artifacts[item.Name] = &itemCopy
			log.Debug("Found OCI artifact: ", item.Name, " (path: ", item.Path, "/", item.Name, ", sha256: ", item.Sha256, ")")
		}
	}
	if buildProperties != "" {
		manifestPath, manifestName := splitOCIChartPath(ociChartPath)
		err = overwriteReaderWithManifestFolder(reader, repoName, manifestPath, manifestName)
		if err != nil {
			return nil, err
		}
		reader.Reset()
		addBuildPropertiesOnArtifacts(serviceManager, reader, buildProperties)
	}
	return artifacts, nil
}

// buildOCIChartPath constructs the full Artifactory storage path for an OCI chart
// by joining the optional subpath (nested directory segments from the OCI URL),
// chartName, and chartVersion. For example:
//
//	subpath="team/app", chartName="mychart", chartVersion="1.0.0"
//	  → "team/app/mychart/1.0.0"
//
//	subpath="", chartName="mychart", chartVersion="1.0.0"
//	  → "mychart/1.0.0"
func buildOCIChartPath(subpath, chartName, chartVersion string) string {
	if subpath != "" {
		return fmt.Sprintf("%s/%s/%s", subpath, chartName, chartVersion)
	}
	return fmt.Sprintf("%s/%s", chartName, chartVersion)
}

// splitOCIChartPath splits a full OCI chart path into the parent directory
// (path) and the leaf directory (name), suitable for Artifactory's folder
// result item format. For "team/app/mychart/1.0.0" it returns
// ("team/app/mychart", "1.0.0").
func splitOCIChartPath(ociChartPath string) (path, name string) {
	if idx := strings.LastIndex(ociChartPath, "/"); idx >= 0 {
		return ociChartPath[:idx], ociChartPath[idx+1:]
	}
	return ociChartPath, ""
}

// updateReaderContents updates the reader contents by writing the specified JSON value to all file paths
func overwriteReaderWithManifestFolder(reader *content.ContentReader, repo, path, name string) error {
	if reader == nil {
		return fmt.Errorf("reader is nil")
	}
	jsonData := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"repo": repo,
				"path": path,
				"name": name,
				"type": "folder",
			},
		},
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	filesPaths := reader.GetFilesPaths()
	for _, filePath := range filesPaths {
		err := os.WriteFile(filePath, jsonBytes, 0644)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to write JSON to file %s: %s", filePath, err))
			continue
		}
		log.Debug(fmt.Sprintf("Successfully updated file %s with JSON content", filePath))
	}
	return nil
}

func addBuildPropertiesOnArtifacts(serviceManager artifactory.ArtifactoryServicesManager, reader *content.ContentReader, buildProps string) {
	propsParams := services.PropsParams{
		Reader:      reader,
		Props:       buildProps,
		IsRecursive: true,
	}
	_, _ = serviceManager.SetProps(propsParams)
}
