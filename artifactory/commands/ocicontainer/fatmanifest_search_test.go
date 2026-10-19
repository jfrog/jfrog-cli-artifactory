package ocicontainer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fatManifestTestRepo  = "docker-local"
	fatManifestTestImage = "org/img"
	fatManifestTestTag   = "1.0"
)

// Simulates an image with many tags, each pushed as a multi-platform image.
type fakeDockerRepo struct {
	files []utils.ResultItem
}

func newFakeDockerRepo(tags int) *fakeDockerRepo {
	repo := &fakeDockerRepo{}
	for i := 0; i < tags; i++ {
		tag := fmt.Sprintf("%d.0", i+1)
		repo.addFile(fatManifestTestImage+"/"+tag, FatManifestJsonFile, "fat"+tag)
		for _, platform := range []string{"amd64", "arm64"} {
			folder := fatManifestTestImage + "/sha256:" + platformDigestHex(tag, platform)
			repo.addFile(folder, ManifestJsonFile, platformDigestHex(tag, platform))
			repo.addFile(folder, "sha256__config"+tag+platform, "config"+tag+platform)
			repo.addFile(folder, "sha256__layer"+tag+platform, "layer"+tag+platform)
		}
	}
	return repo
}

func platformDigestHex(tag, platform string) string {
	return "digest-" + tag + "-" + platform
}

func (repo *fakeDockerRepo) addFile(path, name, sha256 string) {
	repo.files = append(repo.files, utils.ResultItem{Repo: fatManifestTestRepo, Path: path, Name: name, Type: "file", Sha256: sha256})
}

// Returns the files matching an exact-paths query, or all files under the image root for a recursive root query.
func (repo *fakeDockerRepo) find(aql string) []utils.ResultItem {
	var results []utils.ResultItem
	recursiveRootQuery := strings.Contains(aql, `"$match":"`+fatManifestTestImage+`/*"`)
	for _, file := range repo.files {
		if recursiveRootQuery && strings.HasPrefix(file.Path, fatManifestTestImage+"/") ||
			strings.Contains(aql, fmt.Sprintf(`{"path":%q}`, file.Path)) {
			results = append(results, file)
		}
	}
	return results
}

type aqlRecorder struct {
	queries      []string
	returnedRows int
}

func newFakeArtifactory(t *testing.T, repo *fakeDockerRepo) (artifactory.ArtifactoryServicesManager, *aqlRecorder) {
	recorder := &aqlRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/system/version") {
			_, _ = w.Write([]byte(`{"version":"7.104.0"}`))
			return
		}
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(r.URL.Path, "/api/search/aql"), r.URL.Path)
		recorder.queries = append(recorder.queries, string(body))
		results := repo.find(string(body))
		recorder.returnedRows += len(results)
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{"results": results}))
	}))
	t.Cleanup(server.Close)
	details := auth.NewArtifactoryDetails()
	details.SetUrl(server.URL + "/artifactory/")
	serviceConfig, err := config.NewConfigBuilder().SetServiceDetails(details).Build()
	require.NoError(t, err)
	serviceManager, err := artifactory.New(serviceConfig)
	require.NoError(t, err)
	return serviceManager, recorder
}

func testFatManifest(tag string) (*utils.ResultItem, *FatManifest) {
	fatManifestResult := &utils.ResultItem{Repo: fatManifestTestRepo, Path: fatManifestTestImage + "/" + tag, Name: FatManifestJsonFile}
	fatManifest := &FatManifest{Manifests: []ManifestDetails{
		{Digest: "sha256:" + platformDigestHex(tag, "amd64"), Platform: Platform{Os: "linux", Architecture: "amd64"}},
		{Digest: "sha256:" + platformDigestHex(tag, "arm64"), Platform: Platform{Os: "linux", Architecture: "arm64"}},
	}}
	return fatManifestResult, fatManifest
}

func TestPerformMultiPlatformImageSearchUsesExactPaths(t *testing.T) {
	serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(3))
	fatManifestResult, fatManifest := testFatManifest(fatManifestTestTag)

	resultMap, err := performMultiPlatformImageSearch(fatManifestResult, fatManifest, serviceManager)
	require.NoError(t, err)

	require.Len(t, recorder.queries, 1)
	query := recorder.queries[0]
	assert.NotContains(t, query, "$match")
	assert.NotContains(t, query, `"property"`)
	assert.Contains(t, query, `"repo":"docker-local"`)
	for _, platform := range []string{"amd64", "arm64"} {
		hex := platformDigestHex(fatManifestTestTag, platform)
		assert.Contains(t, query, `{"path":"org/img/sha256:`+hex+`"}`)
		assert.Contains(t, query, `{"path":"org/img/sha256__`+hex+`"}`)
	}

	require.Len(t, resultMap, 2)
	for _, manifest := range fatManifest.Manifests {
		layers := resultMap[manifest.Digest]
		require.Len(t, layers, 3)
		for _, layer := range layers {
			assert.Equal(t, fatManifestTestImage+"/"+manifest.Digest, layer.Path)
		}
	}
}

func TestPerformMultiPlatformImageSearchWithoutManifests(t *testing.T) {
	serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(1))
	fatManifestResult, _ := testFatManifest(fatManifestTestTag)

	resultMap, err := performMultiPlatformImageSearch(fatManifestResult, &FatManifest{}, serviceManager)
	require.NoError(t, err)
	assert.Empty(t, resultMap)
	assert.Empty(t, recorder.queries)
}

func TestGetPlatformManifestsPaths(t *testing.T) {
	fatManifest := &FatManifest{Manifests: []ManifestDetails{{Digest: "sha256:aaa"}, {Digest: "sha256:aaa"}, {Digest: ""}, {Digest: "sha256:bbb"}}}
	assert.Equal(t,
		[]string{"org/img/sha256:aaa", "org/img/sha256__aaa", "org/img/sha256:bbb", "org/img/sha256__bbb"},
		getPlatformManifestsPaths("org/img", fatManifest))
}

// The rows returned for a multi-platform image must not depend on the number of tags pushed under the image.
func TestMultiPlatformImageSearchRowsIndependentOfTagsCount(t *testing.T) {
	for _, tags := range []int{1, 100, 10000} {
		t.Run(fmt.Sprintf("tags=%d", tags), func(t *testing.T) {
			serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(tags))
			fatManifestResult, fatManifest := testFatManifest(fatManifestTestTag)
			_, err := performMultiPlatformImageSearch(fatManifestResult, fatManifest, serviceManager)
			require.NoError(t, err)
			// 2 platforms * (manifest.json + config + layer)
			assert.Equal(t, 6, recorder.returnedRows)
		})
	}
}
