package helm

import (
	"encoding/json"
	"fmt"

	"os"
	"path"
	"strconv"
	"strings"
	"time"

	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"

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
	repoKey, subpath, _, resultMap, err := resolveOCIPushArtifacts(registryURL, chartName, chartVersion, serviceManager)
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if project != "" {
		buildProps += fmt.Sprintf(";build.project=%s", project)
	}
	manifestFolderPath := path.Join(subpath, chartName)
	if err = applyBuildPropertiesOnManifestFolder(serviceManager, repoKey, manifestFolderPath, chartVersion, buildProps); err != nil {
		return fmt.Errorf("failed to apply build properties on OCI manifest folder for %s : %s: %w", chartName, chartVersion, err)
	}
	artifactManifest, err := getManifest(resultMap, serviceManager, repoKey)
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
	addArtifactsInBuildInfo(buildInfo, artifacts, chartName, chartVersion)
	removeDuplicateArtifacts(buildInfo)
	return saveBuildInfo(buildInfo, buildName, buildNumber, project)
}

func resolveOCIPushArtifacts(registryURL, chartName, chartVersion string, sm artifactory.ArtifactoryServicesManager) (repoKey, subpath, storagePath string, resultMap map[string]*servicesUtils.ResultItem, err error) {
	rawReference := strings.TrimRight(strings.TrimPrefix(registryURL, oci), "/")
	if !strings.Contains(rawReference, "/") {
		repoKey = extractRepositoryFromHostSubdomain(rawReference)
		if repoKey == "" {
			return "", "", "", nil, fmt.Errorf("could not resolve OCI push repository key for %q", registryURL)
		}
		storagePath = path.Join(chartName, chartVersion)
		resultMap, err = searchPushedArtifacts(sm, repoKey, storagePath)
		if err != nil {
			return "", "", "", nil, fmt.Errorf("failed to search oci layers for %s : %s: %w", chartName, chartVersion, err)
		}
		if len(resultMap) == 0 {
			return "", "", "", nil, fmt.Errorf("no oci artifacts found for repoKey %q and storagePath %q", repoKey, storagePath)
		}
		return repoKey, "", storagePath, resultMap, nil
	}
	ref, err := parseOCIReference(rawReference)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("failed to parse OCI registry URL %q: %w", registryURL, err)
	}
	for _, candidate := range generateRepoCandidates(ref.Registry, ref.Repository) {
		if candidate.repoKey == "" {
			continue
		}
		candidateStoragePath := path.Join(candidate.subpath, chartName, chartVersion)
		candidateResultMap, searchErr := searchPushedArtifacts(sm, candidate.repoKey, candidateStoragePath)
		if searchErr != nil {
			return "", "", "", nil, fmt.Errorf("failed to search oci layers for %s : %s: %w", chartName, chartVersion, searchErr)
		}
		if len(candidateResultMap) == 0 {
			continue
		}
		return candidate.repoKey, candidate.subpath, candidateStoragePath, candidateResultMap, nil
	}
	return "", "", "", nil, fmt.Errorf("could not resolve OCI push repository key for %q", registryURL)
}

// searchPushedArtifacts searches for pushed OCI artifacts using a search pattern.
func searchPushedArtifacts(serviceManager artifactory.ArtifactoryServicesManager, repoKey, storagePath string) (map[string]*servicesUtils.ResultItem, error) {
	aqlQuery := fmt.Sprintf(`{
	  "repo": "%s",
	  "path": "%s"
	}`, repoKey, storagePath)
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

// overwriteReaderWithManifestFolder overwrites the reader's backing files with JSON describing the manifest folder result.
func overwriteReaderWithManifestFolder(reader *content.ContentReader, repoKey, manifestFolderPath, manifestFolderName string) error {
	if reader == nil {
		return fmt.Errorf("reader is nil")
	}
	jsonData := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"repo": repoKey,
				"path": manifestFolderPath,
				"name": manifestFolderName,
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

func newManifestFolderReader(repoKey, manifestFolderPath, manifestFolderName string) (reader *content.ContentReader, cleanup func() error, err error) {
	tmpFile, err := os.CreateTemp("", "jfrog-helm-push-manifest-folder-*.json")
	if err != nil {
		return nil, nil, err
	}
	tmpFilePath := tmpFile.Name()
	if err = tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFilePath)
		return nil, nil, err
	}
	reader = content.NewContentReader(tmpFilePath, content.DefaultKey)
	cleanup = func() error {
		var closeErr error
		ioutils.Close(reader, &closeErr)
		removeErr := os.Remove(tmpFilePath)
		if closeErr != nil {
			return closeErr
		}
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		return nil
	}
	if err = overwriteReaderWithManifestFolder(reader, repoKey, manifestFolderPath, manifestFolderName); err != nil {
		_ = cleanup()
		return nil, nil, err
	}
	reader.Reset()
	return reader, cleanup, nil
}

func applyBuildPropertiesOnManifestFolder(serviceManager artifactory.ArtifactoryServicesManager, repoKey, manifestFolderPath, manifestFolderName, buildProps string) (err error) {
	if buildProps == "" {
		return nil
	}
	reader, cleanup, err := newManifestFolderReader(repoKey, manifestFolderPath, manifestFolderName)
	if err != nil {
		return err
	}
	defer func() {
		cleanupErr := cleanup()
		if err == nil && cleanupErr != nil {
			err = cleanupErr
		}
	}()
	addBuildPropertiesOnArtifacts(serviceManager, reader, buildProps)
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
