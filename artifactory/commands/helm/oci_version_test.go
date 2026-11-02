package helm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSearchManager implements only SearchFiles. It records every AQL query and answers with
// a manifest.json item only when the query targets one of the paths in existingPaths.
type fakeSearchManager struct {
	artifactory.ArtifactoryServicesManager
	t             *testing.T
	existingPaths map[string]bool
	queries       []string
}

type aqlPathQuery struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
}

func (f *fakeSearchManager) SearchFiles(params services.SearchParams) (*content.ContentReader, error) {
	query := params.Aql.ItemsFind
	f.queries = append(f.queries, query)
	var parsed aqlPathQuery
	require.NoError(f.t, json.Unmarshal([]byte(query), &parsed))

	results := []map[string]string{}
	if f.existingPaths[parsed.Path] {
		results = append(results, map[string]string{
			"repo": parsed.Repo, "path": parsed.Path, "name": "manifest.json", "type": "file",
		})
	}
	data, err := json.Marshal(map[string]any{"results": results})
	require.NoError(f.t, err)
	file := filepath.Join(f.t.TempDir(), "results.json")
	require.NoError(f.t, os.WriteFile(file, data, 0600))
	return content.NewContentReader(file, "results"), nil
}

// RTECO-2742: Helm pushes chart version X.Y.Z+meta under the OCI tag X.Y.Z_meta,
// so the Artifactory lookup must search the '_' folder.
func TestResolveOCIPushArtifactsBuildMetadataVersion(t *testing.T) {
	t.Run("finds the artifacts under the underscore tag", func(t *testing.T) {
		fake := &fakeSearchManager{t: t, existingPaths: map[string]bool{"mychart/1.2.3_abc1234": true}}

		repo, chartPath, results, err := resolveOCIPushArtifacts(fake, "oci://acme.jfrog.io/helm-local", "mychart", "1.2.3+abc1234")

		require.NoError(t, err)
		assert.Equal(t, "helm-local", repo)
		assert.Equal(t, "mychart/1.2.3_abc1234", chartPath)
		assert.Contains(t, results, "manifest.json")
	})

	t.Run("subpath", func(t *testing.T) {
		fake := &fakeSearchManager{t: t, existingPaths: map[string]bool{"team/app/mychart/1.2.3_abc1234": true}}

		repo, chartPath, _, err := resolveOCIPushArtifacts(fake, "oci://acme.jfrog.io/helm-local/team/app", "mychart", "1.2.3+abc1234")

		require.NoError(t, err)
		assert.Equal(t, "helm-local", repo)
		assert.Equal(t, "team/app/mychart/1.2.3_abc1234", chartPath)
	})

	t.Run("the plus form is not searched", func(t *testing.T) {
		fake := &fakeSearchManager{t: t, existingPaths: map[string]bool{}}

		_, _, _, err := resolveOCIPushArtifacts(fake, "oci://acme.jfrog.io/helm-local", "mychart", "1.2.3+abc1234")

		assert.ErrorContains(t, err, "no OCI artifacts found for chart mychart:1.2.3+abc1234")
		require.NotEmpty(t, fake.queries)
		for _, query := range fake.queries {
			assert.NotContains(t, query, "+")
		}
	})
}

// Dependencies pushed to OCI with build metadata live under the same '_' tag folder.
func TestAddOCILayersForDependencyBuildMetadataVersion(t *testing.T) {
	fake := &fakeSearchManager{t: t, existingPaths: map[string]bool{}}
	var processed []entities.Dependency

	addOCILayersForDependency(entities.Dependency{Id: "common:1.0.0+meta", Repository: "oci://acme.jfrog.io/helm-local"}, fake, &processed)

	require.NotEmpty(t, fake.queries)
	assert.JSONEq(t, `{"repo":"helm-local","path":"common/1.0.0_meta"}`, fake.queries[0])
	for _, query := range fake.queries {
		assert.Contains(t, query, "1.0.0_meta")
		assert.NotContains(t, query, "+")
	}
	assert.Empty(t, processed)
}
