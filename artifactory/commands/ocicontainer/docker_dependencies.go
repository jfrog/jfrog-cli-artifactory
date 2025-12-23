package ocicontainer

import (
	"errors"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	"strings"
	"sync"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type DockerDependenciesBuilder struct {
	dockerImages   []DockerImage
	serviceManager artifactory.ArtifactoryServicesManager
}

func NewDockerDependenciesBuilder(dockerImages []DockerImage, serviceManager artifactory.ArtifactoryServicesManager) *DockerDependenciesBuilder {
	return &DockerDependenciesBuilder{
		dockerImages:   dockerImages,
		serviceManager: serviceManager,
	}
}

// getDependencies collects dependencies for all base images in parallel
func (ddp *DockerDependenciesBuilder) getDependencies() ([]buildinfo.Dependency, error) {
	var wg sync.WaitGroup
	errChan := make(chan error, len(ddp.dockerImages))
	dependencyResultChan := make(chan []utils.ResultItem, len(ddp.dockerImages))

	for _, baseImage := range ddp.dockerImages {
		wg.Add(1)
		go func(img DockerImage) {
			defer wg.Done()
			resultItems, err := ddp.collectDetailsForBaseImage(img)
			if err != nil {
				errChan <- err
				return
			}
			dependencyResultChan <- resultItems
		}(baseImage)
	}

	wg.Wait()
	close(errChan)
	close(dependencyResultChan)

	var errorList []error
	for err := range errChan {
		errorList = append(errorList, err)
	}

	if len(errorList) > 0 {
		return []buildinfo.Dependency{}, fmt.Errorf("errors occurred during build info collection: %v", errors.Join(errorList...))
	}

	var allDependencyResultItems []utils.ResultItem
	for resultItems := range dependencyResultChan {
		allDependencyResultItems = append(allDependencyResultItems, resultItems...)
	}

	return ddp.createDependenciesFromResults(allDependencyResultItems), nil
}

// collectDetailsForBaseImage collects layer details for a single base image
func (ddp *DockerDependenciesBuilder) collectDetailsForBaseImage(baseImage DockerImage) ([]utils.ResultItem, error) {
	log.Debug(fmt.Sprintf("Collecting details for image: %s", baseImage.Image))

	image := NewImage(baseImage.Image)
	imageTag, err := image.GetImageTag()
	if err != nil {
		return []utils.ResultItem{}, err
	}
	imageName, err := image.GetImageShortName()
	if err != nil {
		return []utils.ResultItem{}, err
	}

	// Handle digest-based images (sha256:xxx) - skip unnecessary API calls
	if strings.HasPrefix(imageTag, "sha256:") {
		return ddp.collectDetailsForDigestBasedImage(image, imageName, imageTag)
	}

	// Tag-based image flow
	remoteRepo, dockerManifestType, _, err := GetRemoteRepoAndManifestTypeWithLeadSha(baseImage.Image, ddp.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	manifestSha, err := baseImage.GetManifestDetails()
	if err != nil {
		return []utils.ResultItem{}, err
	}
	searchableRepository, repositoryDetails, err := GetSearchableRepositoryAndDetails(remoteRepo, ddp.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	log.Debug(fmt.Sprintf("SearchableRepository: %s, Type: %s", remoteRepo, repositoryDetails.RepoType))

	// Use interface to get base path, then apply repo-type modifications
	handler := NewDockerManifestHandler(ddp.serviceManager).GetManifestHandler(dockerManifestType)
	basePath := handler.BuildSearchPaths(imageName, imageTag, manifestSha)
	layerPaths := ddp.applyRepoTypeModifications(basePath, *repositoryDetails)

	layers, err := SearchForImageLayersInPath(imageName, searchableRepository, layerPaths, ddp.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, err
	}

	if repositoryDetails.RepoType == repository.Remote {
		var markerLayers []string
		markerLayers, layers = getMarkerLayerShasFromSearchResult(layers)
		markerLayersDetails := handleMarkerLayersForDockerBuild(markerLayers, ddp.serviceManager, repositoryDetails.Key, imageName)
		layers = append(layers, markerLayersDetails...)
	}

	return layers, nil
}

// collectDetailsForDigestBasedImage handles images pulled by digest (@sha256:xxx)
func (ddp *DockerDependenciesBuilder) collectDetailsForDigestBasedImage(image *Image, imageName, digest string) ([]utils.ResultItem, error) {
	log.Debug(fmt.Sprintf("Collecting details for digest-based image: %s", image.Name()))

	remoteRepo, err := image.GetRemoteRepo(ddp.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	searchableRepository, repositoryDetails, err := GetSearchableRepositoryAndDetails(remoteRepo, ddp.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	log.Debug(fmt.Sprintf("SearchableRepository: %s, Type: %s", searchableRepository, repositoryDetails.RepoType))

	// Find manifest path by digest property using AQL
	manifestPath, err := SearchManifestPathByDigest(searchableRepository, digest, ddp.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	log.Debug(fmt.Sprintf("Found manifest at path: %s", manifestPath))

	layers, err := SearchForImageLayersInPath(imageName, searchableRepository, []string{manifestPath}, ddp.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, err
	}

	if repositoryDetails.RepoType == repository.Remote {
		var markerLayers []string
		markerLayers, layers = getMarkerLayerShasFromSearchResult(layers)
		markerLayersDetails := handleMarkerLayersForDockerBuild(markerLayers, ddp.serviceManager, repositoryDetails.Key, imageName)
		layers = append(layers, markerLayersDetails...)
	}

	return layers, nil
}

// applyRepoTypeModifications applies repository-type-specific path modifications
func (ddp *DockerDependenciesBuilder) applyRepoTypeModifications(basePath string, repositoryDetails DockerRepositoryDetails) []string {
	// for remote repositories, the image path is prefixed with "library/"
	if repositoryDetails.RepoType == repository.Remote {
		return []string{modifyPathForRemoteRepo(basePath)}
	}

	// virtual repository can contain remote repository and local repository
	// multi-platform images are stored in local under folders like sha256:xyz format
	// but in remote it's stored in folders like library/sha256__xyz format
	if repositoryDetails.RepoType == repository.Virtual {
		return append([]string{modifyPathForRemoteRepo(basePath)}, basePath)
	}

	return []string{basePath}
}

// createDependenciesFromResults converts search results to dependencies
func (ddp *DockerDependenciesBuilder) createDependenciesFromResults(results []utils.ResultItem) []buildinfo.Dependency {
	deduplicated := deduplicateResultsBySha256(results)
	dependencies := make([]buildinfo.Dependency, 0, len(deduplicated))
	for _, result := range deduplicated {
		dependencies = append(dependencies, result.ToDependency())
	}
	return dependencies
}
