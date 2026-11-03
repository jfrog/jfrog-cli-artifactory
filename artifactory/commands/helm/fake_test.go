package helm

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/stretchr/testify/require"
)

// fakeSearchManager implements only SearchFiles. It records every AQL query and answers with
// a manifest.json item, plus one item per name in layerNames, only when the query targets one of the paths in
// existingPaths. ReadRemoteFile serves manifestJSON.
type fakeSearchManager struct {
	artifactory.ArtifactoryServicesManager
	t             *testing.T
	existingPaths map[string]bool
	layerNames    []string
	manifestJSON  string
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
		for _, name := range append([]string{"manifest.json"}, f.layerNames...) {
			results = append(results, map[string]string{
				"repo": parsed.Repo, "path": parsed.Path, "name": name, "type": "file", "sha256": name,
			})
		}
	}
	data, err := json.Marshal(map[string]any{"results": results})
	require.NoError(f.t, err)
	file := filepath.Join(f.t.TempDir(), "results.json")
	require.NoError(f.t, os.WriteFile(file, data, 0600))
	return content.NewContentReader(file, "results"), nil
}

func (f *fakeSearchManager) ReadRemoteFile(string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.manifestJSON)), nil
}
