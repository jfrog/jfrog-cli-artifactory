package ocicontainer

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ExtractLayersFromManifestData extracts image layers using manifest layer data.
// configDigest is the config layer digest (from manifest.Config.Digest).
// layerDigests is a slice of layer digests with their media types: []struct{Digest, MediaType string}
func ExtractLayersFromManifestData(candidateLayers map[string]*utils.ResultItem, configDigest string, layerDigests []struct{ Digest, MediaType string }) ([]utils.ResultItem, error) {
	var imageLayers []utils.ResultItem

	// Add manifest.json
	if manifestItem, ok := candidateLayers[ManifestJsonFile]; ok {
		imageLayers = append(imageLayers, *manifestItem)
	} else {
		return nil, errorutils.CheckErrorf("manifest.json not found in candidate layers")
	}

	// Add config layer
	configLayerName := digestToLayer(configDigest)
	if configLayer, ok := candidateLayers[configLayerName]; ok {
		imageLayers = append(imageLayers, *configLayer)
	} else {
		return nil, errorutils.CheckErrorf("config layer %s not found in candidate layers", configLayerName)
	}

	// Add all layers from manifest
	for _, layerItem := range layerDigests {
		layerFileName := digestToLayer(layerItem.Digest)
		item, layerExists := candidateLayers[layerFileName]
		if !layerExists {
			err := handleForeignLayer(layerItem.MediaType, layerFileName)
			if err != nil {
				return nil, err
			}
			continue
		}
		imageLayers = append(imageLayers, *item)
	}

	return imageLayers, nil
}

// SearchLayersForDetailedSummary searches for container image layers in Artifactory
// without using build info builders, returning layers for detailed summary display.
// This function searches for layers, extracts them from the manifest, and returns them
// in a format suitable for displaying a detailed summary.
func SearchLayersForDetailedSummary(image *Image, repo string, serviceManager artifactory.ArtifactoryServicesManager, imageSha256 string) (*[]utils.ResultItem, error) {
	// Get repository details to determine searchable repo
	repoDetails := &services.RepositoryDetails{}
	err := serviceManager.GetRepository(repo, repoDetails)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to get details for repository '%s'. Error:\n%s", repo, err.Error())
	}

	isRemote := repoDetails.GetRepoType() == "remote"
	searchableRepo := repo
	if isRemote {
		searchableRepo = repo + "-cache"
	}

	// Get image path
	longImageName, err := image.GetImageLongNameWithTag()
	if err != nil {
		return nil, err
	}
	imagePath := strings.Replace(longImageName, ":", "/", 1)

	// Get manifest paths
	manifestPathsCandidates := getManifestPaths(imagePath, searchableRepo, Push)

	var resultMap map[string]*utils.ResultItem
	var imageManifest *manifest

	// Search for manifest and layers
	for _, searchPath := range manifestPathsCandidates {
		log.Debug(`Searching in:"` + searchPath + `"`)
		resultMap, err = performSearch(searchPath, serviceManager)
		if err != nil {
			log.Debug("Failed to search layers. Error:", err.Error())
			continue
		}
		if len(resultMap) == 0 {
			continue
		}

		imageManifest, err = getManifest(resultMap, serviceManager, repo)
		if err != nil {
			// Check if error is 403 Forbidden (download blocked by Xray policy)
			if strings.Contains(err.Error(), "download blocking policy configured in Xray") {
				log.Info("Artifact download blocked by Xray policy. Returning basic summary with available files.")
				// Return all found files as basic summary (excluding manifest.json since we can't download it)
				var basicSummary []utils.ResultItem
				for fileName, item := range resultMap {
					if fileName != ManifestJsonFile {
						basicSummary = append(basicSummary, *item)
					}
				}
				if len(basicSummary) > 0 {
					log.Info(fmt.Sprintf("Found %d file(s) in repository.", len(basicSummary)))
					return &basicSummary, nil
				}
				// If no files found, return empty result without error
				return &[]utils.ResultItem{}, nil
			}
			log.Debug("Failed to get manifest")
			continue
		}
		if imageManifest != nil {
			// Verify manifest if we have image SHA
			if imageSha256 != "" {
				if imageManifest.Config.Digest != imageSha256 {
					log.Debug(`Found incorrect manifest.json file. Expects digest "` + imageSha256 + `" found "` + imageManifest.Config.Digest)
					continue
				}
			}
			break
		}
	}

	if imageManifest == nil {
		return nil, errorutils.CheckErrorf("could not find image manifest in Artifactory")
	}

	// Extract layers using the reusable helper function
	layerDigests := make([]struct{ Digest, MediaType string }, len(imageManifest.Layers))
	for i, layerItem := range imageManifest.Layers {
		layerDigests[i] = struct{ Digest, MediaType string }{
			Digest:    layerItem.Digest,
			MediaType: layerItem.MediaType,
		}
	}

	imageLayers, err := ExtractLayersFromManifestData(resultMap, imageManifest.Config.Digest, layerDigests)
	if err != nil {
		return nil, err
	}

	return &imageLayers, nil
}
