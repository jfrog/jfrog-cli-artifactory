package ocicontainer

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetManifestPaths(t *testing.T) {
	results := getManifestPaths("/hello-world/latest", "docker-local", Push)
	assert.Len(t, results, 2)
	expected := []string{"docker-local/hello-world/latest/*", "docker-local/latest/*"}
	for i, result := range results {
		assert.Equal(t, expected[i], result)
	}

	results = getManifestPaths("/hello-world/latest", "docker-local", Pull)
	assert.Len(t, results, 4)
	expected = append(expected, "docker-local/library/hello-world/latest/*", "docker-local/library/latest/*")
	for i, result := range results {
		assert.Equal(t, expected[i], result)
	}

	results = getManifestPaths("/docker-remote/hello-world/latest", "docker-remote", Push)
	assert.Len(t, results, 2)
	expected = []string{"docker-remote/docker-remote/hello-world/latest/*", "docker-remote/hello-world/latest/*"}
	for i, result := range results {
		assert.Equal(t, expected[i], result)
	}

	results = getManifestPaths("/docker-remote/hello-world/latest", "docker-remote", Pull)
	assert.Len(t, results, 4)
	expected = append(expected, "docker-remote/library/docker-remote/hello-world/latest/*", "docker-remote/library/hello-world/latest/*")
	for i, result := range results {
		assert.Equal(t, expected[i], result)
	}
}

func TestGetImageWithDigest(t *testing.T) {
	filePath := filepath.Join("..", "testdata", "container", "imageTagWithDigest")
	tag, sha256, err := GetImageTagWithDigest(filePath)
	assert.NoError(t, err)
	assert.Equal(t, "my-image-tag", tag.name)
	assert.Equal(t, "sha256:12345", sha256)
}

var dummySearchResults = &utils.ResultItem{
	Type:        "json",
	Actual_Md5:  "md5",
	Actual_Sha1: "sha1",
	Sha256:      "sha2",
	Size:        1,
}

func TestManifestConfig(t *testing.T) {
	dependencies, err := getDependenciesFromManifestConfig(createManifestConfig())
	assert.NoError(t, err)
	assert.Len(t, dependencies, 2)
}

func createManifestConfig() (map[string]*utils.ResultItem, string) {
	config := make(map[string]*utils.ResultItem)
	config["manifest.json"] = dummySearchResults
	config["sha__123"] = dummySearchResults
	return config, "sha:123"
}

func TestManifestConfigNoManifestFound(t *testing.T) {
	_, err := getDependenciesFromManifestConfig(createEmptyManifestConfig())
	assert.ErrorContains(t, err, "The manifest.json was not found in Artifactory")
}

func createEmptyManifestConfig() (map[string]*utils.ResultItem, string) {
	config := make(map[string]*utils.ResultItem)
	return config, "sha:123"
}

func TestManifestConfigNoLayer(t *testing.T) {
	_, err := getDependenciesFromManifestConfig(createManifestConfigWithNoLayer())
	assert.ErrorContains(t, err, "Image 'sha:123' was not found in Artifactory")
}

func createManifestConfigWithNoLayer() (map[string]*utils.ResultItem, string) {
	config := make(map[string]*utils.ResultItem)
	config["manifest.json"] = dummySearchResults
	return config, "sha:123"
}

func TestGetDependenciesFromManifestLayer(t *testing.T) {
	searchResults, manifest := createManifestConfigWithLayer()
	dependencies, err := getDependenciesFromManifestLayer(searchResults, manifest)
	assert.NoError(t, err)
	assert.Len(t, dependencies, 1)
}

func createManifestConfigWithLayer() (map[string]*utils.ResultItem, *manifest) {
	manifest := &manifest{
		Layers: []layer{{
			Digest:    "sha:1",
			MediaType: "MediaType",
		}},
	}
	searchResults := make(map[string]*utils.ResultItem)
	searchResults["manifest.json"] = dummySearchResults
	searchResults["sha__1"] = dummySearchResults
	searchResults["sha__2"] = dummySearchResults
	return searchResults, manifest
}

func TestMissingDependenciesInManifestLayer(t *testing.T) {
	searchResults, manifest := createManifestConfigWithMissingLayer()
	_, err := getDependenciesFromManifestLayer(searchResults, manifest)
	assert.ErrorContains(t, err, "Could not find layer: sha__2 in Artifactory")
}

func createManifestConfigWithMissingLayer() (map[string]*utils.ResultItem, *manifest) {
	manifest := &manifest{
		Layers: []layer{
			{
				Digest:    "sha:1",
				MediaType: "MediaType",
			},
			//  Missing layer
			{
				Digest:    "sha:2",
				MediaType: "type",
			},
		},
	}
	searchResults := make(map[string]*utils.ResultItem)
	searchResults["manifest.json"] = dummySearchResults
	searchResults["sha__1"] = dummySearchResults
	return searchResults, manifest
}

func TestForeignDependenciesInManifestLayer(t *testing.T) {
	searchResults, manifest := createManifestConfigWithForeignLayer()
	dependencies, err := getDependenciesFromManifestLayer(searchResults, manifest)
	assert.NoError(t, err)
	assert.Len(t, dependencies, 1)
}

func createManifestConfigWithForeignLayer() (map[string]*utils.ResultItem, *manifest) {
	manifest := &manifest{
		Layers: []layer{
			{
				Digest:    "sha:1",
				MediaType: "MediaType",
			},
			//  Foreign layer
			{
				Digest:    "sha:2",
				MediaType: foreignLayerMediaType,
			},
		},
	}
	searchResults := make(map[string]*utils.ResultItem)
	searchResults["manifest.json"] = dummySearchResults
	searchResults["sha__1"] = dummySearchResults
	return searchResults, manifest
}

// fakeAqlManager implements only the Aql method of the services manager, and records the queries it receives.
type fakeAqlManager struct {
	artifactory.ArtifactoryServicesManager
	queries  []string
	response string
	err      error
}

func (f *fakeAqlManager) Aql(aql string) (io.ReadCloser, error) {
	f.queries = append(f.queries, aql)
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.response)), nil
}

func TestGetFatManifestPlatformPaths(t *testing.T) {
	hexA, hexB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	fatManifestResult := &utils.ResultItem{Repo: "docker-local", Path: "org/hello-world/1"}
	pathsOf := func(hex string) []string {
		return []string{"org/hello-world/sha256:" + hex, "org/hello-world/sha256__" + hex}
	}

	testCases := []struct {
		name     string
		digests  []string
		expected []string
	}{
		{name: "two platforms", digests: []string{"sha256:" + hexA, "sha256:" + hexB}, expected: append(pathsOf(hexA), pathsOf(hexB)...)},
		{name: "no manifests", digests: nil, expected: []string{}},
		// Invalid digests never matched a platform folder, so a fat manifest holding only invalid digests is not an error.
		// The build-info then holds only the fat manifest.
		{name: "all digests invalid", digests: []string{`sha256:a"}`, "abc"}, expected: []string{}},
		{name: "invalid digest is skipped", digests: []string{`sha256:a"}`, "sha256:" + hexA}, expected: pathsOf(hexA)},
		{name: "sha512 digest is skipped", digests: []string{"sha512:" + strings.Repeat("c", 128), "sha256:" + hexA}, expected: pathsOf(hexA)},
		{name: "short digest is skipped", digests: []string{"sha256:abc"}, expected: []string{}},
		{name: "uppercase digest is skipped", digests: []string{"sha256:" + strings.ToUpper(hexA)}, expected: []string{}},
		{name: "digest without algorithm is skipped", digests: []string{hexA}, expected: []string{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fatManifest := &FatManifest{}
			for _, digest := range testCase.digests {
				fatManifest.Manifests = append(fatManifest.Manifests, ManifestDetails{Digest: digest})
			}
			paths, err := getFatManifestPlatformPaths(fatManifestResult, fatManifest)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expected, paths)
		})
	}

	t.Run("nil fat manifest", func(t *testing.T) {
		_, err := getFatManifestPlatformPaths(fatManifestResult, nil)
		assert.Error(t, err)
	})
}

func TestSearchFatManifestPlatformImages(t *testing.T) {
	hexA, hexB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	// The fat manifest was found in the remote cache repository, so this is the repository to search.
	fatManifestResult := &utils.ResultItem{Repo: "docker-remote-cache", Path: "org/hello-world/1", Name: FatManifestJsonFile}
	fatManifest := &FatManifest{Manifests: []ManifestDetails{{Digest: "sha256:" + hexA}, {Digest: "sha256:" + hexB}}}

	t.Run("searches only the platform folders", func(t *testing.T) {
		fake := &fakeAqlManager{response: `{"results":[
			{"repo":"docker-remote-cache","path":"org/hello-world/sha256__` + hexA + `","name":"manifest.json","sha256":"` + hexA + `"},
			{"repo":"docker-remote-cache","path":"org/hello-world/sha256__` + hexA + `","name":"sha256__layer1"},
			{"repo":"docker-remote-cache","path":"org/hello-world/sha256__` + hexB + `","name":"manifest.json","sha256":"` + hexB + `"},
			{"repo":"docker-remote-cache","path":"org/hello-world/sha256__` + hexB + `","name":"sha256__layer2"}]}`}

		result, err := searchFatManifestPlatformImages(fatManifestResult, fatManifest, fake)
		require.NoError(t, err)

		require.Len(t, fake.queries, 1)
		assert.Contains(t, fake.queries[0], `{ "repo": "docker-remote-cache" }`)
		for _, hex := range []string{hexA, hexB} {
			assert.Contains(t, fake.queries[0], `{"path": {"$eq": "org/hello-world/sha256:`+hex+`"}}`)
			assert.Contains(t, fake.queries[0], `{"path": {"$eq": "org/hello-world/sha256__`+hex+`"}}`)
		}
		assert.Equal(t, 4, strings.Count(fake.queries[0], `"$eq"`))
		assert.NotContains(t, fake.queries[0], "$match")

		require.Len(t, result, 2)
		assert.Equal(t, []string{ManifestJsonFile, "sha256__layer1"}, layerNames(result["sha256:"+hexA]))
		assert.Equal(t, []string{ManifestJsonFile, "sha256__layer2"}, layerNames(result["sha256:"+hexB]))
	})

	t.Run("returns the AQL error", func(t *testing.T) {
		errNetwork := errors.New("network down")
		result, err := searchFatManifestPlatformImages(fatManifestResult, fatManifest, &fakeAqlManager{err: errNetwork})
		assert.ErrorIs(t, err, errNetwork)
		assert.Nil(t, result)
	})

	t.Run("does not search without platform digests", func(t *testing.T) {
		fake := &fakeAqlManager{}
		result, err := searchFatManifestPlatformImages(fatManifestResult, &FatManifest{}, fake)
		assert.NoError(t, err)
		assert.Empty(t, result)
		assert.Empty(t, fake.queries)
	})

	t.Run("nil fat manifest", func(t *testing.T) {
		fake := &fakeAqlManager{}
		_, err := searchFatManifestPlatformImages(fatManifestResult, nil, fake)
		assert.Error(t, err)
		assert.Empty(t, fake.queries)
	})
}

func layerNames(layers []*utils.ResultItem) []string {
	names := make([]string, 0, len(layers))
	for _, layer := range layers {
		names = append(names, layer.Name)
	}
	return names
}

func TestGroupLayersByManifestDigest(t *testing.T) {
	items := []utils.ResultItem{
		{Path: "hello-world/sha256:aaa", Name: ManifestJsonFile, Sha256: "aaa"},
		{Path: "hello-world/sha256:aaa", Name: "sha256__layer1"},
		{Path: "hello-world/sha256:bbb", Name: ManifestJsonFile, Sha256: "bbb"},
		{Path: "hello-world/sha256:bbb", Name: "sha256__layer2"},
		// Folder without manifest.json is ignored.
		{Path: "hello-world/1", Name: FatManifestJsonFile},
	}
	result := groupLayersByManifestDigest(items)
	assert.Len(t, result, 2)
	assert.Equal(t, []string{ManifestJsonFile, "sha256__layer1"}, layerNames(result["sha256:aaa"]))
	assert.Equal(t, []string{ManifestJsonFile, "sha256__layer2"}, layerNames(result["sha256:bbb"]))
}
