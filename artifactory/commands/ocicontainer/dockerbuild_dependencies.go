package ocicontainer

import (
	"errors"
	"fmt"
	"sync"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
)

// getDependencies collects dependencies for all base images in parallel
func (dbib *DockerBuildInfoBuilder) getDependencies() ([]buildinfo.Dependency, error) {
	var wg sync.WaitGroup
	errChan := make(chan error, len(dbib.baseImages))
	dependencyResultChan := make(chan []utils.ResultItem, len(dbib.baseImages))

	for _, baseImage := range dbib.baseImages {
		wg.Add(1)
		go func(img BaseImage) {
			defer wg.Done()
			resultItems, err := dbib.collectDetailsForBaseImage(img)
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

	return dbib.createDependenciesFromResults(allDependencyResultItems), nil
}

// collectDetailsForBaseImage collects layer details for a single base image
func (dbib *DockerBuildInfoBuilder) collectDetailsForBaseImage(baseImage BaseImage) ([]utils.ResultItem, error) {
	remoteRepo, dockerManifestType, _, err := dbib.getRemoteRepoAndManifestTypeWithLeadSha(baseImage.Image)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	manifestSha, err := dbib.manifestDetails(baseImage)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	searchableRepository, err := dbib.getSearchableRepository(remoteRepo)
	if err != nil {
		return []utils.ResultItem{}, err
	}

	image := NewImage(baseImage.Image)
	imageTag, err := image.GetImageTag()
	if err != nil {
		return []utils.ResultItem{}, err
	}
	imageName, err := image.GetImageShortName()
	if err != nil {
		return []utils.ResultItem{}, err
	}

	// Use interface to get base path, then apply repo-type modifications
	handler := dbib.getManifestHandler(dockerManifestType)
	basePath := handler.BuildSearchPaths(imageName, imageTag, manifestSha)
	layerPaths := dbib.applyRepoTypeModifications(basePath)

	layers, err := dbib.searchForImageLayersInPath(imageName, searchableRepository, layerPaths)
	if err != nil {
		return []utils.ResultItem{}, err
	}

	if dbib.repositoryDetails.RepoType == "remote" {
		var markerLayers []string
		markerLayers, layers = getMarkerLayerShasFromSearchResult(layers)
		markerLayersDetails := handleMarkerLayersForDockerBuild(markerLayers, dbib.serviceManager, dbib.repositoryDetails.Key, imageName)
		layers = append(layers, markerLayersDetails...)
	}

	return layers, nil
}

// applyRepoTypeModifications applies repository-type-specific path modifications
func (dbib *DockerBuildInfoBuilder) applyRepoTypeModifications(basePath string) []string {
	// for remote repositories, the image path is prefixed with "library/"
	if dbib.repositoryDetails.RepoType == "remote" {
		return []string{modifyPathForRemoteRepo(basePath)}
	}

	// virtual repository can contain remote repository and local repository
	// multi-platform images are stored in local under folders like sha256:xyz format
	// but in remote it's stored in folders like library/sha256__xyz format
	if dbib.repositoryDetails.RepoType == "virtual" {
		return append([]string{modifyPathForRemoteRepo(basePath)}, basePath)
	}

	return []string{basePath}
}

// createDependenciesFromResults converts search results to dependencies
func (dbib *DockerBuildInfoBuilder) createDependenciesFromResults(results []utils.ResultItem) []buildinfo.Dependency {
	deduplicated := deduplicateResultsBySha256(results)
	dependencies := make([]buildinfo.Dependency, 0, len(deduplicated))
	for _, result := range deduplicated {
		dependencies = append(dependencies, result.ToDependency())
	}
	return dependencies
}
