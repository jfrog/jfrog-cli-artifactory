package ocicontainer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	remoteRepoLibraryPrefix = "library"
	sha256Prefix            = "sha256:"
	sha256RemoteFormat      = "sha256__"
	uploadsFolder           = "_uploads"
	remoteCacheSuffix       = "-cache"
	remoteRepositoryType    = "remote"
)

// getRemoteRepoAndManifestTypeWithLeadSha determines the repository, manifest type, and lead SHA for an image
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
		return "", "", "", errorutils.CheckErrorf("unknown/other artifact type: %s", manifestFileName)
	}
}

// manifestDetails gets the manifest SHA for a base image, considering platform (OS/architecture)
func (dbib *DockerBuildInfoBuilder) manifestDetails(baseImage BaseImage) (string, error) {
	imageRef := baseImage.Image
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parsing reference %s: %w", imageRef, err)
	}
	var osName, osArch string

	if baseImage.OS != "" && baseImage.Architecture != "" {
		osName = baseImage.OS
		osArch = baseImage.Architecture
	} else {
		osName = runtime.GOOS
		if osName == "darwin" {
			osName = "linux"
		}
		osArch = runtime.GOARCH
	}

	remoteImage, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithPlatform(v1.Platform{OS: osName, Architecture: osArch}))
	if err != nil || remoteImage == nil {
		return "", fmt.Errorf("error fetching manifest for %s: %w", imageRef, err)
	}

	manifestShaDigest, err := remoteImage.Digest()
	if err != nil {
		return "", fmt.Errorf("error getting manifest digest for %s: %w", imageRef, err)
	}
	return manifestShaDigest.String(), nil
}

// getSearchableRepository resolves the repository name based on type (adds -cache for remote repos)
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
	if dbib.repositoryDetails.RepoType == remoteRepositoryType {
		return dbib.repositoryDetails.Key + "-cache", nil
	}
	return dbib.repositoryDetails.Key, nil
}

// searchArtifactoryForFilesByPath performs AQL query with exact path matching
func (dbib *DockerBuildInfoBuilder) searchArtifactoryForFilesByPath(repository string, paths []string) ([]utils.ResultItem, error) {
	if len(paths) == 0 {
		return []utils.ResultItem{}, nil
	}

	var pathConditions []string
	for _, item := range paths {
		pathConditions = append(pathConditions, fmt.Sprintf(`{"path": {"$eq": "%s"}}`, item))
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
		repository, strings.Join(pathConditions, ",\n        "))

	// Execute AQL search
	allResults, err := executeAqlQuery(dbib.serviceManager, aqlQuery)
	if err != nil {
		return []utils.ResultItem{}, fmt.Errorf("failed to search Artifactory for layers by path: %w", err)
	}
	log.Debug(fmt.Sprintf("Found %d artifacts matching %d paths", len(allResults), len(paths)))
	return allResults, nil
}

// searchForImageLayersInPath performs AQL query with $match/$nmatch patterns
// this function looks for the uploaded layers in docker-repo/imageName/path* provided and neglects the _uploads folder
// upload folder contains actual uploaded layer which are copied to their final location by docker
// adding properties in uploaded folder is redundant to form tree structure in build info page
func (dbib *DockerBuildInfoBuilder) searchForImageLayersInPath(imageName, repository string, paths []string) ([]utils.ResultItem, error) {
	excludePath := fmt.Sprintf("%s/%s", imageName, uploadsFolder)
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

// modifyPathForRemoteRepo adds library/ prefix and converts sha256: to sha256__
func modifyPathForRemoteRepo(path string) string {
	return fmt.Sprintf("%s/%s", remoteRepoLibraryPrefix, strings.Replace(path, sha256Prefix, sha256RemoteFormat, 1))
}

// deduplicateResultsBySha256 removes duplicate results based on SHA256
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

// executeAqlQuery executes an AQL query and parses the JSON response
func executeAqlQuery(serviceManager artifactory.ArtifactoryServicesManager, aqlQuery string) ([]utils.ResultItem, error) {
	reader, err := serviceManager.Aql(aqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to search Artifactory for layers: %w", err)
	}
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()

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

// MARKER LAYER HANDLING

// getMarkerLayerShasFromSearchResult separates marker layers from regular layers
func getMarkerLayerShasFromSearchResult(searchResults []utils.ResultItem) ([]string, []utils.ResultItem) {
	var markerLayerShas []string
	var filteredLayerShas []utils.ResultItem
	for _, result := range searchResults {
		if strings.HasSuffix(result.Name, markerLayerSuffix) {
			layerSha := strings.TrimSuffix(result.Name, markerLayerSuffix)
			markerLayerShas = append(markerLayerShas, layerSha)
			continue
		}
		filteredLayerShas = append(filteredLayerShas, result)
	}
	return markerLayerShas, filteredLayerShas
}

// handleMarkerLayersForDockerBuild downloads marker layers into the remote cache repository
func handleMarkerLayersForDockerBuild(markerLayerShas []string, serviceManager artifactory.ArtifactoryServicesManager, remoteRepo, imageShortName string) []utils.ResultItem {
	log.Debug("Handling marker layers for shas: ", strings.Join(markerLayerShas, ", "))
	if len(markerLayerShas) == 0 {
		return nil
	}
	baseUrl := serviceManager.GetConfig().GetServiceDetails().GetUrl()

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

	wg.Wait()
	close(resultChan)

	resultItems := make([]utils.ResultItem, 0, len(markerLayerShas))
	for resultItem := range resultChan {
		resultItems = append(resultItems, *resultItem)
	}
	return resultItems
}

// downloadSingleMarkerLayer downloads a single marker layer into the remote cache repository
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

	resultItem := &utils.ResultItem{
		Actual_Sha1: resp.Header.Get("X-Checksum-Sha1"),
		Actual_Md5:  resp.Header.Get("X-Checksum-Md5"),
		Sha256:      resp.Header.Get("X-Checksum-Sha256"),
		Name:        resp.Header.Get("X-Artifactory-Filename"),
		Repo:        remoteRepo + remoteCacheSuffix,
	}

	log.Debug(fmt.Sprintf("Collected checksums for layer %s - SHA1: %s, SHA256: %s, MD5: %s, Filename: %s", layerSha, resultItem.Actual_Sha1, resultItem.Sha256, resultItem.Actual_Md5, resultItem.Name))
	return resultItem
}
