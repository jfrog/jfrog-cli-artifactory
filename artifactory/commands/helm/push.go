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
	ensureBuildAgent(buildInfo)
	log.Debug("Processing push command for chart: ", filePath, " to registry: ", registryURL)

	repoName, ociChartPath, resultMap, err := resolveOCIPushArtifacts(serviceManager, registryURL, chartName, chartVersion)
	if err != nil {
		return err
	}

	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if project != "" {
		buildProps += fmt.Sprintf(";build.project=%s", project)
	}
	applyBuildPropertiesOnManifestFolder(serviceManager, repoName, ociChartPath, buildProps)

	artifactManifest, err := getManifest(resultMap, serviceManager, repoName)
	if err != nil {
		return fmt.Errorf("failed to get manifest: %w", err)
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
	helmModule := &entities.Module{
		Id:        fmt.Sprintf("%s:%s", chartName, chartVersion),
		Type:      "helm",
		Artifacts: artifacts,
	}
	appendModuleInExistingBuildInfo(buildInfo, helmModule)
	removeDuplicateArtifacts(buildInfo)
	return saveBuildInfo(buildInfo, buildName, buildNumber, project)
}

// resolveOCIPushArtifacts generates plausible Artifactory repo/subpath
// candidates for the given OCI registryURL and validates each by searching
// for the pushed chart's OCI layers. Returns the winning repo key, the full
// storage path, and the artifact map on success.
func resolveOCIPushArtifacts(serviceManager artifactory.ArtifactoryServicesManager, registryURL, chartName, chartVersion string) (repoName, ociChartPath string, resultMap map[string]*servicesUtils.ResultItem, err error) {
	candidates := generateOCIRepoCandidates(registryURL)
	if len(candidates) == 0 {
		err = fmt.Errorf("could not determine Artifactory repository from URL: %s", registryURL)
		return
	}

	for _, candidate := range candidates {
		if candidate.repoKey == "" {
			continue
		}
		storagePath := buildOCIChartPath(candidate.subpath, chartName, chartVersion)
		results, searchErr := searchPushedArtifacts(serviceManager, candidate.repoKey, storagePath)
		if searchErr != nil {
			err = fmt.Errorf("failed to search OCI layers for %s/%s: %w", candidate.repoKey, storagePath, searchErr)
			return
		}
		if len(results) > 0 {
			repoName = candidate.repoKey
			ociChartPath = storagePath
			resultMap = results
			log.Debug("Resolved OCI push artifacts: repo=", repoName, " path=", ociChartPath)
			return
		}
		log.Debug("No OCI artifacts found for candidate: repo=", candidate.repoKey, " path=", storagePath)
	}

	err = fmt.Errorf("no OCI artifacts found for chart %s:%s in any candidate repository", chartName, chartVersion)
	return
}

// searchPushedArtifacts searches for pushed OCI artifacts using AQL.
// ociChartPath is the full path under the repository, e.g. "subdir1/subdir2/chart-name/1.0.0".
func searchPushedArtifacts(serviceManager artifactory.ArtifactoryServicesManager, repoName, ociChartPath string) (map[string]*servicesUtils.ResultItem, error) {
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

// applyBuildPropertiesOnManifestFolder sets build properties recursively on
// the manifest folder for a pushed OCI chart. It creates a synthetic content
// reader pointing at the resolved manifest folder path.
func applyBuildPropertiesOnManifestFolder(serviceManager artifactory.ArtifactoryServicesManager, repoName, ociChartPath, buildProps string) {
	if buildProps == "" {
		return
	}
	manifestPath, manifestName := splitOCIChartPath(ociChartPath)
	reader, err := newManifestFolderReader(repoName, manifestPath, manifestName)
	if err != nil {
		log.Warn("Failed to create manifest folder reader for build properties: ", err)
		return
	}
	defer func() {
		ioutils.Close(reader, &err)
	}()
	propsParams := services.PropsParams{
		Reader:      reader,
		Props:       buildProps,
		IsRecursive: true,
	}
	_, err = serviceManager.SetProps(propsParams)
	if err != nil {
		log.Warn("Failed to set build properties on manifest folder: ", err)
	}
}

// newManifestFolderReader creates a content reader containing a single
// synthetic folder item pointing at the OCI manifest directory.
func newManifestFolderReader(repo, path, name string) (*content.ContentReader, error) {
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
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	tmpFile, err := os.CreateTemp("", "manifest-folder-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err = tmpFile.Write(jsonBytes); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			log.Debug("Failed to close temp file: ", closeErr)
		}
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	if err = tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}
	return content.NewContentReader(tmpFile.Name(), "results"), nil
}
