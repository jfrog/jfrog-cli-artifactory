package ocicontainer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
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
	buildName                       string
	buildNumber                     string
	project                         string
	module                          string
	serviceManager                  artifactory.ArtifactoryServicesManager
	imageTag                        string
	baseImages                      []BaseImage
	isImagePushed                   bool
	cmdArgs                         []string
	repositoryDetails               dockerRepositoryDetails
	searchableLayerForApplyingProps []utils.ResultItem
}

type dockerRepositoryDetails struct {
	Key                   string `json:"key"`
	RepoType              string `json:"rclass"`
	DefaultDeploymentRepo string `json:"defaultDeploymentRepo"`
}

type BaseImage struct {
	Image        string
	OS           string
	Architecture string
}

type manifestType string

const (
	ManifestList manifestType = "list.manifest.json"
	Manifest     manifestType = "manifest.json"
)

// NewDockerBuildInfoBuilder creates a new builder for docker build command
func NewDockerBuildInfoBuilder(buildName, buildNumber, project string, module string, serviceManager artifactory.ArtifactoryServicesManager,
	imageTag string, baseImages []BaseImage, isImagePushed bool, cmdArgs []string) *DockerBuildInfoBuilder {

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

	dependencies, err := dbib.getDependencies()
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to get dependencies for '%s'. Error: %v", dbib.buildName, err))
	}

	artifacts, leadSha, err := dbib.getArtifacts()
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to get artifacts for '%s'. Error: %v", dbib.buildName, err))
	}

	err = dbib.applyBuildProps(dbib.searchableLayerForApplyingProps)
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to apply build prop. Error: %v", err))
	}

	biProperties := dbib.getBiProperties(leadSha)

	buildInfo := &buildinfo.BuildInfo{Modules: []buildinfo.Module{{
		Id:           dbib.module,
		Type:         buildinfo.Docker,
		Properties:   biProperties,
		Dependencies: dependencies,
		Artifacts:    artifacts,
	}}}

	if err = build.SaveBuildInfo(dbib.buildName, dbib.buildNumber, dbib.project, buildInfo); err != nil {
		return errorutils.CheckErrorf("failed to save build info for '%s/%s': %s", dbib.buildName, dbib.buildNumber, err.Error())
	}

	return nil
}

func (dbib *DockerBuildInfoBuilder) getDependencies() ([]buildinfo.Dependency, error) {
	// Use a wait group to wait for all goroutines to complete
	var wg sync.WaitGroup
	errChan := make(chan error, len(dbib.baseImages))
	dependencyResultChan := make(chan []utils.ResultItem, len(dbib.baseImages))

	for _, baseImage := range dbib.baseImages {
		wg.Add(1)
		go func(img BaseImage) {
			defer wg.Done()
			resultItems, err := dbib.CollectDetailsForBaseImage(img)
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
		return []buildinfo.Dependency{}, fmt.Errorf("errors occurred during build info collection: %v", errors.Join(errorList...))
	}

	// Collect all results from all base images
	var allDependencyResultItems []utils.ResultItem
	for resultItems := range dependencyResultChan {
		allDependencyResultItems = append(allDependencyResultItems, resultItems...)
	}

	// Convert results to dependencies
	return dbib.createDependenciesFromResults(allDependencyResultItems), nil
}

func (dbib *DockerBuildInfoBuilder) getArtifacts() (artifacts []buildinfo.Artifact, leadSha string, err error) {
	// need to collect artifacts if the image is pushed
	if dbib.isImagePushed {
		log.Debug(fmt.Sprintf("Building artifacts for the pushed image %s", dbib.imageTag))
		leadSha, resultItems, err := dbib.CollectDetailsForPushedImage(dbib.imageTag)
		if err != nil {
			return artifacts, leadSha, err
		}
		artifacts = dbib.createArtifactsFromResults(resultItems)
	}
	return artifacts, leadSha, nil
}

// PUSHED IMAGE LAYERS COLLECTION

// DECIDING LAYERS TO COLLECT BASED ON MANIFEST TYPE

func (dbib *DockerBuildInfoBuilder) getRemoteRepoAndManifestTypeWithLeadSha(imageRef string) (string, manifestType, string, error) {
	image := NewImage(imageRef)
	repository, manifestFileName, leadSha, err := image.GetRemoteRepoAndManifestTypeAndLeadSha(dbib.serviceManager)
	if err != nil {
		return "", "", "", err
	}
	switch manifestFileName {
	case string(ManifestList):
		return repository, ManifestList, leadSha, nil
	case string(Manifest):
		return repository, Manifest, leadSha, nil
	default:
		return "", "", "", errorutils.CheckErrorf(fmt.Sprintf("Unknown/Other Artifact Type: %s", manifestFileName))
	}
}

//func (dbib *DockerBuildInfoBuilder) manifestType(baseImage BaseImage) (manifestType, error) {
//	imageRef := baseImage.Image
//	ref, err := name.ParseReference(imageRef)
//	if err != nil {
//		return "", fmt.Errorf("parsing reference %s: %w", imageRef, err)
//	}
//
//	// Use remote.Head to get the descriptor without downloading the full image
//	desc, err := remote.Head(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
//	if err != nil {
//		return "", fmt.Errorf("fetching descriptor for %s: %w", imageRef, err)
//	}
//
//	// Check the MediaType
//	switch desc.MediaType {
//	case types.DockerManifestList, types.OCIImageIndex:
//		return ManifestList, nil
//	case types.DockerManifestSchema2, types.OCIManifestSchema1:
//		return Manifest, nil
//	default:
//		return "", errorutils.CheckErrorf(fmt.Sprintf("Unknown/Other Artifact Type: %s", desc.MediaType))
//	}
//}

func (dbib *DockerBuildInfoBuilder) manifestDetails(baseImage BaseImage) (string, error) {
	imageRef := baseImage.Image
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parsing reference %s: %w", imageRef, err)
	}
	var goos, goarch string

	if baseImage.OS != "" && baseImage.Architecture != "" {
		goos = baseImage.OS
		goarch = baseImage.Architecture
	} else {
		goos = runtime.GOOS
		if goos == "darwin" {
			goos = "linux"
		}
		goarch = runtime.GOARCH
	}

	remoteImage, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithPlatform(v1.Platform{OS: goos, Architecture: goarch}))
	if err != nil || remoteImage == nil {
		return "", fmt.Errorf("error fetching manifest for %s: %w", imageRef, err)
	}

	manifestShaDigest, err := remoteImage.Digest()
	if err != nil {
		return "", fmt.Errorf("error getting manifest digest for %s: %w", imageRef, err)
	}
	return manifestShaDigest.String(), nil
}

func (dbib *DockerBuildInfoBuilder) fetchLayersOfPushedImage(imageRef, repository string, manifestType manifestType) ([]utils.ResultItem, error) {
	switch manifestType {
	case ManifestList:
		return dbib.getLayersForFatManifestImage(imageRef, repository)
	case Manifest:
		return dbib.getLayersForSingleManifestImage(imageRef, repository)
	default:
		return []utils.ResultItem{}, errorutils.CheckErrorf(fmt.Sprintf("Unknown/Other manifest type provided: %s", manifestType))
	}
}

// BELOW FUNCTIONS ARE FOR SINGLE MANIFEST IMAGES
func (dbib *DockerBuildInfoBuilder) getLayersForSingleManifestImage(imageRef string, repository string) ([]utils.ResultItem, error) {
	image := NewImage(imageRef)
	imageTag, err := image.GetImageTag()
	if err != nil {
		return []utils.ResultItem{}, err
	}
	imageName, err := image.GetImageShortName()
	if err != nil {
		return []utils.ResultItem{}, err
	}
	expectedImagePath := imageName + "/" + imageTag
	dbib.searchableLayerForApplyingProps = append(dbib.searchableLayerForApplyingProps, utils.ResultItem{
		Repo: repository,
		Path: expectedImagePath,
		Type: "folder",
	})
	layers, err := dbib.searchArtifactoryForFilesByPath(repository, []string{expectedImagePath})
	if err != nil {
		return []utils.ResultItem{}, err
	}
	return layers, nil
}

// BELOW FUNCTIONS ARE FOR FAT MANIFEST IMAGES
func (dbib *DockerBuildInfoBuilder) getLayersForFatManifestImage(imageRef string, repository string) ([]utils.ResultItem, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return []utils.ResultItem{}, fmt.Errorf("parsing reference %s: %w", imageRef, err)
	}
	manifestShas := dbib.getManifestShaListForImage(ref)
	return dbib.getLayersForManifestSha(imageRef, manifestShas, repository)
}

func (dbib *DockerBuildInfoBuilder) getManifestShaListForImage(imageReference name.Reference) []string {
	index, err := remote.Index(imageReference, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to get image index for image: %s. Error: %s", imageReference.Name(), err.Error()))
		return []string{}
	}
	manifestList, err := index.IndexManifest()
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to get manifest list for image: %s. Error: %s", imageReference.Name(), err.Error()))
		return []string{}
	}
	manifestShas := make([]string, 0, len(manifestList.Manifests))
	for _, descriptor := range manifestList.Manifests {
		manifestShas = append(manifestShas, descriptor.Digest.String())
	}
	return manifestShas
}

func (dbib *DockerBuildInfoBuilder) getLayersForManifestSha(imageRef string, manifestShas []string, repository string) ([]utils.ResultItem, error) {
	searchablePathForManifest := dbib.createSearchablePathForDockerManifestContents(imageRef, manifestShas)

	for _, path := range searchablePathForManifest {
		dbib.searchableLayerForApplyingProps = append(dbib.searchableLayerForApplyingProps, utils.ResultItem{
			Repo: repository,
			Path: path,
			Type: "folder",
		})
	}

	layers, err := dbib.searchArtifactoryForFilesByPath(repository, searchablePathForManifest)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	return layers, nil
}

// construct path like imageName/sha256:01571af2b1dc48c810160767ae96e01c1491bd3b628613cfc6caf8c8d5738b24
func (dbib *DockerBuildInfoBuilder) createSearchablePathForDockerManifestContents(imageRef string, manifestShas []string) (searchablePath []string) {
	imageName, err := NewImage(imageRef).GetImageShortName()
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to get image name: %s. Error: %s while creating searchable paths for docker manifest contents.", imageRef, err.Error()))
		return []string{}
	}
	searchablePaths := make([]string, 0, len(manifestShas))
	for _, manifestSha := range manifestShas {
		searchablePaths = append(searchablePaths, fmt.Sprintf("%s/%s", imageName, manifestSha))
	}
	return searchablePaths
}

// COMMON FUNCTIONS USED BY BOTH SINGLE AND FAT MANIFEST IMAGES
func (dbib *DockerBuildInfoBuilder) searchArtifactoryForFilesByPath(repository string, paths []string) ([]utils.ResultItem, error) {
	if len(paths) == 0 {
		return []utils.ResultItem{}, nil
	}

	var searchPathString []string
	for _, item := range paths {
		searchPathString = append(searchPathString, fmt.Sprintf(`{"path": {"$eq": "%s"}}`, item))
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
		repository, strings.Join(searchPathString, ",\n        "))

	// Execute AQL search
	allResults, err := executeAqlQuery(dbib.serviceManager, aqlQuery)
	if err != nil {
		return []utils.ResultItem{}, fmt.Errorf("failed to search Artifactory for layers by path: %w", err)
	}
	log.Debug(fmt.Sprintf("Found %d artifacts matching %d paths", len(allResults), len(paths)))
	return allResults, nil
}

// USED IN BASE IMAGE LAYERS COLLECTION
func (dbib *DockerBuildInfoBuilder) constructLayersSearchPath(imageName, imageTag, manifestDigest string, manifestType manifestType) []string {
	var basePath string
	switch manifestType {
	case ManifestList:
		basePath = constructPathForManifestList(imageName, manifestDigest)
	case Manifest:
		basePath = constructPathForManifest(imageName, imageTag)
	}

	// for remote repositories, the image path is prefixed with "library/"
	if dbib.repositoryDetails.RepoType == "remote" {
		return []string{modifyPathForRemoteRepo(basePath)}
	}

	// virtual repository can contain remote repository and local repository
	// also multi-platform images is stored in local under folders like sha256:xyz format
	// but in remote it's stored in folders like library/sha256__xyz format
	if dbib.repositoryDetails.RepoType == "virtual" {
		// Prepend remote format path to the beginning of the slice
		return append([]string{modifyPathForRemoteRepo(basePath)}, basePath)
	}

	return []string{basePath}
}

func constructPathForManifestList(imageName string, manifestSha string) string {
	return fmt.Sprintf("%s/%s", imageName, manifestSha)
}

func constructPathForManifest(imageName string, imageTag string) string {
	return fmt.Sprintf("%s/%s", imageName, imageTag)
}

func modifyPathForRemoteRepo(path string) string {
	return fmt.Sprintf("library/%s", strings.Replace(path, "sha256:", "sha256__", 1))
}

func (dbib *DockerBuildInfoBuilder) searchForImageLayersInPath(imageName, repository string, paths []string) ([]utils.ResultItem, error) {
	excludePath := fmt.Sprintf("%s/%s", imageName, "_uploads")
	var allResults []utils.ResultItem
	var err error

	for _, path := range paths {

		// Build AQL query with $and, $match, and $nmatch operators
		aqlQuery := fmt.Sprintf(`items.find({
  "$and": [
    { "repo": "%s" },
    {
      "path": {
        "$match": "%s"
      }
    },
    {
      "path": {
        "$nmatch": "%s"
      }
    }
  ]
})
.include("name", "repo", "path", "sha256", "actual_sha1", "actual_md5")`,
			repository, path, excludePath)

		// Execute AQL search
		allResults, err = executeAqlQuery(dbib.serviceManager, aqlQuery)
		if err != nil {
			return []utils.ResultItem{}, fmt.Errorf("failed to search Artifactory for layers in path: %w", err)
		}
		log.Debug(fmt.Sprintf("Found %d artifacts matching path pattern %s", len(allResults), path))
		if len(allResults) > 0 {
			return allResults, nil
		}
	}
	return allResults, nil
}

func (dbib *DockerBuildInfoBuilder) CollectDetailsForBaseImage(baseImage BaseImage) ([]utils.ResultItem, error) {
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

	layerPath := dbib.constructLayersSearchPath(imageName, imageTag, manifestSha, dockerManifestType)
	layers, err := dbib.searchForImageLayersInPath(imageName, searchableRepository, layerPath)
	if err != nil {
		return []utils.ResultItem{}, err
	}

	if dbib.repositoryDetails.RepoType == "remote" {
		// Handle marker layers for remote repositories
		var markerLayers []string
		markerLayers, layers = getMarkerLayerShasFromSearchResult(layers)
		markerLayersDetails := handleMarkerLayersForDockerBuild(markerLayers, dbib.serviceManager, dbib.repositoryDetails.Key, imageName)
		layers = append(layers, markerLayersDetails...)
	}

	return layers, nil
}

func (dbib *DockerBuildInfoBuilder) CollectDetailsForPushedImage(imageRef string) (string, []utils.ResultItem, error) {
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

// Extract deduplication logic
func deduplicateResultsBySha256(results []utils.ResultItem) []utils.ResultItem {
    encountered := make(map[string]bool)
    deduplicated := make([]utils.ResultItem, 0, len(results))
    for _, result := range results {
        if !encountered[result.Sha256] {
            deduplicated = append(deduplicated, result)
            encountered[result.Sha256] = true
        }
    }
    return deduplicated
}

// converts search results to dependencies following the same pattern as buildinfo.go
func (dbib *DockerBuildInfoBuilder) createDependenciesFromResults(results []utils.ResultItem) []buildinfo.Dependency {
    deduplicated := deduplicateResultsBySha256(results)
    dependencies := make([]buildinfo.Dependency, 0, len(deduplicated))
    for _, result := range deduplicated {
        dependencies = append(dependencies, result.ToDependency())
    }
    return dependencies
}

func (dbib *DockerBuildInfoBuilder) createArtifactsFromResults(results []utils.ResultItem) []buildinfo.Artifact {
    deduplicated := deduplicateResultsBySha256(results)
    artifacts := make([]buildinfo.Artifact, 0, len(deduplicated))
    for _, result := range deduplicated {
        artifacts = append(artifacts, result.ToArtifact())
    }
    return artifacts
}

func (dbib *DockerBuildInfoBuilder) getSearchableRepository(repositoryName string) (string, error) {
	repositoryDetails := &dockerRepositoryDetails{}
	err := dbib.serviceManager.GetRepository(repositoryName, &repositoryDetails)
	if err != nil {
		return "", err
	}
	dbib.repositoryDetails = *repositoryDetails
	if dbib.repositoryDetails.RepoType == "" || dbib.repositoryDetails.Key == "" {
		return "", errorutils.CheckErrorf("repository details are incomplete: %+v", dbib.repositoryDetails)
	}
	if dbib.repositoryDetails.RepoType == "remote" {
		return dbib.repositoryDetails.Key + "-cache", nil
	}
	return dbib.repositoryDetails.Key, nil
}

func (dbib *DockerBuildInfoBuilder) getBiProperties(leadSha string) map[string]string {
	// prepare special properties for buildInfo
	properties := map[string]string{
		"docker.image.tag": dbib.imageTag,
	}
	if dbib.isImagePushed {
		properties["docker.image.id"] = leadSha
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
	filteredLayers := filterLayersFromVirtualRepo(items, pushedRepo)
	pathToFile, err := writeLayersToFile(filteredLayers)
	if err != nil {
		return
	}
	reader := content.NewContentReader(pathToFile, content.DefaultKey)
	defer ioutils.Close(reader, &err)
	_, err = dbib.serviceManager.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true, Recursive: true})
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
func filterLayersFromVirtualRepo(items []utils.ResultItem, pushedRepo string) []utils.ResultItem {
	filteredLayers := make([]utils.ResultItem, 0, len(items))
	for _, item := range items {
		if item.Repo == pushedRepo {
			filteredLayers = append(filteredLayers, item)
		}
	}
	return filteredLayers
}

func getMarkerLayerShasFromSearchResult(searchResults []utils.ResultItem) ([]string, []utils.ResultItem) {
	var markerLayerShas []string
	var filteredLayerShas []utils.ResultItem
	for _, result := range searchResults {
		if strings.HasSuffix(result.Name, ".marker") {
			// Remove the .marker suffix to get the actual layer SHA
			layerSha := strings.TrimSuffix(result.Name, ".marker")
			markerLayerShas = append(markerLayerShas, layerSha)
			continue
		}
		filteredLayerShas = append(filteredLayerShas, result)
	}
	return markerLayerShas, filteredLayerShas
}

// When a client tries to pull an image from a remote repository in Artifactory and the client has some the layers cached locally on the disk,
// then Artifactory will not download these layers into the remote repository cache. Instead, it will mark the layer artifacts with .marker suffix files in the remote cache.
// This function download all the marker layers into the remote cache repository concurrently using goroutines.
// Returns a slice of ResultItems populated with checksums and filenames from HTTP response headers.
func handleMarkerLayersForDockerBuild(markerLayerShas []string, serviceManager artifactory.ArtifactoryServicesManager, remoteRepo, imageShortName string) []utils.ResultItem {
	log.Debug("Handling marker layers for shas: ", strings.Join(markerLayerShas, ", "))
	if len(markerLayerShas) == 0 {
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
			resultItem := downloadSingleMarkerLayer(sha, remoteRepo, imageShortName, baseUrl, serviceManager)
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
	log.Debug(fmt.Sprintf("Downloading marker %s layer into remote repository cache...", layerSha))
	endpoint := "api/docker/" + remoteRepo + "/v2/" + imageName + "/blobs/" + "sha256:" + layerSha
	clientDetails := serviceManager.GetConfig().GetServiceDetails().CreateHttpClientDetails()

	resp, body, err := serviceManager.Client().SendHead(baseUrl+endpoint, &clientDetails)
	if err != nil {
		log.Warn(fmt.Sprintf("Skipping adding layer %s to build info. Failed to download layer in cache. Error: %s", layerSha, err.Error()))
		return nil
	}
	if err = errorutils.CheckResponseStatusWithBody(resp, body, http.StatusOK); err != nil {
		log.Warn(fmt.Sprintf("Skipping adding layer %s to build info. Failed to download layer in cache. Error: %s, httpStatus: %d", layerSha, err.Error(), resp.StatusCode))
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

func executeAqlQuery(serviceManager artifactory.ArtifactoryServicesManager, aqlQuery string) ([]utils.ResultItem, error) {
	// Execute AQL search
	reader, err := serviceManager.Aql(aqlQuery)
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

	return allResults, nil
}
