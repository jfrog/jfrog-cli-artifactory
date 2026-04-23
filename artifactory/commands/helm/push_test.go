package helm

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pushSearchCall struct {
	repo string
	path string
}

type pushPropsCall struct {
	props string
	item  servicesUtils.ResultItem
}

type pushTestServiceManager struct {
	artifactory.EmptyArtifactoryServicesManager
	t              *testing.T
	searchResults  map[pushSearchCall][]servicesUtils.ResultItem
	searchCalls    []pushSearchCall
	propsCalls     []pushPropsCall
	propsErr       error
	remoteContents map[string]string
}

func newPushTestServiceManager(t *testing.T) *pushTestServiceManager {
	return &pushTestServiceManager{
		t:              t,
		searchResults:  map[pushSearchCall][]servicesUtils.ResultItem{},
		remoteContents: map[string]string{},
	}
}

func (m *pushTestServiceManager) SearchFiles(params services.SearchParams) (*content.ContentReader, error) {
	m.t.Helper()
	call := parsePushSearchCall(m.t, params.CommonParams.Aql.ItemsFind)
	m.searchCalls = append(m.searchCalls, call)
	return newPushSearchReader(m.t, m.searchResults[call]), nil
}

func (m *pushTestServiceManager) SetProps(params services.PropsParams) (int, error) {
	m.t.Helper()
	item := new(servicesUtils.ResultItem)
	require.NoError(m.t, params.Reader.NextRecord(item))
	m.propsCalls = append(m.propsCalls, pushPropsCall{props: params.Props, item: *item})
	if m.propsErr != nil {
		return 0, m.propsErr
	}
	return 1, nil
}

func (m *pushTestServiceManager) ReadRemoteFile(path string) (io.ReadCloser, error) {
	body, ok := m.remoteContents[path]
	if !ok {
		return nil, fmt.Errorf("unexpected remote path: %s", path)
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

func parsePushSearchCall(t *testing.T, aql string) pushSearchCall {
	t.Helper()
	var query struct {
		Repo string `json:"repo"`
		Path string `json:"path"`
	}
	require.NoError(t, json.Unmarshal([]byte(aql), &query))
	return pushSearchCall{repo: query.Repo, path: query.Path}
}

func newPushSearchReader(t *testing.T, items []servicesUtils.ResultItem) *content.ContentReader {
	t.Helper()
	tmpFile, err := os.CreateTemp(t.TempDir(), "push-search-*.json")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, tmpFile.Close())
	}()
	payload := map[string]any{"results": items}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = tmpFile.Write(data)
	require.NoError(t, err)
	return content.NewContentReader(tmpFile.Name(), content.DefaultKey)
}

func newOCIArtifact(repo, storagePath, name, sha256 string) servicesUtils.ResultItem {
	return servicesUtils.ResultItem{
		Repo:   repo,
		Path:   storagePath,
		Name:   name,
		Type:   "file",
		Sha256: sha256,
	}
}

func createChartArchive(t *testing.T, chartName, chartVersion string) string {
	t.Helper()
	chartPath := filepath.Join(t.TempDir(), fmt.Sprintf("%s-%s.tgz", chartName, chartVersion))
	file, err := os.Create(chartPath)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, file.Close())
	}()
	gzWriter := gzip.NewWriter(file)
	defer func() {
		require.NoError(t, gzWriter.Close())
	}()
	tarWriter := tar.NewWriter(gzWriter)
	defer func() {
		require.NoError(t, tarWriter.Close())
	}()
	chartYAML := fmt.Sprintf("apiVersion: v2\nname: %s\nversion: %s\n", chartName, chartVersion)
	header := &tar.Header{
		Name: fmt.Sprintf("%s/Chart.yaml", chartName),
		Mode: 0o600,
		Size: int64(len(chartYAML)),
	}
	require.NoError(t, tarWriter.WriteHeader(header))
	_, err = tarWriter.Write([]byte(chartYAML))
	require.NoError(t, err)
	return chartPath
}

func createPushManifestJSON(configDigest, layerDigest string) string {
	return fmt.Sprintf(`{"config":{"digest":"%s"},"layers":[{"digest":"%s","mediaType":"application/vnd.oci.image.layer.v1.tar+gzip"}]}`,
		configDigest, layerDigest)
}

func TestResolveOCIPushArtifacts(t *testing.T) {
	const (
		chartName    = "chart"
		chartVersion = "0.1.0"
	)
	chartStoragePath := chartName + "/" + chartVersion

	tests := []struct {
		name              string
		registryURL       string
		responses         map[pushSearchCall][]servicesUtils.ResultItem
		expectedRepoKey   string
		expectedSubpath   string
		expectedPath      string
		expectedCalls     []pushSearchCall
		expectedErrorText string
	}{
		{
			name:        "form 1 without path resolves host repo locally",
			registryURL: "oci://helm-repo.art.com",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: chartStoragePath}: {newOCIArtifact("helm-repo", chartStoragePath, "manifest.json", "manifest")},
			},
			expectedRepoKey: "helm-repo",
			expectedSubpath: "",
			expectedPath:    chartStoragePath,
			expectedCalls:   []pushSearchCall{{repo: "helm-repo", path: chartStoragePath}},
		},
		{
			name:        "form 2 virtual host with subpath resolves on first host-based candidate",
			registryURL: "oci://helm-repo.art.com/team-a/charts",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: "team-a/charts/" + chartStoragePath}: {newOCIArtifact("helm-repo", "team-a/charts/"+chartStoragePath, "manifest.json", "manifest")},
				{repo: "team-a", path: "charts/" + chartStoragePath}:           nil,
			},
			expectedRepoKey: "helm-repo",
			expectedSubpath: "team-a/charts",
			expectedPath:    "team-a/charts/" + chartStoragePath,
			expectedCalls: []pushSearchCall{
				{repo: "helm-repo", path: "team-a/charts/" + chartStoragePath},
			},
		},
		{
			name:        "host-only URL with trailing slash is normalized",
			registryURL: "oci://helm-repo.art.com/",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: chartStoragePath}: {newOCIArtifact("helm-repo", chartStoragePath, "manifest.json", "manifest")},
			},
			expectedRepoKey: "helm-repo",
			expectedSubpath: "",
			expectedPath:    chartStoragePath,
			expectedCalls:   []pushSearchCall{{repo: "helm-repo", path: chartStoragePath}},
		},
		{
			name:        "form 3 repo in path without extra subpath falls back after host-first miss",
			registryURL: "oci://art.company.com/helm-repo",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "art", path: "helm-repo/" + chartStoragePath}: nil,
				{repo: "helm-repo", path: chartStoragePath}:          {newOCIArtifact("helm-repo", chartStoragePath, "manifest.json", "manifest")},
			},
			expectedRepoKey: "helm-repo",
			expectedSubpath: "",
			expectedPath:    chartStoragePath,
			expectedCalls: []pushSearchCall{
				{repo: "art", path: "helm-repo/" + chartStoragePath},
				{repo: "helm-repo", path: chartStoragePath},
			},
		},
		{
			name:        "single-segment virtual host subpath resolves on first host-based candidate",
			registryURL: "oci://helm-repo.art.com/team-a",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: "team-a/" + chartStoragePath}: {newOCIArtifact("helm-repo", "team-a/"+chartStoragePath, "manifest.json", "manifest")},
				{repo: "team-a", path: chartStoragePath}:                nil,
			},
			expectedRepoKey: "helm-repo",
			expectedSubpath: "team-a",
			expectedPath:    "team-a/" + chartStoragePath,
			expectedCalls: []pushSearchCall{
				{repo: "helm-repo", path: "team-a/" + chartStoragePath},
			},
		},
		{
			name:        "form 4 repo in path with extra subpath falls back after host-first miss",
			registryURL: "oci://art.company.com/helm-repo/staging/libs",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "art", path: "helm-repo/staging/libs/" + chartStoragePath}: nil,
				{repo: "helm-repo", path: "staging/libs/" + chartStoragePath}:     {newOCIArtifact("helm-repo", "staging/libs/"+chartStoragePath, "manifest.json", "manifest")},
			},
			expectedRepoKey: "helm-repo",
			expectedSubpath: "staging/libs",
			expectedPath:    "staging/libs/" + chartStoragePath,
			expectedCalls: []pushSearchCall{
				{repo: "art", path: "helm-repo/staging/libs/" + chartStoragePath},
				{repo: "helm-repo", path: "staging/libs/" + chartStoragePath},
			},
		},
		{
			name:        "tries path fallback when host-first candidate has no artifact",
			registryURL: "oci://helm-repo.art.com/folder/subfolder",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: "folder/subfolder/" + chartStoragePath}: {newOCIArtifact("helm-repo", "folder/subfolder/"+chartStoragePath, "manifest.json", "manifest")},
				{repo: "folder", path: "subfolder/" + chartStoragePath}:           nil,
			},
			expectedRepoKey: "helm-repo",
			expectedSubpath: "folder/subfolder",
			expectedPath:    "folder/subfolder/" + chartStoragePath,
			expectedCalls: []pushSearchCall{
				{repo: "helm-repo", path: "folder/subfolder/" + chartStoragePath},
			},
		},
		{
			name:        "returns host-based match first on plausible multi-label host",
			registryURL: "oci://helm-prod.company.example/team-a/charts",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-prod", path: "team-a/charts/" + chartStoragePath}: {newOCIArtifact("helm-prod", "team-a/charts/"+chartStoragePath, "manifest.json", "manifest")},
				{repo: "team-a", path: "charts/" + chartStoragePath}:           {newOCIArtifact("team-a", "charts/"+chartStoragePath, "manifest.json", "wrong")},
			},
			expectedRepoKey: "helm-prod",
			expectedSubpath: "team-a/charts",
			expectedPath:    "team-a/charts/" + chartStoragePath,
			expectedCalls:   []pushSearchCall{{repo: "helm-prod", path: "team-a/charts/" + chartStoragePath}},
		},
		{
			name:        "returns unresolved error when both host-first and path fallback miss",
			registryURL: "oci://helm-repo.art.com/team-a/charts",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: "team-a/charts/" + chartStoragePath}: nil,
				{repo: "team-a", path: "charts/" + chartStoragePath}:           nil,
			},
			expectedErrorText: "could not resolve OCI push repository key",
			expectedCalls: []pushSearchCall{
				{repo: "helm-repo", path: "team-a/charts/" + chartStoragePath},
				{repo: "team-a", path: "charts/" + chartStoragePath},
			},
		},
		{
			name:        "tries host-based candidate without dash in repo key first",
			registryURL: "oci://helmrepo.company.example/team-a/charts",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helmrepo", path: "team-a/charts/" + chartStoragePath}: {newOCIArtifact("helmrepo", "team-a/charts/"+chartStoragePath, "manifest.json", "manifest")},
				{repo: "team-a", path: "charts/" + chartStoragePath}:          nil,
			},
			expectedRepoKey: "helmrepo",
			expectedSubpath: "team-a/charts",
			expectedPath:    "team-a/charts/" + chartStoragePath,
			expectedCalls: []pushSearchCall{
				{repo: "helmrepo", path: "team-a/charts/" + chartStoragePath},
			},
		},
		{
			name:              "returns unresolved error when neither host-first nor path fallback matches",
			registryURL:       "oci://art.company.com/helm-repo/team-a",
			responses:         map[pushSearchCall][]servicesUtils.ResultItem{},
			expectedErrorText: "could not resolve OCI push repository key",
			expectedCalls: []pushSearchCall{
				{repo: "art", path: "helm-repo/team-a/" + chartStoragePath},
				{repo: "helm-repo", path: "team-a/" + chartStoragePath},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceManager := newPushTestServiceManager(t)
			serviceManager.searchResults = tt.responses

			repoKey, subpath, storagePath, resultMap, err := resolveOCIPushArtifacts(tt.registryURL, chartName, chartVersion, serviceManager)

			if tt.expectedErrorText != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.expectedErrorText)
				assert.Nil(t, resultMap)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedRepoKey, repoKey)
				assert.Equal(t, tt.expectedSubpath, subpath)
				assert.Equal(t, tt.expectedPath, storagePath)
				assert.NotEmpty(t, resultMap)
			}
			assert.Equal(t, tt.expectedCalls, serviceManager.searchCalls)
		})
	}
}

func TestSearchPushedArtifactsUsesResolvedStoragePath(t *testing.T) {
	serviceManager := newPushTestServiceManager(t)
	serviceManager.searchResults[pushSearchCall{repo: "helm-repo", path: "team-a/charts/chart/0.1.0"}] = []servicesUtils.ResultItem{
		newOCIArtifact("helm-repo", "team-a/charts/chart/0.1.0", "manifest.json", "manifest"),
	}

	resultMap, err := searchPushedArtifacts(serviceManager, "helm-repo", "team-a/charts/chart/0.1.0")
	require.NoError(t, err)
	assert.Len(t, resultMap, 1)
	assert.Equal(t, []pushSearchCall{{repo: "helm-repo", path: "team-a/charts/chart/0.1.0"}}, serviceManager.searchCalls)
}

func TestNewManifestFolderReader(t *testing.T) {
	reader, cleanup, err := newManifestFolderReader("helm-repo", "team-a/charts/chart", "0.1.0")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cleanup())
	}()

	item := new(servicesUtils.ResultItem)
	require.NoError(t, reader.NextRecord(item))
	assert.Equal(t, "helm-repo", item.Repo)
	assert.Equal(t, "team-a/charts/chart", item.Path)
	assert.Equal(t, "0.1.0", item.Name)
	assert.Equal(t, "folder", item.Type)
}

func TestApplyBuildPropertiesOnManifestFolderReturnsSetPropsError(t *testing.T) {
	serviceManager := newPushTestServiceManager(t)
	serviceManager.propsErr = fmt.Errorf("set props failed")

	err := applyBuildPropertiesOnManifestFolder(serviceManager, "helm-repo", "team-a/charts/chart", "0.1.0", "build.name=test")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to set build properties on artifacts")
	assert.ErrorContains(t, err, "set props failed")
	require.Len(t, serviceManager.propsCalls, 1)
	assert.Equal(t, "helm-repo", serviceManager.propsCalls[0].item.Repo)
	assert.Equal(t, "team-a/charts/chart", serviceManager.propsCalls[0].item.Path)
	assert.Equal(t, "0.1.0", serviceManager.propsCalls[0].item.Name)
}

func TestOverwriteReaderWithManifestFolderUsesRealArtifactoryPath(t *testing.T) {
	reader := newPushSearchReader(t, []servicesUtils.ResultItem{
		newOCIArtifact("old-repo", "ignored", "manifest.json", "manifest"),
	})

	require.NoError(t, overwriteReaderWithManifestFolder(reader, "helm-repo", "team-a/charts/chart", "0.1.0"))
	reader.Reset()
	item := new(servicesUtils.ResultItem)
	require.NoError(t, reader.NextRecord(item))
	assert.Equal(t, "helm-repo", item.Repo)
	assert.Equal(t, "team-a/charts/chart", item.Path)
	assert.Equal(t, "0.1.0", item.Name)
	assert.Equal(t, "folder", item.Type)
}

func TestHandlePushCommandResolvesOCIPaths(t *testing.T) {
	tests := []struct {
		name              string
		registryURL       string
		responses         map[pushSearchCall][]servicesUtils.ResultItem
		expectedSearches  []pushSearchCall
		expectedManifest  string
		expectedPropsRepo string
		expectedPropsPath string
		expectedPropsName string
	}{
		{
			name:        "form 1 keeps root-only flow without extra disambiguation",
			registryURL: "oci://helm-repo.art.com",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: "chart/0.1.0"}: {
					newOCIArtifact("helm-repo", "chart/0.1.0", "manifest.json", "manifest-sha"),
					newOCIArtifact("helm-repo", "chart/0.1.0", "sha256__config", "config-sha"),
					newOCIArtifact("helm-repo", "chart/0.1.0", "sha256__layer", "layer-sha"),
				},
			},
			expectedSearches:  []pushSearchCall{{repo: "helm-repo", path: "chart/0.1.0"}},
			expectedManifest:  "helm-repo/chart/0.1.0/manifest.json",
			expectedPropsRepo: "helm-repo",
			expectedPropsPath: "chart",
			expectedPropsName: "0.1.0",
		},
		{
			name:        "host-first push keeps real manifest folder when the host candidate resolves",
			registryURL: "oci://helm-repo.art.com/team-a/charts",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: "team-a/charts/chart/0.1.0"}: {
					newOCIArtifact("helm-repo", "team-a/charts/chart/0.1.0", "manifest.json", "manifest-sha"),
					newOCIArtifact("helm-repo", "team-a/charts/chart/0.1.0", "sha256__config", "config-sha"),
					newOCIArtifact("helm-repo", "team-a/charts/chart/0.1.0", "sha256__layer", "layer-sha"),
				},
				{repo: "team-a", path: "charts/chart/0.1.0"}: nil,
			},
			expectedSearches: []pushSearchCall{
				{repo: "helm-repo", path: "team-a/charts/chart/0.1.0"},
			},
			expectedManifest:  "helm-repo/team-a/charts/chart/0.1.0/manifest.json",
			expectedPropsRepo: "helm-repo",
			expectedPropsPath: "team-a/charts/chart",
			expectedPropsName: "0.1.0",
		},
		{
			name:        "form 3 path-based repo keeps chart root paths after host-first miss",
			registryURL: "oci://art.company.com/helm-repo",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "art", path: "helm-repo/chart/0.1.0"}: nil,
				{repo: "helm-repo", path: "chart/0.1.0"}: {
					newOCIArtifact("helm-repo", "chart/0.1.0", "manifest.json", "manifest-sha"),
					newOCIArtifact("helm-repo", "chart/0.1.0", "sha256__config", "config-sha"),
					newOCIArtifact("helm-repo", "chart/0.1.0", "sha256__layer", "layer-sha"),
				},
			},
			expectedSearches: []pushSearchCall{
				{repo: "art", path: "helm-repo/chart/0.1.0"},
				{repo: "helm-repo", path: "chart/0.1.0"},
			},
			expectedManifest:  "helm-repo/chart/0.1.0/manifest.json",
			expectedPropsRepo: "helm-repo",
			expectedPropsPath: "chart",
			expectedPropsName: "0.1.0",
		},
		{
			name:        "form 4 path-based repo uses resolved manifest folder after host-first miss",
			registryURL: "oci://art.company.com/helm-repo/staging/libs",
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "art", path: "helm-repo/staging/libs/chart/0.1.0"}: nil,
				{repo: "helm-repo", path: "staging/libs/chart/0.1.0"}: {
					newOCIArtifact("helm-repo", "staging/libs/chart/0.1.0", "manifest.json", "manifest-sha"),
					newOCIArtifact("helm-repo", "staging/libs/chart/0.1.0", "sha256__config", "config-sha"),
					newOCIArtifact("helm-repo", "staging/libs/chart/0.1.0", "sha256__layer", "layer-sha"),
				},
			},
			expectedSearches: []pushSearchCall{
				{repo: "art", path: "helm-repo/staging/libs/chart/0.1.0"},
				{repo: "helm-repo", path: "staging/libs/chart/0.1.0"},
			},
			expectedManifest:  "helm-repo/staging/libs/chart/0.1.0/manifest.json",
			expectedPropsRepo: "helm-repo",
			expectedPropsPath: "staging/libs/chart",
			expectedPropsName: "0.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JFROG_CLI_HOME_DIR", t.TempDir())
			serviceManager := newPushTestServiceManager(t)
			serviceManager.searchResults = tt.responses
			serviceManager.remoteContents[tt.expectedManifest] = createPushManifestJSON("sha256:config", "sha256:layer")

			buildInfo := &entities.BuildInfo{
				Modules:    []entities.Module{{Id: "chart:0.1.0", Type: "helm"}},
				BuildAgent: &entities.Agent{Name: "Helm", Version: "test"},
			}
			chartPath := createChartArchive(t, "chart", "0.1.0")

			err := handlePushCommand(buildInfo, []string{chartPath, tt.registryURL}, serviceManager, "build-name", "42", "proj")
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSearches, serviceManager.searchCalls)
			require.Len(t, serviceManager.propsCalls, 1)
			assert.Equal(t, tt.expectedPropsRepo, serviceManager.propsCalls[0].item.Repo)
			assert.Equal(t, tt.expectedPropsPath, serviceManager.propsCalls[0].item.Path)
			assert.Equal(t, tt.expectedPropsName, serviceManager.propsCalls[0].item.Name)
			assert.Contains(t, serviceManager.propsCalls[0].props, "build.name=build-name")
			assert.Contains(t, serviceManager.propsCalls[0].props, "build.number=42")
			assert.Contains(t, serviceManager.propsCalls[0].props, "build.project=proj")
			require.Len(t, buildInfo.Modules, 1)
			assert.Len(t, buildInfo.Modules[0].Artifacts, 3)
		})
	}
}

func TestHandlePushCommandReturnsBuildPropsError(t *testing.T) {
	t.Setenv("JFROG_CLI_HOME_DIR", t.TempDir())
	serviceManager := newPushTestServiceManager(t)
	serviceManager.propsErr = fmt.Errorf("set props failed")
	serviceManager.searchResults[pushSearchCall{repo: "helm-repo", path: "chart/0.1.0"}] = []servicesUtils.ResultItem{
		newOCIArtifact("helm-repo", "chart/0.1.0", "manifest.json", "manifest-sha"),
		newOCIArtifact("helm-repo", "chart/0.1.0", "sha256__config", "config-sha"),
		newOCIArtifact("helm-repo", "chart/0.1.0", "sha256__layer", "layer-sha"),
	}
	chartPath := createChartArchive(t, "chart", "0.1.0")
	buildInfo := &entities.BuildInfo{
		Modules:    []entities.Module{{Id: "chart:0.1.0", Type: "helm"}},
		BuildAgent: &entities.Agent{Name: "Helm", Version: "test"},
	}

	err := handlePushCommand(buildInfo, []string{chartPath, "oci://helm-repo.art.com"}, serviceManager, "build-name", "42", "proj")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to apply build properties on OCI manifest folder")
	assert.ErrorContains(t, err, "failed to set build properties on artifacts")
	assert.ErrorContains(t, err, "set props failed")
	require.Len(t, serviceManager.propsCalls, 1)
	assert.Empty(t, buildInfo.Modules[0].Artifacts)
}
