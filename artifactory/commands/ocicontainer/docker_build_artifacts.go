package ocicontainer

import (
	"fmt"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// getArtifacts collects artifacts for the pushed image
func (dbib *DockerBuildInfoBuilder) getArtifacts() (artifacts []buildinfo.Artifact, leadSha string, err error) {
	var resultItems []utils.ResultItem
	if dbib.isImagePushed {
		log.Debug(fmt.Sprintf("Building artifacts for the pushed image %s", dbib.imageTag))
		leadSha, resultItems, err = dbib.collectDetailsForPushedImage(dbib.imageTag)
		if err != nil {
			return artifacts, leadSha, err
		}
		artifacts = dbib.createArtifactsFromResults(resultItems)
	}
	return artifacts, leadSha, err
}

// collectDetailsForPushedImage collects layer details for a pushed image
func (dbib *DockerBuildInfoBuilder) collectDetailsForPushedImage(imageRef string) (string, []utils.ResultItem, error) {
	remoteRepo, dockerManifestType, leadSha, err := dbib.getRemoteRepoAndManifestTypeWithLeadSha(imageRef)
	if err != nil {
		return "", []utils.ResultItem{}, err
	}
	searchableRepository, err := dbib.getSearchableRepository(remoteRepo)
	if err != nil {
		return "", []utils.ResultItem{}, err
	}
	layers, err := dbib.fetchLayersOfPushedImage(imageRef, searchableRepository, dockerManifestType)
	return leadSha, layers, err
}

// createArtifactsFromResults converts search results to artifacts
func (dbib *DockerBuildInfoBuilder) createArtifactsFromResults(results []utils.ResultItem) []buildinfo.Artifact {
	deduplicated := deduplicateResultsBySha256(results)
	artifacts := make([]buildinfo.Artifact, 0, len(deduplicated))
	for _, result := range deduplicated {
		artifacts = append(artifacts, result.ToArtifact())
	}
	return artifacts
}

// getPushedRepo returns the repository where the image was pushed
func (dbib *DockerBuildInfoBuilder) getPushedRepo() string {
	if dbib.repositoryDetails.RepoType == "virtual" {
		return dbib.repositoryDetails.DefaultDeploymentRepo
	}
	return dbib.repositoryDetails.Key
}

// filterLayersFromVirtualRepo filters layers to only include those from the pushed repository
func filterLayersFromVirtualRepo(items []utils.ResultItem, pushedRepo string) []utils.ResultItem {
	filteredLayers := make([]utils.ResultItem, 0, len(items))
	for _, item := range items {
		if item.Repo == pushedRepo {
			filteredLayers = append(filteredLayers, item)
		}
	}
	return filteredLayers
}
