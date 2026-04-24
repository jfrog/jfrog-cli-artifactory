package cli

import (
	"encoding/json"
	"os"
	"testing"

	coreformat "github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createSearchContentReader writes SearchResult-compatible JSON to a temp file and
// returns a *content.ContentReader backed by that file.
func createSearchContentReader(t *testing.T, items []map[string]interface{}) *content.ContentReader {
	t.Helper()
	type resultFile struct {
		Results []map[string]interface{} `json:"results"`
	}
	rf := resultFile{Results: items}
	data, err := json.Marshal(rf)
	require.NoError(t, err)

	f, err := os.CreateTemp("", "search-results-*.json")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	return content.NewContentReader(f.Name(), content.DefaultKey)
}

// newTestContext constructs a minimal *components.Context with the given string flags set.
func newTestContext(flags map[string]string) *components.Context {
	ctx := &components.Context{}
	for k, v := range flags {
		ctx.AddStringFlag(k, v)
	}
	ctx.PrintCommandHelp = func(string) error { return nil }
	return ctx
}

// ---------------------------------------------------------------------------
// getSearchOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetSearchOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getSearchOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format, "default format should be json (backward-compatible)")
}

func TestGetSearchOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getSearchOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetSearchOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getSearchOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetSearchOutputFormat_CaseInsensitive(t *testing.T) {
	for _, val := range []string{"JSON", "Json", "TABLE", "Table"} {
		t.Run(val, func(t *testing.T) {
			ctx := newTestContext(map[string]string{"format": val})
			_, err := getSearchOutputFormat(ctx)
			require.NoError(t, err)
		})
	}
}

func TestGetSearchOutputFormat_UnsupportedFormat_Sarif(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "sarif"})
	_, err := getSearchOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

func TestGetSearchOutputFormat_UnsupportedFormat_SimpleJson(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "simple-json"})
	_, err := getSearchOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

func TestGetSearchOutputFormat_UnsupportedFormat_XML(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getSearchOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printSearchResponse tests
// ---------------------------------------------------------------------------

func TestPrintSearchResponse_JSON(t *testing.T) {
	items := []map[string]interface{}{
		{"path": "repo/path/file.txt", "type": "file", "size": 1234, "sha256": "abc123"},
	}
	reader := createSearchContentReader(t, items)
	defer reader.Close()

	err := printSearchResponse(reader, coreformat.Json)
	assert.NoError(t, err)
}

func TestPrintSearchResponse_Table(t *testing.T) {
	items := []map[string]interface{}{
		{"path": "repo/path/file.jar", "type": "file", "size": 5678, "sha256": "deadbeef"},
	}
	reader := createSearchContentReader(t, items)
	defer reader.Close()

	err := printSearchResponse(reader, coreformat.Table)
	assert.NoError(t, err)
}

func TestPrintSearchResponse_UnsupportedFormat(t *testing.T) {
	reader := createSearchContentReader(t, nil)
	defer reader.Close()

	err := printSearchResponse(reader, coreformat.Sarif)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt search")
}

// ---------------------------------------------------------------------------
// printSearchTable tests
// ---------------------------------------------------------------------------

func TestPrintSearchTable_WithResults(t *testing.T) {
	items := []map[string]interface{}{
		{"path": "my-repo/a/b/file.jar", "type": "file", "size": 5678, "sha256": "deadbeef"},
		{"path": "my-repo/a/c/other.zip", "type": "file", "size": 999, "sha256": "cafebabe"},
	}
	reader := createSearchContentReader(t, items)
	defer reader.Close()

	err := printSearchTable(reader)
	assert.NoError(t, err)
}

func TestPrintSearchTable_EmptyResults(t *testing.T) {
	reader := createSearchContentReader(t, nil)
	defer reader.Close()

	err := printSearchTable(reader)
	assert.NoError(t, err)
}
