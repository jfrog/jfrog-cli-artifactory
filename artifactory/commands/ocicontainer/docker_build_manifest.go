package ocicontainer

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ManifestHandler interface for handling different manifest types
type ManifestHandler interface {
	// FetchLayers fetches layers for a pushed image from Artifactory
	FetchLayers(imageRef, repository string) ([]utils.ResultItem, error)
	// BuildSearchPaths builds the search path for base image layers
	BuildSearchPaths(imageName, imageTag, manifestDigest string) string
}

// SingleManifestHandler handles single manifest images
type SingleManifestHandler struct {
	builder *DockerBuildInfoBuilder
}

// FatManifestHandler handles fat manifest (multi-platform) images
type FatManifestHandler struct {
	builder *DockerBuildInfoBuilder
}

// getManifestHandler returns the appropriate handler based on manifest type
func (dbib *DockerBuildInfoBuilder) getManifestHandler(dockerManifestType manifestType) ManifestHandler {
	switch dockerManifestType {
	case ManifestList:
		return &FatManifestHandler{builder: dbib}
	case Manifest:
		return &SingleManifestHandler{builder: dbib}
	default:
		return nil
	}
}

// fetchLayersOfPushedImage dispatches to the appropriate manifest handler
func (dbib *DockerBuildInfoBuilder) fetchLayersOfPushedImage(imageRef, repository string, dockerManifestType manifestType) ([]utils.ResultItem, error) {
	handler := dbib.getManifestHandler(dockerManifestType)
	if handler == nil {
		return []utils.ResultItem{}, errorutils.CheckErrorf("unknown/other manifest type provided: %s", dockerManifestType)
	}
	return handler.FetchLayers(imageRef, repository)
}

// SINGLE MANIFEST HANDLER IMPLEMENTATION

// FetchLayers fetches layers for a single manifest image
func (h *SingleManifestHandler) FetchLayers(imageRef string, repository string) ([]utils.ResultItem, error) {
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
	h.builder.searchableLayerForApplyingProps = append(h.builder.searchableLayerForApplyingProps, utils.ResultItem{
		Repo: repository,
		Path: expectedImagePath,
		Type: "folder",
	})
	layers, err := h.builder.searchArtifactoryForFilesByPath(repository, []string{expectedImagePath})
	if err != nil {
		return []utils.ResultItem{}, err
	}
	return layers, nil
}

// BuildSearchPaths returns the search path for a single manifest image (imageName/imageTag)
func (h *SingleManifestHandler) BuildSearchPaths(imageName, imageTag, manifestDigest string) string {
	return fmt.Sprintf("%s/%s", imageName, imageTag)
}

// FAT MANIFEST HANDLER IMPLEMENTATION

// FetchLayers fetches layers for a fat manifest (multi-platform) image
func (h *FatManifestHandler) FetchLayers(imageRef string, repository string) ([]utils.ResultItem, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return []utils.ResultItem{}, fmt.Errorf("parsing reference %s: %w", imageRef, err)
	}
	manifestShas := h.getManifestShaListForImage(ref)
	return h.getLayersForManifestSha(imageRef, manifestShas, repository)
}

// BuildSearchPaths returns the search path for a fat manifest image (imageName/sha256:xxx)
func (h *FatManifestHandler) BuildSearchPaths(imageName, imageTag, manifestDigest string) string {
	return fmt.Sprintf("%s/%s", imageName, manifestDigest)
}

// getManifestShaListForImage retrieves all platform manifest SHAs from a fat manifest
func (h *FatManifestHandler) getManifestShaListForImage(imageReference name.Reference) []string {
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

// getLayersForManifestSha searches for layers across all manifest SHAs
func (h *FatManifestHandler) getLayersForManifestSha(imageRef string, manifestShas []string, repository string) ([]utils.ResultItem, error) {
	searchablePathForManifest := h.createSearchablePathForDockerManifestContents(imageRef, manifestShas)

	for _, path := range searchablePathForManifest {
		h.builder.searchableLayerForApplyingProps = append(h.builder.searchableLayerForApplyingProps, utils.ResultItem{
			Repo: repository,
			Path: path,
			Type: "folder",
		})
	}

	layers, err := h.builder.searchArtifactoryForFilesByPath(repository, searchablePathForManifest)
	if err != nil {
		return []utils.ResultItem{}, err
	}
	return layers, nil
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
