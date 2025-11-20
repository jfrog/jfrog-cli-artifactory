package ocicontainer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// DockerBuildInfoBuilder is a simplified builder for docker build command
type DockerBuildInfoBuilder struct {
	buildName         string
	buildNumber       string
	project           string
	module            string
	serviceManager    artifactory.ArtifactoryServicesManager
	imageTag          string
	baseImages        []string
	isImagePushed     bool
	configDigest      string
	cmdArgs           []string
	repositoryDetails dockerRepositoryDetails
}

type dockerRepositoryDetails struct {
	Key                   string `json:"key"`
	RepoType              string `json:"rclass"`
	DefaultDeploymentRepo string `json:"defaultDeploymentRepo"`
}

// NewDockerBuildInfoBuilder creates a new builder for docker build command
func NewDockerBuildInfoBuilder(buildName, buildNumber, project string, module string, serviceManager artifactory.ArtifactoryServicesManager,
	imageTag string, baseImages []string, isImagePushed bool, cmdArgs []string) *DockerBuildInfoBuilder {

	biImage := NewImage(imageTag)

	var err error
	if module == "" {
		module, err = biImage.GetImageShortNameWithTag()
		if err != nil {
			log.Warn("Failed to extract module name from image tag '%s': %s. Using entire image tag as module name.", imageTag, err.Error())
			module = imageTag
		}
	}

	return &DockerBuildInfoBuilder{
		buildName:      buildName,
		buildNumber:    buildNumber,
		project:        project,
		module:         module,
		serviceManager: serviceManager,
		imageTag:       imageTag,
		baseImages:     baseImages,
		isImagePushed:  isImagePushed,
		cmdArgs:        cmdArgs,
	}
}

func (dbib *DockerBuildInfoBuilder) Build() error {
	if err := build.SaveBuildGeneralDetails(dbib.buildName, dbib.buildNumber, dbib.project); err != nil {
		return err
	}

	// Use a wait group to wait for all goroutines to complete
	var wg sync.WaitGroup
	errChan := make(chan error, len(dbib.baseImages))
	dependencyResultChan := make(chan []utils.ResultItem, len(dbib.baseImages))

	for _, baseImage := range dbib.baseImages {
		wg.Add(1)
		go func(img string) {
			defer wg.Done()
			resultItems, err := dbib.collectArtifactDetailsForImage(img)
			if err != nil {
				errChan <- err
				return
			}
			dependencyResultChan <- resultItems
		}(baseImage)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)
	close(dependencyResultChan)

	// Check for any errors
	var errorList []error
	for err := range errChan {
		errorList = append(errorList, err)
	}

	if len(errorList) > 0 {
		return fmt.Errorf("errors occurred during build info collection: %v", errors.Join(errorList...))
	}

	// Collect all results from all base images
	var allDependencyResultItems []utils.ResultItem
	for resultItems := range dependencyResultChan {
		allDependencyResultItems = append(allDependencyResultItems, resultItems...)
	}

	// Convert results to dependencies
	dependencies := dbib.createDependenciesFromResults(allDependencyResultItems)

	// need to collect artifacts if the image is pushed
	var artifacts []buildinfo.Artifact
	if dbib.isImagePushed {
		resultItems, err := dbib.collectArtifactDetailsForImage(dbib.imageTag)
		if err != nil {
			log.Warn("failed to collect build info for the pushed image: %s", err.Error())
		}
		artifacts = dbib.createArtifactsFromResults(resultItems)
		err = dbib.applyBuildProps(resultItems)
		if err != nil {
			log.Warn("failed to apply build info properties to pushed image layers: %s, Skipping....", err.Error())
		}
	}

	buildInfo := &buildinfo.BuildInfo{Modules: []buildinfo.Module{{
		Id:           dbib.module,
		Type:         buildinfo.Docker,
		Properties:   dbib.getBiProperties(),
		Dependencies: dependencies,
		Artifacts:    artifacts,
	}}}

	if err := build.SaveBuildInfo(dbib.buildName, dbib.buildNumber, dbib.project, buildInfo); err != nil {
		return errorutils.CheckErrorf("failed to save build info for '%s/%s': %s", dbib.buildName, dbib.buildNumber, err.Error())
	}

	return nil
}

func (dbib *DockerBuildInfoBuilder) collectArtifactDetailsForImage(imageRef string) ([]utils.ResultItem, error) {
	err := dbib.getImageRepositoryDetails(imageRef)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	searchableRepository, err := dbib.getSearchableRepository()
	if err != nil {
		return []utils.ResultItem{}, err
	}
	manifest, manifestSha, err := dbib.fetchManifest(imageRef)
	if err != nil {
		return []utils.ResultItem{}, err
	}

	layersSHA := cleanShaString(append(extractLayerSHAs(manifest), manifest.Config.Digest.String(), manifestSha))
	resultItems, err := dbib.searchArtifactoryForFilesBySha(layersSHA, searchableRepository)

	// this can happen in case of artifactory storing marker layers and foreign layers instead of actual layer blobs
	var markerLayerShas []string
	if (len(layersSHA) > len(resultItems)) && (dbib.repositoryDetails.RepoType == "remote") {
		markerLayerShas = getForeignLayerShaFromSearcResult(resultItems, layersSHA)
		markerLayerResultItems := handleMarkerLayersForDockerBuild(markerLayerShas, dbib.serviceManager, dbib.repositoryDetails.Key, imageRef)
		resultItems = append(resultItems, markerLayerResultItems...)
	}

	if err != nil {
		return []utils.ResultItem{}, err
	}
	dbib.configDigest = manifest.Config.Digest.String()
	return resultItems, nil
}

// fetchManifestAndExtractLayers fetches manifest from registry
func (dbib *DockerBuildInfoBuilder) fetchManifest(imageRef string) (imageManifest *v1.Manifest, manifestSha string, err error) {
	// Parse the image reference
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, "", err
	}

	image, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, "", err
	}

	imageManifest, err = image.Manifest()
	if err != nil {
		return nil, "", errorutils.CheckErrorf("failed to get manifest from image: %w", err)
	}

	manifestShaHash, err := image.Digest()
	if err != nil {
		log.Warn("failed to get manifest sha from image: %s", err.Error())
	}
	manifestSha = manifestShaHash.String()

	return imageManifest, manifestSha, nil
}

// extractLayerSHAs extracts layer SHAs from the manifest
func extractLayerSHAs(manifest *v1.Manifest) []string {
	var layerSHAs []string
	for _, layer := range manifest.Layers {
		layerSHAs = append(layerSHAs, layer.Digest.String())
	}
	return layerSHAs
}

// searchArtifactoryForFilesBySha searches for layers and config by SHA using AQL
func (dbib *DockerBuildInfoBuilder) searchArtifactoryForFilesBySha(shaCollection []string, repository string) ([]utils.ResultItem, error) {
	if len(shaCollection) == 0 {
		return nil, nil
	}

	var shaConditions []string
	for _, sha := range shaCollection {
		shaConditions = append(shaConditions, fmt.Sprintf(`{"sha256": {"$eq": "%s"}}`, sha))
	}

	// Build AQL query with $and and $or operators
	aqlQuery := fmt.Sprintf(`items.find({
  "$and": [
    { "repo": "%s" },
    {
      "$or": [
		%s
      ]
    }
  ]
})
.include("name", "repo", "path", "sha256", "actual_sha1", "actual_md5")`,
		repository, strings.Join(shaConditions, ",\n        "))

	log.Debug("Searching Artifactory with AQL:\n" + aqlQuery)

	// Execute AQL search
	reader, err := dbib.serviceManager.Aql(aqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to search Artifactory for layers: %w", err)
	}
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()

	// Parse all results from AqlSearchResult
	aqlResults, err := io.ReadAll(reader)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	parsedResult := new(utils.AqlSearchResult)
	if err = json.Unmarshal(aqlResults, parsedResult); err != nil {
		return nil, errorutils.CheckError(err)
	}

	var allResults []utils.ResultItem
	if parsedResult.Results != nil {
		allResults = parsedResult.Results
	}

	log.Debug(fmt.Sprintf("Found %d artifacts matching %d SHAs", len(allResults), len(shaCollection)))

	return allResults, nil
}

// createDependenciesFromResults converts search results to dependencies following the same pattern as buildinfo.go
// Includes deduplication similar to removeDuplicateLayers since we might find the same layer in multiple repos
func (dbib *DockerBuildInfoBuilder) createDependenciesFromResults(results []utils.ResultItem) []buildinfo.Dependency {
	var dependencies []buildinfo.Dependency

	// Use map to track duplicates (similar to removeDuplicateLayers in buildinfo.go)
	// Key by name (layer digest filename) since that's unique per layer
	// This prevents duplicate dependencies when the same layer exists in multiple repositories
	encountered := make(map[string]bool)

	for _, result := range results {
		// The Name field contains the layer digest filename (e.g., "sha256__abc123...")
		// which uniquely identifies each layer
		if !encountered[result.Name] {
			dependencies = append(dependencies, result.ToDependency())
			encountered[result.Name] = true
		}
	}

	return dependencies
}

func (dbib *DockerBuildInfoBuilder) createArtifactsFromResults(results []utils.ResultItem) []buildinfo.Artifact {
	var artifacts []buildinfo.Artifact

	// Use map to track duplicates (similar to removeDuplicateLayers in buildinfo.go)
	// Key by name (layer digest filename) since that's unique per layer
	// This prevents duplicate dependencies when the same layer exists in multiple repositories
	encountered := make(map[string]bool)

	for _, result := range results {
		// The Name field contains the layer digest filename (e.g., "sha256__abc123...")
		// which uniquely identifies each layer
		if !encountered[result.Name] {
			artifacts = append(artifacts, result.ToArtifact())
			encountered[result.Name] = true
		}
	}

	return artifacts
}

func (dbib *DockerBuildInfoBuilder) getImageRepositoryDetails(imageRef string) error {
	image := NewImage(imageRef)
	repository, err := image.GetRemoteRepo(dbib.serviceManager)
	if err != nil {
		return err
	}

	repositoryDetails := &dockerRepositoryDetails{}
	err = dbib.serviceManager.GetRepository(repository, &repositoryDetails)
	if err != nil {
		return err
	}
	dbib.repositoryDetails = *repositoryDetails
	return nil
}

func (dbib *DockerBuildInfoBuilder) getSearchableRepository() (string, error) {
	if dbib.repositoryDetails.RepoType == "" || dbib.repositoryDetails.Key == "" {
		return "", errorutils.CheckErrorf("repository details are incomplete: %+v", dbib.repositoryDetails)
	}
	if dbib.repositoryDetails.RepoType == "remote" {
		return dbib.repositoryDetails.Key + "-cache", nil
	}
	return dbib.repositoryDetails.Key, nil
}

func (dbib *DockerBuildInfoBuilder) getBiProperties() map[string]string {
	// prepare special properties for buildInfo
	properties := map[string]string{
		"docker.image.tag": dbib.imageTag,
	}
	if dbib.isImagePushed {
		properties["docker.image.id"] = dbib.configDigest
	}
	if dbib.cmdArgs != nil {
		properties["docker.build.command"] = strings.Join(dbib.cmdArgs, " ")
	}
	return properties
}

func (dbib *DockerBuildInfoBuilder) applyBuildProps(items []utils.ResultItem) (err error) {
	props, err := build.CreateBuildProperties(dbib.buildName, dbib.buildNumber, dbib.project)
	if err != nil {
		return
	}
	pushedRepo := dbib.getPushedRepo()
	filteredLayers := dbib.filterLayersFromVirtualRepo(items, pushedRepo)
	pathToFile, err := writeLayersToFile(filteredLayers)
	if err != nil {
		return
	}
	reader := content.NewContentReader(pathToFile, content.DefaultKey)
	defer ioutils.Close(reader, &err)
	_, err = dbib.serviceManager.SetProps(services.PropsParams{Reader: reader, Props: props})
	return err
}

// we need to keep in mind that pushing to remote repositories is not allowed
// also pushing to a virtual repository without a default deployment repo is not allowed
func (dbib *DockerBuildInfoBuilder) getPushedRepo() string {
	if dbib.repositoryDetails.RepoType == "virtual" {
		return dbib.repositoryDetails.DefaultDeploymentRepo
	}
	return dbib.repositoryDetails.Key
}

// it is necessary to filter out layers that are only available as part of the default deployment repo
// since layers can be shared from a repository without writable permissions
func (dbib *DockerBuildInfoBuilder) filterLayersFromVirtualRepo(items []utils.ResultItem, pushedRepo string) []utils.ResultItem {
	filteredLayers := make([]utils.ResultItem, 0, len(items))
	for _, item := range items {
		if item.Repo == pushedRepo {
			filteredLayers = append(filteredLayers, item)
		}
	}
	return filteredLayers
}

// cleanShaString removes the "sha256:" prefix from each string in the slice.
func cleanShaString(shas []string) []string {
	result := make([]string, len(shas))
	for i, sha := range shas {
		result[i] = strings.TrimPrefix(sha, "sha256:")
	}
	return result
}

func getForeignLayerShaFromSearcResult(searchResults []utils.ResultItem, layerShas []string) []string {
	// Create a set of SHA256 values from searchResults for efficient lookup
	searchResultShas := make(map[string]bool, len(searchResults))
	for _, result := range searchResults {
		if result.Sha256 != "" {
			searchResultShas[result.Sha256] = true
		}
	}

	var foreignLayerShas []string
	for _, layerSha := range layerShas {
		// Return SHA if it's in layerShas but NOT in searchResults
		if !searchResultShas[layerSha] {
			foreignLayerShas = append(foreignLayerShas, layerSha)
		}
	}
	return foreignLayerShas
}

// When a client tries to pull an image from a remote repository in Artifactory and the client has some the layers cached locally on the disk,
// then Artifactory will not download these layers into the remote repository cache. Instead, it will mark the layer artifacts with .marker suffix files in the remote cache.
// This function download all the marker layers into the remote cache repository concurrently using goroutines.
// Returns a slice of ResultItems populated with checksums and filenames from HTTP response headers.
func handleMarkerLayersForDockerBuild(markerLayerShas []string, serviceManager artifactory.ArtifactoryServicesManager, remoteRepo, imageRef string) []utils.ResultItem {
	if len(markerLayerShas) == 0 {
		return nil
	}
	image := NewImage(imageRef)
	imageName, err := image.GetImageShortName()
	if err != nil {
		log.Warn("Failed to extract image name from %s: %s", imageRef, err.Error())
		return nil
	}
	baseUrl := serviceManager.GetConfig().GetServiceDetails().GetUrl()

	// Download marker layers concurrently and collect results using channels
	var wg sync.WaitGroup
	resultChan := make(chan *utils.ResultItem, len(markerLayerShas))

	for _, layerSha := range markerLayerShas {
		wg.Add(1)
		go func(sha string) {
			defer wg.Done()
			resultItem := downloadSingleMarkerLayer(sha, remoteRepo, imageName, baseUrl, serviceManager)
			if resultItem != nil {
				resultChan <- resultItem
			}
		}(layerSha)
	}

	// Wait for all goroutines to complete, then close channel
	wg.Wait()
	close(resultChan)

	// Collect all results from channel
	resultItems := make([]utils.ResultItem, 0, len(markerLayerShas))
	for resultItem := range resultChan {
		resultItems = append(resultItems, *resultItem)
	}
	return resultItems
}

// downloadSingleMarkerLayer downloads a single marker layer into the remote cache repository.
// Returns a ResultItem populated with checksums and filename extracted from HTTP response headers.
func downloadSingleMarkerLayer(layerSha, remoteRepo, imageName, baseUrl string, serviceManager artifactory.ArtifactoryServicesManager) *utils.ResultItem {
	log.Debug(fmt.Sprintf("Downloading %s layer into remote repository cache...", layerSha))
	endpoint := "api/docker/" + remoteRepo + "/v2/" + imageName + "/blobs/" + "sha256:" + layerSha
	clientDetails := serviceManager.GetConfig().GetServiceDetails().CreateHttpClientDetails()

	resp, body, err := serviceManager.Client().SendHead(baseUrl+endpoint, &clientDetails)
	if err != nil {
		log.Warn("Skipping adding layer %s to build info. Failed to download layer in cache. Error: %s", layerSha, err.Error())
		return nil
	}
	if err = errorutils.CheckResponseStatusWithBody(resp, body, http.StatusOK); err != nil {
		log.Warn("Skipping adding layer %s to build info. Failed to download layer in cache. Error: %s, httpStatus: %d", layerSha, err.Error(), resp.StatusCode)
		return nil
	}

	// Extract checksums and filename from HTTP response headers and populate ResultItem
	// we cannot populate the Path field since we don't know the exact path of the layer in the artifactory
	resultItem := &utils.ResultItem{
		Actual_Sha1: resp.Header.Get("X-Checksum-Sha1"),
		Actual_Md5:  resp.Header.Get("X-Checksum-Md5"),
		Sha256:      resp.Header.Get("X-Checksum-Sha256"),
		Name:        resp.Header.Get("X-Artifactory-Filename"),
		Repo:        remoteRepo + "-cache",
	}

	log.Debug(fmt.Sprintf("Collected checksums for layer %s - SHA1: %s, SHA256: %s, MD5: %s, Filename: %s", layerSha, resultItem.Actual_Sha1, resultItem.Sha256, resultItem.Actual_Md5, resultItem.Name))
	return resultItem
}
