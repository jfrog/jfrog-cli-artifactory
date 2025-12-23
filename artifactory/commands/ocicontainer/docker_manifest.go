package ocicontainer

import (
	"fmt"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"runtime"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	osDarwin = "darwin"
	osLinux  = "linux"
)

type DockerImage struct {
	Image        string
	OS           string
	Architecture string
}

// GetManifestDetails gets the manifest SHA for a base image, considering platform (OS/architecture)
func (baseImage DockerImage) GetManifestDetails() (string, error) {
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
		if osName == osDarwin {
			osName = osLinux
		}
		osArch = runtime.GOARCH
	}

	remoteImage, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithPlatform(v1.Platform{OS: osName, Architecture: osArch}))
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	if remoteImage == nil {
		return "", fmt.Errorf("error fetching manifest for %s", imageRef)
	}

	manifestShaDigest, err := remoteImage.Digest()
	if err != nil {
		return "", fmt.Errorf("error getting manifest digest for %s: %w", imageRef, err)
	}
	return manifestShaDigest.String(), nil
}

func GetManifestTypeAndLeadSha(imageRef string) (ManifestType, string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", "", errorutils.CheckErrorf("parsing reference %s: %w", imageRef, err)
	}
	desc, err := remote.Head(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return "", "", errorutils.CheckErrorf("getting remote manifest for %s: %w", imageRef, err)
	}
	log.Debug(fmt.Sprintf("Manifest type: %s, digest: %s", desc.MediaType, desc.Digest.Hex))

	if desc.MediaType.IsIndex() {
		return ManifestList, desc.Digest.Hex, nil
	} else if desc.MediaType.IsImage() {
		return Manifest, desc.Digest.Hex, nil
	}
	return "", "", errorutils.CheckErrorf("unsupported manifest media type: %s", desc.MediaType)
}

// ManifestHandler interface for handling different manifest types
type ManifestHandler interface {
	// FetchLayers fetches layers for a pushed image from Artifactory, also return folders, whose elements are eligible to apply props on
	FetchLayers(imageRef, repository string) ([]utils.ResultItem, []utils.ResultItem, error)
	// BuildSearchPaths builds the search path for base image layers
	BuildSearchPaths(imageName, imageTag, manifestDigest string) string
}

type DockerManifestHandler struct {
	serviceManager artifactory.ArtifactoryServicesManager
}

// SingleManifestHandler handles single manifest images
type SingleManifestHandler struct {
	*DockerManifestHandler
}

// FatManifestHandler handles fat manifest (multi-platform) images
type FatManifestHandler struct {
	*DockerManifestHandler
}

func NewDockerManifestHandler(serviceManager artifactory.ArtifactoryServicesManager) *DockerManifestHandler {
	return &DockerManifestHandler{serviceManager: serviceManager}
}

// GetManifestHandler returns the appropriate handler based on manifest type
func (dmh *DockerManifestHandler) GetManifestHandler(dockerManifestType ManifestType) ManifestHandler {
	switch dockerManifestType {
	case ManifestList:
		return &FatManifestHandler{dmh}
	case Manifest:
		return &SingleManifestHandler{dmh}
	default:
		return nil
	}
}

// FetchLayersOfPushedImage dispatches to the appropriate manifest handler
func (dmh *DockerManifestHandler) FetchLayersOfPushedImage(imageRef, repository string, dockerManifestType ManifestType) ([]utils.ResultItem, []utils.ResultItem, error) {
	log.Debug(fmt.Sprintf("Fetching layers for the pushed image %s", imageRef))
	handler := dmh.GetManifestHandler(dockerManifestType)
	if handler == nil {
		return []utils.ResultItem{}, []utils.ResultItem{},
			errorutils.CheckErrorf("unknown/other manifest type provided: %s", dockerManifestType)
	}
	return handler.FetchLayers(imageRef, repository)
}

// SINGLE MANIFEST HANDLER IMPLEMENTATION

// FetchLayers fetches layers for a single manifest image
func (h *SingleManifestHandler) FetchLayers(imageRef string, repository string) ([]utils.ResultItem, []utils.ResultItem, error) {
	log.Debug(fmt.Sprintf("Fetching layers for single manifest image: %s", imageRef))
	var folderToApplyProps []utils.ResultItem
	image := NewImage(imageRef)
	imageTag, err := image.GetImageTag()
	if err != nil {
		return []utils.ResultItem{}, []utils.ResultItem{}, err
	}
	imageName, err := image.GetImageShortName()
	if err != nil {
		return []utils.ResultItem{}, []utils.ResultItem{}, err
	}
	expectedImagePath := imageName + "/" + imageTag
	folderToApplyProps = append(folderToApplyProps, utils.ResultItem{
		Repo: repository,
		Path: expectedImagePath,
		Type: "folder",
	})
	layers, err := searchArtifactoryForFilesByPath(repository, []string{expectedImagePath}, h.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, folderToApplyProps, err
	}
	log.Debug(fmt.Sprintf("Found %d layers at path %s", len(layers), expectedImagePath))
	return layers, folderToApplyProps, nil
}

// BuildSearchPaths returns the search path for a single manifest image (imageName/imageTag)
func (h *SingleManifestHandler) BuildSearchPaths(imageName, imageTag, manifestDigest string) string {
	return fmt.Sprintf("%s/%s", imageName, imageTag)
}

// FAT MANIFEST HANDLER IMPLEMENTATION

// FetchLayers fetches layers for a fat manifest (multi-platform) image
func (h *FatManifestHandler) FetchLayers(imageRef string, repository string) ([]utils.ResultItem, []utils.ResultItem, error) {
	log.Debug(fmt.Sprintf("Fetching layers for fat manifest image: %s", imageRef))
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return []utils.ResultItem{}, []utils.ResultItem{}, fmt.Errorf("parsing reference %s: %w", imageRef, err)
	}
	manifestShas, err := h.getManifestShaListForImage(ref)
	if err != nil {
		return []utils.ResultItem{}, []utils.ResultItem{}, err
	}
	return h.getLayersForManifestSha(imageRef, manifestShas, repository)
}

// BuildSearchPaths returns the search path for a fat manifest image (imageName/sha256:xxx)
func (h *FatManifestHandler) BuildSearchPaths(imageName, imageTag, manifestDigest string) string {
	return fmt.Sprintf("%s/%s", imageName, manifestDigest)
}

// getManifestShaListForImage retrieves all platform manifest SHAs from a fat manifest
func (h *FatManifestHandler) getManifestShaListForImage(imageReference name.Reference) ([]string, error) {
	index, err := remote.Index(imageReference, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return []string{}, errorutils.CheckErrorf("Failed to get image index for image: %s. Error: %s", imageReference.Name(), err.Error())
	}
	manifestList, err := index.IndexManifest()
	if err != nil {
		return []string{}, errorutils.CheckErrorf("Failed to get manifest list for image: %s. Error: %s", imageReference.Name(), err.Error())
	}
	manifestShas := make([]string, 0, len(manifestList.Manifests))
	for _, descriptor := range manifestList.Manifests {
		manifestShas = append(manifestShas, descriptor.Digest.String())
	}
	log.Debug(fmt.Sprintf("Found %d platform manifests", len(manifestShas)))
	return manifestShas, nil
}

// getLayersForManifestSha searches for layers across all manifest SHAs
func (h *FatManifestHandler) getLayersForManifestSha(imageRef string, manifestShas []string, repository string) ([]utils.ResultItem, []utils.ResultItem, error) {
	var foldersToApplyProps []utils.ResultItem
	searchablePathForManifest := h.createSearchablePathForDockerManifestContents(imageRef, manifestShas)

	for _, path := range searchablePathForManifest {
		foldersToApplyProps = append(foldersToApplyProps, utils.ResultItem{
			Repo: repository,
			Path: path,
			Type: "folder",
		})
	}

	layers, err := searchArtifactoryForFilesByPath(repository, searchablePathForManifest, h.serviceManager)
	if err != nil {
		return []utils.ResultItem{}, foldersToApplyProps, err
	}
	return layers, foldersToApplyProps, nil
}

// createSearchablePathForDockerManifestContents builds search paths like imageName/sha256:xxx
func (h *FatManifestHandler) createSearchablePathForDockerManifestContents(imageRef string, manifestShas []string) []string {
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
