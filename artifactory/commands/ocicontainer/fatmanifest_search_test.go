package ocicontainer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/build"
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
	// The prefix of the platform manifest folders: "sha256:" or "sha256__".
	digestFolderPrefix string
}

func newFakeDockerRepo(tags int) *fakeDockerRepo {
	return newFakeDockerRepoWithLayout(tags, "sha256:")
}

func newFakeDockerRepoWithLayout(tags int, digestFolderPrefix string) *fakeDockerRepo {
	repo := &fakeDockerRepo{digestFolderPrefix: digestFolderPrefix}
	for i := 0; i < tags; i++ {
		tag := fmt.Sprintf("%d.0", i+1)
		repo.addFile(fatManifestTestImage+"/"+tag, FatManifestJsonFile, "fat"+tag)
		for _, platform := range []string{"amd64", "arm64"} {
			folder := fatManifestTestImage + "/" + digestFolderPrefix + platformDigestHex(tag, platform)
			repo.addFile(folder, ManifestJsonFile, platformDigestHex(tag, platform))
			repo.addFile(folder, "sha256__config"+tag+platform, "config"+tag+platform)
			repo.addFile(folder, "sha256__layer"+tag+platform, "layer"+tag+platform)
		}
	}
	return repo
}

func platformDigestHex(tag, platform string) string {
	sum := sha256.Sum256([]byte(tag + "/" + platform))
	return hex.EncodeToString(sum[:])
}

func (repo *fakeDockerRepo) addFile(path, name, sha256 string) {
	repo.files = append(repo.files, utils.ResultItem{Repo: fatManifestTestRepo, Path: path, Name: name, Type: "file", Size: 1,
		Sha256: sha256, Actual_Sha1: "sha1-" + sha256, Actual_Md5: "md5-" + sha256,
		Properties: []utils.Property{{Key: "sha256", Value: sha256}}})
}

var aqlFindRegex = regexp.MustCompile(`^items\.find\((.*)\)\.include\((.*)\)$`)

// Evaluates an exact-paths AQL query: a repo and type condition and an $or of exact paths.
// Only the fields listed in the include part are returned, like Artifactory does.
func (repo *fakeDockerRepo) find(t *testing.T, aql string) []utils.ResultItem {
	match := aqlFindRegex.FindStringSubmatch(aql)
	require.Len(t, match, 3, aql)
	var criteria struct {
		Repo string              `json:"repo"`
		Type string              `json:"type"`
		Or   []map[string]string `json:"$or"`
	}
	require.NoError(t, json.Unmarshal([]byte(match[1]), &criteria), aql)
	var fields []string
	require.NoError(t, json.Unmarshal([]byte("["+match[2]+"]"), &fields), aql)
	paths := make(map[string]bool)
	for _, condition := range criteria.Or {
		paths[condition["path"]] = true
	}
	var results []utils.ResultItem
	for _, file := range repo.files {
		if file.Repo == criteria.Repo && (criteria.Type == "" || file.Type == criteria.Type) && paths[file.Path] {
			results = append(results, includeFields(file, fields))
		}
	}
	return results
}

func includeFields(file utils.ResultItem, fields []string) utils.ResultItem {
	result := utils.ResultItem{}
	for _, field := range fields {
		switch field {
		case "name":
			result.Name = file.Name
		case "repo":
			result.Repo = file.Repo
		case "path":
			result.Path = file.Path
		case "type":
			result.Type = file.Type
		case "size":
			result.Size = file.Size
		case "sha256":
			result.Sha256 = file.Sha256
		case "actual_sha1":
			result.Actual_Sha1 = file.Actual_Sha1
		case "actual_md5":
			result.Actual_Md5 = file.Actual_Md5
		case "property":
			result.Properties = file.Properties
		}
	}
	return result
}

type aqlRecorder struct {
	queries      []string
	returnedRows int
	// The paths build properties were set on.
	propsPaths []string
	// The fat manifest returned when downloading list.manifest.json.
	fatManifest *FatManifest
	// If set, AQL requests fail with this status.
	failWithStatus int
	propsMutex     sync.Mutex
}

func newFakeArtifactory(t *testing.T, repo *fakeDockerRepo) (artifactory.ArtifactoryServicesManager, *aqlRecorder) {
	recorder := &aqlRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/system/version") {
			_, _ = w.Write([]byte(`{"version":"7.104.0"}`))
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/"+FatManifestJsonFile) {
			fatManifest := recorder.fatManifest
			if fatManifest == nil {
				_, fatManifest = testFatManifest()
			}
			assert.NoError(t, json.NewEncoder(w).Encode(fatManifest))
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/storage/") {
			assert.Contains(t, r.URL.RawQuery, "build.name")
			recorder.propsMutex.Lock()
			recorder.propsPaths = append(recorder.propsPaths, strings.SplitN(r.URL.Path, "/api/storage/", 2)[1])
			recorder.propsMutex.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(r.URL.Path, "/api/search/aql"), r.URL.Path)
		recorder.queries = append(recorder.queries, string(body))
		if recorder.failWithStatus != 0 {
			w.WriteHeader(recorder.failWithStatus)
			return
		}
		results := repo.find(t, string(body))
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

func testFatManifest() (*utils.ResultItem, *FatManifest) {
	tag := fatManifestTestTag
	fatManifestResult := &utils.ResultItem{Repo: fatManifestTestRepo, Path: fatManifestTestImage + "/" + tag, Name: FatManifestJsonFile}
	fatManifest := &FatManifest{Manifests: []ManifestDetails{
		{Digest: "sha256:" + platformDigestHex(tag, "amd64"), Platform: Platform{Os: "linux", Architecture: "amd64"}},
		{Digest: "sha256:" + platformDigestHex(tag, "arm64"), Platform: Platform{Os: "linux", Architecture: "arm64"}},
	}}
	return fatManifestResult, fatManifest
}

func TestPerformMultiPlatformImageSearchUsesExactPaths(t *testing.T) {
	serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(3))
	fatManifestResult, fatManifest := testFatManifest()

	resultMap, err := performMultiPlatformImageSearch(fatManifestResult, fatManifest, serviceManager)
	require.NoError(t, err)

	require.Len(t, recorder.queries, 1)
	query := recorder.queries[0]
	assert.NotContains(t, query, "$match")
	assert.NotContains(t, query, `property`)
	assert.Contains(t, query, `"repo":"docker-local"`)
	for _, platform := range []string{"amd64", "arm64"} {
		hex := platformDigestHex(fatManifestTestTag, platform)
		assert.Contains(t, query, `{"path":"org/img/sha256:`+hex+`"}`)
		assert.Contains(t, query, `{"path":"org/img/sha256__`+hex+`"}`)
	}

	require.Len(t, resultMap, 2)
	for _, manifest := range fatManifest.Manifests {
		assertPlatformLayers(t, resultMap[manifest.Digest], fatManifestTestImage+"/"+manifest.Digest)
	}
}

// Assert the layers of a platform manifest are found, with the fields needed for the build-info.
func assertPlatformLayers(t *testing.T, layers []*utils.ResultItem, expectedPath string) {
	require.Len(t, layers, 3)
	for _, layer := range layers {
		assert.Equal(t, fatManifestTestRepo, layer.Repo)
		assert.Equal(t, expectedPath, layer.Path)
		assert.NotEmpty(t, layer.Name)
		assert.NotEmpty(t, layer.Sha256)
		assert.Equal(t, "sha1-"+layer.Sha256, layer.Actual_Sha1)
		assert.Equal(t, "md5-"+layer.Sha256, layer.Actual_Md5)
		assert.Empty(t, layer.Properties)
	}
}

func TestPerformMultiPlatformImageSearchWithUnderscoreDigestFolders(t *testing.T) {
	serviceManager, _ := newFakeArtifactory(t, newFakeDockerRepoWithLayout(3, "sha256__"))
	fatManifestResult, fatManifest := testFatManifest()

	resultMap, err := performMultiPlatformImageSearch(fatManifestResult, fatManifest, serviceManager)
	require.NoError(t, err)
	require.Len(t, resultMap, 2)
	for _, manifest := range fatManifest.Manifests {
		assertPlatformLayers(t, resultMap[manifest.Digest], fatManifestTestImage+"/"+digestToLayer(manifest.Digest))
	}
}

// The platform manifests must be searched in the repository the fat manifest was found in,
// e.g. the remote repository cache.
func TestPerformMultiPlatformImageSearchUsesFatManifestRepo(t *testing.T) {
	serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(1))
	fatManifestResult, fatManifest := testFatManifest()
	fatManifestResult.Repo = "docker-remote-cache"

	resultMap, err := performMultiPlatformImageSearch(fatManifestResult, fatManifest, serviceManager)
	require.NoError(t, err)
	assert.Empty(t, resultMap)
	require.Len(t, recorder.queries, 1)
	assert.Contains(t, recorder.queries[0], `"repo":"docker-remote-cache"`)
}

func TestPerformMultiPlatformImageSearchAqlError(t *testing.T) {
	serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(1))
	recorder.failWithStatus = http.StatusInternalServerError
	fatManifestResult, fatManifest := testFatManifest()

	resultMap, err := performMultiPlatformImageSearch(fatManifestResult, fatManifest, serviceManager)
	assert.ErrorContains(t, err, "failed to search the platform manifests of 'docker-local/org/img/1.0/list.manifest.json'")
	assert.Nil(t, resultMap)
}

func TestPerformMultiPlatformImageSearchWithoutManifests(t *testing.T) {
	serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(1))
	fatManifestResult, _ := testFatManifest()

	resultMap, err := performMultiPlatformImageSearch(fatManifestResult, &FatManifest{}, serviceManager)
	require.NoError(t, err)
	assert.Empty(t, resultMap)
	assert.Empty(t, recorder.queries)
}

func TestGetPlatformManifestsPaths(t *testing.T) {
	aaa, bbb := strings.Repeat("a", 64), strings.Repeat("b", 64)
	fatManifest := &FatManifest{Manifests: []ManifestDetails{{Digest: "sha256:" + aaa}, {Digest: "sha256:" + aaa}, {Digest: "sha256:" + bbb}}}
	assert.Equal(t,
		[]string{"org/img/sha256:" + aaa, "org/img/sha256__" + aaa, "org/img/sha256:" + bbb, "org/img/sha256__" + bbb},
		getPlatformManifestPaths("org/img", fatManifest))
}

// The fat manifest content is uploaded by the image publisher, so its digests must not be able to change the searched paths.
func TestGetPlatformManifestsPathsSkipsInvalidDigests(t *testing.T) {
	valid := "sha256:" + strings.Repeat("a", 64)
	invalidDigests := []string{
		"",
		"sha256:",
		"sha256:aaa",
		"sha256:" + strings.Repeat("A", 64),
		"sha256:" + strings.Repeat("a", 65),
		"sha512:" + strings.Repeat("a", 128),
		"../../other-image/" + valid,
		"sha256:../../other-image",
		valid + "/../../other-image",
		valid + `"},{"path":{"$match":"*"}}`,
		valid + "\n",
		"sha256:" + strings.Repeat("a", 63) + "\x00",
	}
	manifests := []ManifestDetails{{Digest: valid}}
	for _, digest := range invalidDigests {
		manifests = append(manifests, ManifestDetails{Digest: digest})
	}
	assert.Equal(t,
		[]string{"org/img/" + valid, "org/img/" + digestToLayer(valid)},
		getPlatformManifestPaths("org/img", &FatManifest{Manifests: manifests}))
}

func TestCreatePlatformManifestsAqlEscapesValues(t *testing.T) {
	maliciousRepo := `docker-local","$or":[{"path":{"$match":"*"}}],"repo":"x`
	maliciousPath := "org/img\"},{\"path\":{\"$match\":\"*\"}}"
	query, err := createExactPathsAql(maliciousRepo, []string{maliciousPath, "a\x00\x7f\xff"}, true, platformManifestsReturnFields)
	require.NoError(t, err)

	// The query must parse as a single repo condition and exact path conditions, holding the values unchanged.
	criteria := strings.TrimSuffix(strings.TrimPrefix(query, "items.find("), `).include("name","repo","path","actual_md5","actual_sha1","sha256","size","type","modified","created")`)
	var parsed struct {
		Repo string              `json:"repo"`
		Type string              `json:"type"`
		Or   []map[string]string `json:"$or"`
	}
	require.NoError(t, json.Unmarshal([]byte(criteria), &parsed), criteria)
	assert.Equal(t, maliciousRepo, parsed.Repo)
	assert.Equal(t, "file", parsed.Type)
	require.Len(t, parsed.Or, 2)
	assert.Equal(t, maliciousPath, parsed.Or[0]["path"])
	assert.Equal(t, "a\x00\x7f\uFFFD", parsed.Or[1]["path"])
}

func TestPerformMultiPlatformImageSearchWithOnlyInvalidDigests(t *testing.T) {
	serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(1))
	fatManifestResult, _ := testFatManifest()
	fatManifest := &FatManifest{Manifests: []ManifestDetails{{Digest: "../../other-image"}, {Digest: "sha256:abc"}}}

	resultMap, err := performMultiPlatformImageSearch(fatManifestResult, fatManifest, serviceManager)
	require.NoError(t, err)
	assert.Empty(t, resultMap)
	assert.Empty(t, recorder.queries)
}

// The rows returned for a multi-platform image must not depend on the number of tags pushed under the image.
func TestMultiPlatformImageSearchRowsIndependentOfTagsCount(t *testing.T) {
	for _, tags := range []int{1, 100, 10000} {
		t.Run(fmt.Sprintf("tags=%d", tags), func(t *testing.T) {
			serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(tags))
			fatManifestResult, fatManifest := testFatManifest()
			_, err := performMultiPlatformImageSearch(fatManifestResult, fatManifest, serviceManager)
			require.NoError(t, err)
			// 2 platforms * (manifest.json + config + layer)
			assert.Equal(t, 6, recorder.returnedRows)
		})
	}
}

func TestSearchLayersForFatManifestUsesExactPaths(t *testing.T) {
	serviceManager, recorder := newFakeArtifactory(t, newFakeDockerRepo(100))
	fatManifestResult, _ := testFatManifest()

	layers, err := searchLayersForFatManifest(map[string]*utils.ResultItem{FatManifestJsonFile: fatManifestResult}, serviceManager, fatManifestTestRepo)
	require.NoError(t, err)

	require.Len(t, recorder.queries, 1)
	assert.NotContains(t, recorder.queries[0], "$match")
	assert.NotContains(t, recorder.queries[0], `property`)
	// The fat manifest + 2 platforms * (manifest.json + config + layer)
	require.Len(t, *layers, 7)
	assert.Equal(t, FatManifestJsonFile, (*layers)[0].Name)
	for _, layer := range (*layers)[1:] {
		assert.Contains(t, []string{
			fatManifestTestImage + "/sha256:" + platformDigestHex(fatManifestTestTag, "amd64"),
			fatManifestTestImage + "/sha256:" + platformDigestHex(fatManifestTestTag, "arm64"),
		}, layer.Path)
	}
}

// The build-info of a multi-platform image holds a module per platform manifest listed in the fat manifest,
// including attestation manifests, and build properties are set on exactly these files and the fat manifest.
func TestHandleFatManifestImageCreatesBuildInfo(t *testing.T) {
	repo := newFakeDockerRepo(50)
	attestationHex := platformDigestHex(fatManifestTestTag, "attestation")
	attestationFolder := fatManifestTestImage + "/sha256:" + attestationHex
	repo.addFile(attestationFolder, ManifestJsonFile, attestationHex)
	repo.addFile(attestationFolder, "sha256__attestationconfig", "attestationconfig")
	repo.addFile(attestationFolder, "sha256__attestationlayer", "attestationlayer")
	serviceManager, recorder := newFakeArtifactory(t, repo)
	fatManifestResult, fatManifest := testFatManifest()
	fatManifest.Manifests = append(fatManifest.Manifests, ManifestDetails{
		Digest:      "sha256:" + attestationHex,
		Platform:    Platform{Os: unknownPlatformPlaceholder, Architecture: unknownPlatformPlaceholder},
		Annotations: Annotations{ReferenceType: attestationManifestRefType},
	})
	recorder.fatManifest = fatManifest
	buildName, buildNumber := "rteco485-fat-manifest-test", "1"
	require.NoError(t, build.SaveBuildGeneralDetails(buildName, buildNumber, ""))
	t.Cleanup(func() { _ = build.RemoveBuildDir(buildName, buildNumber, "") }) // best-effort test cleanup

	builder := &RemoteAgentBuildInfoBuilder{buildInfoBuilder: &buildInfoBuilder{
		image:             NewImage("my.registry.io/" + fatManifestTestRepo + "/" + fatManifestTestImage + ":" + fatManifestTestTag),
		repositoryDetails: RepositoryDetails{key: fatManifestTestRepo, repoType: "local"},
		buildName:         buildName,
		buildNumber:       buildNumber,
		serviceManager:    serviceManager,
		workingDirectory:  t.TempDir(),
	}}
	multiPlatformImages, fatManifestDetails, downloadedFatManifest, err := builder.handleFatManifestImage(map[string]*utils.ResultItem{FatManifestJsonFile: fatManifestResult})
	require.NoError(t, err)
	buildInfo, err := builder.buildInfoBuilder.createMultiPlatformBuildInfo(downloadedFatManifest, fatManifestDetails, multiPlatformImages, "")
	require.NoError(t, err)

	moduleId := "img:" + fatManifestTestTag
	expectedModules := map[string]string{
		moduleId:                  "",
		"linux/amd64/" + moduleId: fatManifestTestImage + "/sha256:" + platformDigestHex(fatManifestTestTag, "amd64"),
		"linux/arm64/" + moduleId: fatManifestTestImage + "/sha256:" + platformDigestHex(fatManifestTestTag, "arm64"),
		AttestationsModuleIdPrefix + "/" + moduleId: attestationFolder,
	}
	require.Len(t, buildInfo.Modules, len(expectedModules))
	expectedPropsPaths := []string{path.Join(fatManifestTestRepo, fatManifestResult.Path, FatManifestJsonFile)}
	for _, module := range buildInfo.Modules {
		folder, ok := expectedModules[module.Id]
		require.True(t, ok, module.Id)
		if folder == "" {
			require.Len(t, module.Artifacts, 1)
			assert.Equal(t, FatManifestJsonFile, module.Artifacts[0].Name)
			continue
		}
		require.Len(t, module.Artifacts, 3, module.Id)
		for _, artifact := range module.Artifacts {
			assert.Equal(t, folder, path.Dir(artifact.Path))
			assert.Equal(t, fatManifestTestRepo, artifact.OriginalDeploymentRepo)
			assert.NotEmpty(t, artifact.Sha256)
			assert.Equal(t, "sha1-"+artifact.Sha256, artifact.Sha1)
			assert.Equal(t, "md5-"+artifact.Sha256, artifact.Md5)
			expectedPropsPaths = append(expectedPropsPaths, path.Join(fatManifestTestRepo, artifact.Path))
		}
	}
	sort.Strings(expectedPropsPaths)
	sort.Strings(recorder.propsPaths)
	assert.Equal(t, expectedPropsPaths, recorder.propsPaths)
	// A single query for the platform manifests, whatever the number of tags.
	require.Len(t, recorder.queries, 1)
}
