package ocicontainer

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type DockerArtifactsBuilder struct {
	serviceManager    artifactory.ArtifactoryServicesManager
	isImagePushed     bool
	imageTag          string
	repositoryDetails *DockerRepositoryDetails
}

// NewDockerArtifactsBuilder creates a new builder for docker build command
func NewDockerArtifactsBuilder(serviceManager artifactory.ArtifactoryServicesManager, imageTag string, isImagePushed bool) *DockerArtifactsBuilder {
	return &DockerArtifactsBuilder{
		serviceManager: serviceManager,
		isImagePushed:  isImagePushed,
		imageTag:       imageTag,
	}
}

// getArtifacts collects artifacts for the pushed image
func (dab *DockerArtifactsBuilder) getArtifacts() (artifacts []buildinfo.Artifact, leadSha string, resultsToApplyProps []utils.ResultItem, err error) {
	var resultItems []utils.ResultItem
	if !dab.isImagePushed {
		log.Debug("Image was not pushed, skipping artifact collection!")
		return
	}
	if dab.isImagePushed {
		leadSha, resultItems, resultsToApplyProps, err = dab.collectDetailsForPushedImage(dab.imageTag)
		if err != nil {
			return artifacts, leadSha, resultsToApplyProps, err
		}
		artifacts = dab.createArtifactsFromResults(resultItems)
	}
	return artifacts, leadSha, resultsToApplyProps, err
}

// collectDetailsForPushedImage collects layer details for a pushed image
func (dab *DockerArtifactsBuilder) collectDetailsForPushedImage(imageRef string) (string, []utils.ResultItem, []utils.ResultItem, error) {
	log.Debug(fmt.Sprintf("Building artifacts for the pushed image %s", dab.imageTag))
	remoteRepo, dockerManifestType, leadSha, err := GetRemoteRepoAndManifestTypeWithLeadSha(imageRef, dab.serviceManager)
	if err != nil {
		return "", []utils.ResultItem{}, []utils.ResultItem{}, err
	}
	searchableRepository, repositoryDetails, err := GetSearchableRepositoryAndDetails(remoteRepo, dab.serviceManager)
	if err != nil {
		return "", []utils.ResultItem{}, []utils.ResultItem{}, err
	}
	dab.repositoryDetails = repositoryDetails
	layers, resultsToApplyProps, err := NewDockerManifestHandler(dab.serviceManager).FetchLayersOfPushedImage(imageRef, searchableRepository, dockerManifestType)
	if err != nil {
		return "", []utils.ResultItem{}, []utils.ResultItem{}, errorutils.CheckError(err)
	}
	log.Debug(fmt.Sprintf("Collected %d layers, %d folders for props", len(layers), len(resultsToApplyProps)))
	return leadSha, layers, resultsToApplyProps, err
}

// createArtifactsFromResults converts search results to artifacts
func (dab *DockerArtifactsBuilder) createArtifactsFromResults(results []utils.ResultItem) []buildinfo.Artifact {
	deduplicated := deduplicateResultsBySha256(results)
	artifacts := make([]buildinfo.Artifact, 0, len(deduplicated))
	for _, result := range deduplicated {
		artifacts = append(artifacts, result.ToArtifact())
	}
	return artifacts
}

// GetOriginalDeploymentRepo returns the repository where the image was pushed
func (dab *DockerArtifactsBuilder) GetOriginalDeploymentRepo() string {
	if dab.repositoryDetails == nil {
		return ""
	}
	if dab.repositoryDetails.RepoType == repository.Virtual {
		return dab.repositoryDetails.DefaultDeploymentRepo
	}
	return dab.repositoryDetails.Key
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
