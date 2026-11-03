package helm

import (
	"testing"

	"github.com/jfrog/build-info-go/entities"
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

// Dependencies pushed to OCI with build metadata live under the same '_' tag folder.
func TestAddOCILayersForDependencyBuildMetadataVersion(t *testing.T) {
	dependency := entities.Dependency{Id: "common:1.0.0+meta", Repository: "oci://acme.jfrog.io/helm-local"}

	t.Run("collects the layers found under the underscore tag", func(t *testing.T) {
		fake := &fakeSearchManager{
			t:             t,
			existingPaths: map[string]bool{"common/1.0.0_meta": true},
			layerNames:    []string{"sha256__cfg", "sha256__tgz"},
			manifestJSON:  `{"config":{"digest":"sha256:cfg"},"layers":[{"digest":"sha256:tgz","mediaType":"application/vnd.cncf.helm.chart.content.v1.tar+gzip"}]}`,
		}
		var processed []entities.Dependency

		addOCILayersForDependency(dependency, fake, &processed)

		require.NotEmpty(t, fake.queries)
		assert.JSONEq(t, `{"repo":"helm-local","path":"common/1.0.0_meta"}`, fake.queries[0])
		var names []string
		for _, dep := range processed {
			names = append(names, dep.Id)
			assert.Equal(t, "helm-local", dep.Repository)
		}
		assert.ElementsMatch(t, []string{"manifest.json", "sha256__cfg", "sha256__tgz"}, names)
	})

	t.Run("never searches the plus form", func(t *testing.T) {
		fake := &fakeSearchManager{t: t, existingPaths: map[string]bool{}}
		var processed []entities.Dependency

		addOCILayersForDependency(dependency, fake, &processed)

		require.NotEmpty(t, fake.queries)
		for _, query := range fake.queries {
			assert.NotContains(t, query, "+")
		}
		assert.Empty(t, processed)
	})
}
