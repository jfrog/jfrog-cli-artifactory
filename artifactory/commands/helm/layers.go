package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	artutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

type manifest struct {
	Config manifestConfig `json:"config,omitempty"`
	Layers []layer        `json:"layers,omitempty"`
}

type manifestConfig struct {
	Digest string `json:"digest,omitempty"`
}

type layer struct {
	Digest    string `json:"digest,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}

// updateDependencyManifestAndConfigFile adds manifest.json and config files for all OCI dependencies
func updateDependencyOCILayersInBuildInfo(buildInfo *entities.BuildInfo, serviceManager artifactory.ArtifactoryServicesManager) {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return
	}
	for moduleIdx := range buildInfo.Modules {
		module := &buildInfo.Modules[moduleIdx]
		if len(module.Dependencies) == 0 {
			continue
		}
		processedDependencies := &[]entities.Dependency{}
		processModuleDependencies(module, serviceManager, processedDependencies)
		if len(*processedDependencies) > 0 {
			module.Dependencies = *processedDependencies
		}
	}
}

// processModuleDependencies processes all dependencies in a module
func processModuleDependencies(module *entities.Module, serviceManager artifactory.ArtifactoryServicesManager, processedDependencies *[]entities.Dependency) {
	moduleDependencies := module.Dependencies
	for _, dependency := range moduleDependencies {
		processDependency(dependency, serviceManager, processedDependencies)
	}
}

// processDependency processes a single dependency and returns whether to increment the index
func processDependency(dep entities.Dependency, serviceManager artifactory.ArtifactoryServicesManager, processedDependencies *[]entities.Dependency) {
	if !isOCIRepository(dep.Repository) {
		updateClassicHelmDependencyChecksums(dep, serviceManager, processedDependencies)
		return
	}
	addOCILayersForDependency(dep, serviceManager, processedDependencies)
}

// addOCILayersForDependency adds all OCI layers for a dependency that has checksums
func addOCILayersForDependency(dep entities.Dependency, serviceManager artifactory.ArtifactoryServicesManager, processedDependencies *[]entities.Dependency) {
	versionPath := extractDependencyPath(dep.Id)
	if versionPath == "" {
		log.Error("Failed to find a valid version for dependency: ", dep.Id)
		return
	}
	repoName := extractRepositoryNameFromURL(dep.Repository)
	if repoName == "" {
		log.Error("Failed to find a valid repository for dependency: ", dep.Id)
		return
	}
	aqlQuery := fmt.Sprintf(`{
	  "repo": "%s",
	  "path": "%s"
	}`, repoName, versionPath)
	resultMap, err := searchOCIArtifactsByAQL(serviceManager, aqlQuery)
	if err != nil {
		log.Debug("Failed to search OCI artifacts for dependency ", dep.Id, " : ", err)
		return
	}
	if len(resultMap) == 0 {
		log.Debug("Did not find any OCI artifacts for dependency: ", dep.Id)
		return
	}
	dependencyManifest, err := getManifest(resultMap, serviceManager, repoName)
	if err != nil {
		log.Debug("Failed to get manifest")
		return
	}
	if dependencyManifest == nil {
		log.Debug("Could not find image manifest in Artifactory")
		return
	}
	layerDigests := make([]struct{ Digest, MediaType string }, len(dependencyManifest.Layers))
	for i, layerItem := range dependencyManifest.Layers {
		layerDigests[i] = struct{ Digest, MediaType string }{
			Digest:    layerItem.Digest,
			MediaType: layerItem.MediaType,
		}
	}
	dependencyLayers, err := ocicontainer.ExtractLayersFromManifestData(resultMap, dependencyManifest.Config.Digest, layerDigests)
	if err != nil {
		return
	}
	for _, depLayer := range dependencyLayers {
		*processedDependencies = append(*processedDependencies, entities.Dependency{
			Id:         depLayer.Name,
			Repository: depLayer.Repo,
			Checksum: entities.Checksum{
				Sha1:   depLayer.Actual_Sha1,
				Md5:    depLayer.Actual_Md5,
				Sha256: depLayer.Sha256,
			},
		})
	}
}

// updateClassicHelmDependencyChecksums searches for a classic Helm chart .tgz file in Artifactory
func updateClassicHelmDependencyChecksums(dep entities.Dependency, serviceManager artifactory.ArtifactoryServicesManager, processedDependencies *[]entities.Dependency) {
	if dep.Id == "" {
		return
	}
	if !dep.IsEmpty() {
		if dep.Md5 != "" && dep.Sha1 != "" && dep.Sha256 != "" {
			*processedDependencies = append(*processedDependencies, dep)
			return
		}
	}
	depName, depVersion, err := parseDependencyID(dep.Id)
	if err != nil {
		return
	}
	repoName := extractRepositoryNameFromURL(dep.Repository)
	if repoName == "" {
		return
	}
	log.Debug("Classic Helm dependency ", dep.Id, " has no checksums, searching for .tgz file in Artifactory")
	resultItem, err := searchClassicHelmChart(serviceManager, repoName, depName, depVersion)
	if err != nil {
		log.Debug("Classic Helm chart not found for dependency ", dep.Id, " : ", err)
		return
	}
	*processedDependencies = append(*processedDependencies, resultItem.ToDependency())
	log.Debug("Found classic Helm chart for dependency ", dep.Id, " : ", resultItem.Name, " (sha256: ", dep.Sha256, ")")
}

// parseDependencyID parses dependency ID into name and version
func parseDependencyID(depId string) (string, string, error) {
	parts := strings.Split(depId, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid dependency ID format: %s", depId)
	}
	return parts[0], parts[1], nil
}

// searchClassicHelmChart searches for classic Helm chart .tgz file
func searchClassicHelmChart(serviceManager artifactory.ArtifactoryServicesManager, repoName, depName, depVersion string) (*servicesUtils.ResultItem, error) {
	searchPattern := fmt.Sprintf("%s/%s-%s*.tgz", repoName, depName, depVersion)
	log.Debug("Searching for classic Helm chart with pattern: ", searchPattern)
	searchParams := services.NewSearchParams()
	searchParams.Pattern = searchPattern
	searchParams.Recursive = false
	reader, err := serviceManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to search for classic Helm chart: %w", err)
	}
	var closeErr error
	defer func() {
		if closeErr != nil {
			log.Debug("Failed to close search reader: ", closeErr)
		}
		ioutils.Close(reader, &closeErr)
	}()
	for item := new(servicesUtils.ResultItem); reader.NextRecord(item) == nil; item = new(servicesUtils.ResultItem) {
		if item.Type != "folder" && strings.HasSuffix(item.Name, ".tgz") {
			return item, nil
		}
	}
	return nil, fmt.Errorf("classic Helm chart .tgz file not found")
}

// searchOCIArtifactsByAQL searches for OCI artifacts using a AQL
func searchOCIArtifactsByAQL(serviceManager artifactory.ArtifactoryServicesManager, aqlQuery string) (map[string]*servicesUtils.ResultItem, error) {
	searchParams := services.SearchParams{
		CommonParams: &servicesUtils.CommonParams{
			Aql: servicesUtils.Aql{ItemsFind: aqlQuery},
		},
	}
	searchParams.Recursive = false
	reader, err := serviceManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to search for OCI artifacts: %w", err)
	}
	var closeErr error
	defer func() {
		if closeErr != nil {
			log.Debug("Failed to close search reader: ", closeErr)
		}
		ioutils.Close(reader, &closeErr)
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

func getManifest(resultMap map[string]*servicesUtils.ResultItem, serviceManager artifactory.ArtifactoryServicesManager, repo string) (layerManifest *manifest, err error) {
	if len(resultMap) == 0 {
		return
	}
	manifestSearchResult, ok := resultMap["manifest.json"]
	if !ok {
		return
	}
	err = downloadLayer(*manifestSearchResult, &layerManifest, serviceManager, repo)
	return
}

// Download the content of layer search result.
func downloadLayer(searchResult servicesUtils.ResultItem, result interface{}, serviceManager artifactory.ArtifactoryServicesManager, repo string) error {
	searchResult.Repo = repo
	return artutils.RemoteUnmarshal(serviceManager, searchResult.GetItemRelativePath(), result)
}
