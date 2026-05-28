package ocicontainer

import (
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

const testRepo = "docker-local"

func newLayer(path, name string) *utils.ResultItem {
	return &utils.ResultItem{
		Repo: testRepo,
		Path: path,
		Name: name,
	}
}

func TestLayerKeyIsUniquePerLocation(t *testing.T) {
	a := newLayer("myimg/sha256__aaa", "manifest.json")
	b := newLayer("myimg/sha256__bbb", "manifest.json")
	c := newLayer("myimg/sha256__aaa", "manifest.json")

	assert.NotEqual(t, layerKey(a), layerKey(b))
	assert.Equal(t, layerKey(a), layerKey(c))
}

func TestCollectBasicSummarySkipsManifestEntries(t *testing.T) {
	resultMap := map[string]*utils.ResultItem{
		ManifestJsonFile:    newLayer("myimg/latest", ManifestJsonFile),
		FatManifestJsonFile: newLayer("myimg/latest", FatManifestJsonFile),
		"sha256__layer1":    newLayer("myimg/latest", "sha256__layer1"),
		"sha256__layer2":    newLayer("myimg/latest", "sha256__layer2"),
	}

	summary := collectBasicSummary(resultMap)

	assert.Len(t, summary, 2)
	for _, item := range summary {
		assert.NotEqual(t, ManifestJsonFile, item.Name)
		assert.NotEqual(t, FatManifestJsonFile, item.Name)
	}
}

func TestAggregateFatManifestLayersIncludesAllPlatforms(t *testing.T) {
	fatManifestItem := newLayer("myimg/latest", FatManifestJsonFile)

	amdManifest := newLayer("myimg/sha256__amd", ManifestJsonFile)
	amdConfig := newLayer("myimg/sha256__amd", "sha256__amdconfig")
	amdLayer := newLayer("myimg/sha256__amd", "sha256__amdlayer")

	armManifest := newLayer("myimg/sha256__arm", ManifestJsonFile)
	armConfig := newLayer("myimg/sha256__arm", "sha256__armconfig")
	armLayer := newLayer("myimg/sha256__arm", "sha256__armlayer")

	fatManifest := &FatManifest{
		Manifests: []ManifestDetails{
			{Digest: "sha256:amd", Platform: Platform{Os: "linux", Architecture: "amd64"}},
			{Digest: "sha256:arm", Platform: Platform{Os: "linux", Architecture: "arm64"}},
		},
	}

	multiPlatformImages := map[string][]*utils.ResultItem{
		"sha256:amd": {amdManifest, amdConfig, amdLayer},
		"sha256:arm": {armManifest, armConfig, armLayer},
	}

	layers := aggregateFatManifestLayers(fatManifestItem, fatManifest, multiPlatformImages)

	assert.Len(t, layers, 7)
	assert.Equal(t, FatManifestJsonFile, layers[0].Name)

	keys := make(map[string]bool)
	for _, layer := range layers {
		keys[layerKey(&layer)] = true
	}
	assert.True(t, keys[layerKey(fatManifestItem)])
	assert.True(t, keys[layerKey(amdManifest)])
	assert.True(t, keys[layerKey(amdConfig)])
	assert.True(t, keys[layerKey(amdLayer)])
	assert.True(t, keys[layerKey(armManifest)])
	assert.True(t, keys[layerKey(armConfig)])
	assert.True(t, keys[layerKey(armLayer)])
}

func TestAggregateFatManifestLayersDeduplicatesSharedLayers(t *testing.T) {
	fatManifestItem := newLayer("myimg/latest", FatManifestJsonFile)

	sharedLayer := newLayer("myimg/sha256__amd", "sha256__shared")
	amdManifest := newLayer("myimg/sha256__amd", ManifestJsonFile)
	armManifest := newLayer("myimg/sha256__arm", ManifestJsonFile)

	fatManifest := &FatManifest{
		Manifests: []ManifestDetails{
			{Digest: "sha256:amd"},
			{Digest: "sha256:arm"},
		},
	}

	// Same layer object referenced by both platforms (e.g. promoted base layer).
	multiPlatformImages := map[string][]*utils.ResultItem{
		"sha256:amd": {amdManifest, sharedLayer},
		"sha256:arm": {armManifest, sharedLayer},
	}

	layers := aggregateFatManifestLayers(fatManifestItem, fatManifest, multiPlatformImages)

	assert.Len(t, layers, 4)
}

func TestAggregateFatManifestLayersSkipsMissingPlatformLayers(t *testing.T) {
	fatManifestItem := newLayer("myimg/latest", FatManifestJsonFile)
	amdManifest := newLayer("myimg/sha256__amd", ManifestJsonFile)

	fatManifest := &FatManifest{
		Manifests: []ManifestDetails{
			{Digest: "sha256:amd"},
			{Digest: "sha256:missing"},
		},
	}

	multiPlatformImages := map[string][]*utils.ResultItem{
		"sha256:amd": {amdManifest},
	}

	layers := aggregateFatManifestLayers(fatManifestItem, fatManifest, multiPlatformImages)

	assert.Len(t, layers, 2)
	assert.Equal(t, FatManifestJsonFile, layers[0].Name)
	assert.Equal(t, ManifestJsonFile, layers[1].Name)
}

func TestAggregateFatManifestLayersWithNoPlatforms(t *testing.T) {
	fatManifestItem := newLayer("myimg/latest", FatManifestJsonFile)
	fatManifest := &FatManifest{Manifests: []ManifestDetails{}}
	multiPlatformImages := map[string][]*utils.ResultItem{}

	layers := aggregateFatManifestLayers(fatManifestItem, fatManifest, multiPlatformImages)

	assert.Len(t, layers, 1)
	assert.Equal(t, FatManifestJsonFile, layers[0].Name)
}

func TestExtractLayersFromManifestDataMissingManifest(t *testing.T) {
	_, err := ExtractLayersFromManifestData(map[string]*utils.ResultItem{}, "sha256:abc", nil)
	assert.ErrorContains(t, err, "manifest.json not found in candidate layers")
}

func TestExtractLayersFromManifestDataMissingConfig(t *testing.T) {
	candidates := map[string]*utils.ResultItem{
		ManifestJsonFile: newLayer("myimg/latest", ManifestJsonFile),
	}
	_, err := ExtractLayersFromManifestData(candidates, "sha256:abc", nil)
	assert.ErrorContains(t, err, "config layer sha256__abc not found")
}
