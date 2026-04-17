package helm

import (
	"fmt"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseDependencyID tests the parseDependencyID function
func TestParseDependencyID(t *testing.T) {
	tests := []struct {
		name          string
		depId         string
		expectedName  string
		expectedVer   string
		expectedError bool
	}{
		{
			name:          "Valid dependency ID",
			depId:         "nginx:1.2.3",
			expectedName:  "nginx",
			expectedVer:   "1.2.3",
			expectedError: false,
		},
		{
			name:          "Invalid - no colon",
			depId:         "nginx",
			expectedError: true,
		},
		{
			name:          "Invalid - multiple colons",
			depId:         "nginx:1.2.3:extra",
			expectedError: true,
		},
		{
			name:          "Empty string",
			depId:         "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ver, err := parseDependencyID(tt.depId)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedName, name)
				assert.Equal(t, tt.expectedVer, ver)
			}
		})
	}
}

// TestExtractDependencyPathInLayers tests the extractDependencyPath function (from repository.go but used in layers)
// Note: This test is also in repository_test.go, keeping this version for layers.go specific testing
func TestExtractDependencyPathInLayers(t *testing.T) {
	tests := []struct {
		name         string
		depId        string
		expectedPath string
	}{
		{
			name:         "Valid dependency ID",
			depId:        "nginx:1.2.3",
			expectedPath: "nginx/1.2.3",
		},
		{
			name:         "Invalid format - no colon",
			depId:        "nginx",
			expectedPath: "",
		},
		{
			name:         "Invalid format - multiple colons",
			depId:        "nginx:1.2.3:extra",
			expectedPath: "",
		},
		{
			name:         "Empty string",
			depId:        "",
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDependencyPath(tt.depId)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

func TestAddOCILayersForDependency(t *testing.T) {
	const (
		chartName    = "chart"
		chartVersion = "0.1.0"
	)

	tests := []struct {
		name                 string
		dependency           entities.Dependency
		responses            map[pushSearchCall][]servicesUtils.ResultItem
		expectedSearchCalls  []pushSearchCall
		expectedDependencies []entities.Dependency
		manifestRepo         string
		manifestPath         string
	}{
		{
			name: "single-segment virtual host subpath resolves using host fallback",
			dependency: entities.Dependency{
				Id:         chartName + ":" + chartVersion,
				Repository: "oci://helm-repo.art.com/team-a",
			},
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "team-a", path: "chart/0.1.0"}: nil,
				{repo: "helm-repo", path: "team-a/chart/0.1.0"}: {
					newOCIArtifact("helm-repo", "team-a/chart/0.1.0", "manifest.json", "manifest-sha"),
					newOCIArtifact("helm-repo", "team-a/chart/0.1.0", "sha256__config", "config-sha"),
					newOCIArtifact("helm-repo", "team-a/chart/0.1.0", "sha256__layer", "layer-sha"),
				},
			},
			expectedSearchCalls: []pushSearchCall{
				{repo: "team-a", path: "chart/0.1.0"},
				{repo: "helm-repo", path: "team-a/chart/0.1.0"},
			},
			expectedDependencies: []entities.Dependency{
				newProcessedLayerDependency("manifest.json", "helm-repo", "manifest-sha"),
				newProcessedLayerDependency("sha256__config", "helm-repo", "config-sha"),
				newProcessedLayerDependency("sha256__layer", "helm-repo", "layer-sha"),
			},
			manifestRepo: "helm-repo",
			manifestPath: "team-a/chart/0.1.0",
		},
		{
			name: "oci dependency with non-root subpath resolves using validated candidate",
			dependency: entities.Dependency{
				Id:         chartName + ":" + chartVersion,
				Repository: "oci://helm-repo.art.com/team-a/charts",
			},
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "team-a", path: "charts/chart/0.1.0"}: nil,
				{repo: "helm-repo", path: "team-a/charts/chart/0.1.0"}: {
					newOCIArtifact("helm-repo", "team-a/charts/chart/0.1.0", "manifest.json", "manifest-sha"),
					newOCIArtifact("helm-repo", "team-a/charts/chart/0.1.0", "sha256__config", "config-sha"),
					newOCIArtifact("helm-repo", "team-a/charts/chart/0.1.0", "sha256__layer", "layer-sha"),
				},
			},
			expectedSearchCalls: []pushSearchCall{
				{repo: "team-a", path: "charts/chart/0.1.0"},
				{repo: "helm-repo", path: "team-a/charts/chart/0.1.0"},
			},
			expectedDependencies: []entities.Dependency{
				newProcessedLayerDependency("manifest.json", "helm-repo", "manifest-sha"),
				newProcessedLayerDependency("sha256__config", "helm-repo", "config-sha"),
				newProcessedLayerDependency("sha256__layer", "helm-repo", "layer-sha"),
			},
			manifestRepo: "helm-repo",
			manifestPath: "team-a/charts/chart/0.1.0",
		},
		{
			name: "root-only oci dependency keeps existing resolution",
			dependency: entities.Dependency{
				Id:         chartName + ":" + chartVersion,
				Repository: "oci://helm-repo.art.com",
			},
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "helm-repo", path: "chart/0.1.0"}: {
					newOCIArtifact("helm-repo", "chart/0.1.0", "manifest.json", "manifest-sha"),
					newOCIArtifact("helm-repo", "chart/0.1.0", "sha256__config", "config-sha"),
					newOCIArtifact("helm-repo", "chart/0.1.0", "sha256__layer", "layer-sha"),
				},
			},
			expectedSearchCalls: []pushSearchCall{{repo: "helm-repo", path: "chart/0.1.0"}},
			expectedDependencies: []entities.Dependency{
				newProcessedLayerDependency("manifest.json", "helm-repo", "manifest-sha"),
				newProcessedLayerDependency("sha256__config", "helm-repo", "config-sha"),
				newProcessedLayerDependency("sha256__layer", "helm-repo", "layer-sha"),
			},
			manifestRepo: "helm-repo",
			manifestPath: "chart/0.1.0",
		},
		{
			name: "oci dependency without matching candidate returns without adding layers",
			dependency: entities.Dependency{
				Id:         chartName + ":" + chartVersion,
				Repository: "oci://helm-repo.art.com/team-a/charts",
			},
			responses: map[pushSearchCall][]servicesUtils.ResultItem{
				{repo: "team-a", path: "charts/chart/0.1.0"}:           nil,
				{repo: "helm-repo", path: "team-a/charts/chart/0.1.0"}: nil,
			},
			expectedSearchCalls: []pushSearchCall{
				{repo: "team-a", path: "charts/chart/0.1.0"},
				{repo: "helm-repo", path: "team-a/charts/chart/0.1.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceManager := newPushTestServiceManager(t)
			serviceManager.searchResults = tt.responses
			if tt.manifestRepo != "" {
				serviceManager.remoteContents[fmt.Sprintf("%s/%s/manifest.json", tt.manifestRepo, tt.manifestPath)] = createPushManifestJSON("sha256:config", "sha256:layer")
			}

			var processed []entities.Dependency
			addOCILayersForDependency(tt.dependency, serviceManager, &processed)

			assert.Equal(t, tt.expectedSearchCalls, serviceManager.searchCalls)
			assert.Equal(t, tt.expectedDependencies, processed)
		})
	}
}

func TestUpdateClassicHelmDependencyChecksumsLeavesExistingChecksumsUntouched(t *testing.T) {
	serviceManager := newPushTestServiceManager(t)
	dep := entities.Dependency{
		Id:         "classic:1.2.3",
		Repository: "https://art.company.com/helm-local",
		Checksum: entities.Checksum{
			Md5:    "md5",
			Sha1:   "sha1",
			Sha256: "sha256",
		},
	}

	var processed []entities.Dependency
	updateClassicHelmDependencyChecksums(dep, serviceManager, &processed)

	require.Len(t, processed, 1)
	assert.Equal(t, dep, processed[0])
	assert.Empty(t, serviceManager.searchCalls)
}

func newProcessedLayerDependency(name, repo, sha256 string) entities.Dependency {
	return entities.Dependency{
		Id:         name,
		Repository: repo,
		Checksum: entities.Checksum{
			Sha256: sha256,
		},
	}
}
