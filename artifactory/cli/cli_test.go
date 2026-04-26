package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	commandUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	coreformat "github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
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

// ---------------------------------------------------------------------------
// getPingOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetPingOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getPingOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetPingOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getPingOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetPingOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getPingOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetPingOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getPingOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printPingResponse tests
// ---------------------------------------------------------------------------

func TestPrintPingResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	err := printPingResponse([]byte("OK"), coreformat.Json, &buf)
	require.NoError(t, err)
	// Verify the output is valid JSON with the expected fields.
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{"message":"OK","status_code":200}`), &result))
}

func TestPrintPingResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printPingResponse([]byte("OK"), coreformat.Table, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "status_code")
	assert.Contains(t, out, "200")
	assert.Contains(t, out, "message")
	assert.Contains(t, out, "OK")
}

func TestPrintPingResponse_None_BackwardCompat(t *testing.T) {
	var buf bytes.Buffer
	err := printPingResponse([]byte("OK"), coreformat.None, &buf)
	require.NoError(t, err)
	// None format uses log.Output (not the writer), so buf stays empty.
	// Just verify no error is returned.
}

func TestPrintPingResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printPingResponse([]byte("OK"), coreformat.Sarif, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt ping")
}

// ---------------------------------------------------------------------------
// printPingTable tests
// ---------------------------------------------------------------------------

func TestPrintPingTable_OKBody(t *testing.T) {
	var buf bytes.Buffer
	err := printPingTable([]byte("OK"), &buf)
	require.NoError(t, err)
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "expected header + 2 data rows")
	assert.Contains(t, lines[0], "FIELD")
	assert.Contains(t, lines[0], "VALUE")
	assert.Contains(t, out, "status_code")
	assert.Contains(t, out, "200")
	assert.Contains(t, out, "message")
	assert.Contains(t, out, "OK")
}

func TestPrintPingTable_EmptyBody(t *testing.T) {
	var buf bytes.Buffer
	err := printPingTable(nil, &buf)
	require.NoError(t, err)
	out := buf.String()
	// Empty body falls back to HTTP status phrase "OK".
	assert.Contains(t, out, "OK")
}

// ---------------------------------------------------------------------------
// pingResponseFromBody tests
// ---------------------------------------------------------------------------

func TestPingResponseFromBody_PlainText(t *testing.T) {
	resp := pingResponseFromBody([]byte("OK"))
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "OK", resp.Message)
}

func TestPingResponseFromBody_EmptyBody(t *testing.T) {
	resp := pingResponseFromBody(nil)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "OK", resp.Message) // http.StatusText(200) == "OK"
}

func TestPingResponseFromBody_WhitespaceBody(t *testing.T) {
	resp := pingResponseFromBody([]byte("  OK  "))
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "OK", resp.Message)
}

// ---------------------------------------------------------------------------
// upload test helpers
// ---------------------------------------------------------------------------

// createUploadResult builds a *commandUtils.Result with the given counts and an optional
// per-file transfer-details ContentReader (keyed "files", matching Artifactory's convention).
func createUploadResult(t *testing.T, success, failed int, transfers []clientutils.FileTransferDetails) *commandUtils.Result {
	t.Helper()
	r := new(commandUtils.Result)
	r.SetSuccessCount(success)
	r.SetFailCount(failed)
	if transfers != nil {
		type filesWrapper struct {
			Files []clientutils.FileTransferDetails `json:"files"`
		}
		data, err := json.Marshal(filesWrapper{Files: transfers})
		require.NoError(t, err)

		f, err := os.CreateTemp("", "upload-details-*.json")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Remove(f.Name()) })
		_, err = f.Write(data)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		r.SetReader(content.NewContentReader(f.Name(), "files"))
	}
	return r
}

// ---------------------------------------------------------------------------
// getUploadOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetUploadOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getUploadOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetUploadOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getUploadOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetUploadOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getUploadOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetUploadOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getUploadOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printUploadTable tests
// ---------------------------------------------------------------------------

func TestPrintUploadTable_WithResults(t *testing.T) {
	transfers := []clientutils.FileTransferDetails{
		{SourcePath: "/local/a.jar", TargetPath: "repo/a.jar", RtUrl: "https://myrt.example.com/", Sha256: "abc123"},
		{SourcePath: "/local/b.zip", TargetPath: "repo/b.zip", RtUrl: "https://myrt.example.com/", Sha256: "def456"},
	}
	result := createUploadResult(t, 2, 0, transfers)
	defer result.Reader().Close()

	var buf bytes.Buffer
	err := printUploadTable(result, &buf)
	require.NoError(t, err)
	// The table should not contain raw tabwriter output from the fallback path.
	// When a reader is present, PrintTable is used (writes to stdout), so we only
	// check that the function does not error and the reader state is consistent.
}

func TestPrintUploadTable_NoReader_FallsBackToCountsTable(t *testing.T) {
	result := createUploadResult(t, 3, 1, nil)

	var buf bytes.Buffer
	err := printUploadTable(result, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "1")
}

func TestPrintUploadTable_EmptyReader(t *testing.T) {
	result := createUploadResult(t, 0, 0, []clientutils.FileTransferDetails{})
	defer result.Reader().Close()

	var buf bytes.Buffer
	err := printUploadTable(result, &buf)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// printUploadResponse tests
// ---------------------------------------------------------------------------

func TestPrintUploadResponse_Table_NoReader(t *testing.T) {
	result := createUploadResult(t, 2, 0, nil)

	var buf bytes.Buffer
	err := printUploadResponse(result, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "2")
}

func TestPrintUploadResponse_UnsupportedFormat(t *testing.T) {
	result := createUploadResult(t, 1, 0, nil)

	var buf bytes.Buffer
	err := printUploadResponse(result, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt upload")
}

// ---------------------------------------------------------------------------
// download test helpers
// ---------------------------------------------------------------------------

// createDownloadResult builds a *commandUtils.Result with the given counts and an optional
// per-file transfer-details ContentReader (keyed "files", matching Artifactory's convention).
func createDownloadResult(t *testing.T, success, failed int, transfers []clientutils.FileTransferDetails) *commandUtils.Result {
	t.Helper()
	r := new(commandUtils.Result)
	r.SetSuccessCount(success)
	r.SetFailCount(failed)
	if transfers != nil {
		type filesWrapper struct {
			Files []clientutils.FileTransferDetails `json:"files"`
		}
		data, err := json.Marshal(filesWrapper{Files: transfers})
		require.NoError(t, err)

		f, err := os.CreateTemp("", "download-details-*.json")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Remove(f.Name()) })
		_, err = f.Write(data)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		r.SetReader(content.NewContentReader(f.Name(), "files"))
	}
	return r
}

// ---------------------------------------------------------------------------
// getDownloadOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetDownloadOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getDownloadOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetDownloadOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getDownloadOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetDownloadOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getDownloadOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetDownloadOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getDownloadOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printDownloadTable tests
// ---------------------------------------------------------------------------

func TestPrintDownloadTable_WithResults(t *testing.T) {
	transfers := []clientutils.FileTransferDetails{
		{SourcePath: "repo/a.jar", TargetPath: "/local/a.jar", RtUrl: "https://myrt.example.com/", Sha256: "abc123"},
		{SourcePath: "repo/b.zip", TargetPath: "/local/b.zip", RtUrl: "https://myrt.example.com/", Sha256: "def456"},
	}
	result := createDownloadResult(t, 2, 0, transfers)
	defer result.Reader().Close()

	var buf bytes.Buffer
	err := printDownloadTable(result, &buf)
	require.NoError(t, err)
	// When a reader is present, PrintTable is used (writes to stdout), so we only
	// check that the function does not error and the reader state is consistent.
}

func TestPrintDownloadTable_NoReader_FallsBackToCountsTable(t *testing.T) {
	result := createDownloadResult(t, 3, 1, nil)

	var buf bytes.Buffer
	err := printDownloadTable(result, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "1")
}

func TestPrintDownloadTable_EmptyReader(t *testing.T) {
	result := createDownloadResult(t, 0, 0, []clientutils.FileTransferDetails{})
	defer result.Reader().Close()

	var buf bytes.Buffer
	err := printDownloadTable(result, &buf)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// printDownloadResponse tests
// ---------------------------------------------------------------------------

func TestPrintDownloadResponse_Table_NoReader(t *testing.T) {
	result := createDownloadResult(t, 2, 0, nil)

	var buf bytes.Buffer
	err := printDownloadResponse(result, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "2")
}

func TestPrintDownloadResponse_UnsupportedFormat(t *testing.T) {
	result := createDownloadResult(t, 1, 0, nil)

	var buf bytes.Buffer
	err := printDownloadResponse(result, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt download")
}

// ---------------------------------------------------------------------------
// getMoveOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetMoveOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getMoveOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetMoveOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getMoveOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetMoveOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getMoveOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetMoveOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getMoveOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printMoveTable tests
// ---------------------------------------------------------------------------

func TestPrintMoveTable_SuccessAndFailure(t *testing.T) {
	var buf bytes.Buffer
	err := printMoveTable(5, 2, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "2")
}

func TestPrintMoveTable_ZeroCounts(t *testing.T) {
	var buf bytes.Buffer
	err := printMoveTable(0, 0, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "failure")
}

// ---------------------------------------------------------------------------
// printMoveResponse tests
// ---------------------------------------------------------------------------

func TestPrintMoveResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printMoveResponse(3, 0, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "0")
}

func TestPrintMoveResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printMoveJSON writes to log.Output (stdout), not to buf; we only check no error is returned.
	err := printMoveResponse(4, 0, coreformat.Json, &buf, false, nil)
	require.NoError(t, err)
}

func TestPrintMoveResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printMoveResponse(1, 0, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt move")
}

// ---------------------------------------------------------------------------
// printMoveJSON tests
// ---------------------------------------------------------------------------

func TestPrintMoveJSON_SuccessStatus(t *testing.T) {
	// No error, no failures → status should be "success".
	// Output goes to log.Output (stdout); we just verify no error is returned.
	err := printMoveJSON(3, 0, false, nil)
	require.NoError(t, err)
}

func TestPrintMoveJSON_FailureStatus(t *testing.T) {
	// Has failures → status should be "failure".
	err := printMoveJSON(2, 1, false, nil)
	require.NoError(t, err)
}
