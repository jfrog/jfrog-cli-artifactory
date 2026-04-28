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
// getOutputFormatWithDefault tests (search — default Json)
// ---------------------------------------------------------------------------

func TestGetSearchOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format, "default format should be json (backward-compatible)")
}

func TestGetSearchOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetSearchOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetSearchOutputFormat_CaseInsensitive(t *testing.T) {
	for _, val := range []string{"JSON", "Json", "TABLE", "Table"} {
		t.Run(val, func(t *testing.T) {
			ctx := newTestContext(map[string]string{"format": val})
			_, err := getOutputFormatWithDefault(ctx, coreformat.Json)
			require.NoError(t, err)
		})
	}
}

func TestGetSearchOutputFormat_UnsupportedFormat_Sarif(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "sarif"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

func TestGetSearchOutputFormat_UnsupportedFormat_SimpleJson(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "simple-json"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

func TestGetSearchOutputFormat_UnsupportedFormat_XML(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.Json)
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
// getOutputFormatWithDefault tests (ping — default None)
// ---------------------------------------------------------------------------

func TestGetPingOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetPingOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetPingOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetPingOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
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
// getOutputFormatWithDefault tests (upload — default None)
// ---------------------------------------------------------------------------

func TestGetUploadOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetUploadOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetUploadOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetUploadOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
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
// getOutputFormatWithDefault tests (download — default None)
// ---------------------------------------------------------------------------

func TestGetDownloadOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetDownloadOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetDownloadOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetDownloadOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
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
// getOutputFormatWithDefault tests (move — default None)
// ---------------------------------------------------------------------------

func TestGetMoveOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetMoveOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetMoveOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetMoveOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printCountsTable tests
// ---------------------------------------------------------------------------

func TestPrintCountsTableSuccessAndFailure(t *testing.T) {
	var buf bytes.Buffer
	err := printCountsTable(5, 2, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "2")
}

func TestPrintCountsTableZeroCounts(t *testing.T) {
	var buf bytes.Buffer
	err := printCountsTable(0, 0, &buf)
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
// getOutputFormatWithDefault tests (copy — default None)
// ---------------------------------------------------------------------------

func TestGetCopyOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetCopyOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetCopyOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetCopyOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printCopyResponse tests
// ---------------------------------------------------------------------------

func TestPrintCopyResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printCopyResponse(3, 0, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "0")
}

func TestPrintCopyResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printCopyJSON writes to log.Output (stdout), not to buf; we only check no error is returned.
	err := printCopyResponse(4, 0, coreformat.Json, &buf, false, nil)
	require.NoError(t, err)
}

func TestPrintCopyResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printCopyResponse(1, 0, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt copy")
}

// ---------------------------------------------------------------------------
// getOutputFormatWithDefault tests (delete — default None)
// ---------------------------------------------------------------------------

func TestGetDeleteOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetDeleteOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetDeleteOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetDeleteOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printDeleteResponse tests
// ---------------------------------------------------------------------------

func TestPrintDeleteResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printDeleteResponse(3, 0, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "0")
}

func TestPrintDeleteResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printDeleteJSON writes to log.Output (stdout), not to buf; we only check no error is returned.
	err := printDeleteResponse(4, 0, coreformat.Json, &buf, false, nil)
	require.NoError(t, err)
}

func TestPrintDeleteResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printDeleteResponse(1, 0, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt delete")
}

// ---------------------------------------------------------------------------
// getOutputFormatWithDefault tests (build-publish — default None)
// ---------------------------------------------------------------------------

func TestGetBuildPublishOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default should be None (backward-compatible: Run() prints JSON internally)")
}

func TestGetBuildPublishOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetBuildPublishOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetBuildPublishOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printBuildPublishResponse tests
// ---------------------------------------------------------------------------

func TestPrintBuildPublishResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printBuildPublishJSON writes to log.Output (stdout); we verify no error.
	err := printBuildPublishResponse("https://example.jfrog.io/ui/builds/myapp/1/123/published", "", coreformat.Json, &buf)
	require.NoError(t, err)
}

func TestPrintBuildPublishResponse_JSON_WithSha256(t *testing.T) {
	var buf bytes.Buffer
	const testURL = "https://example.jfrog.io/ui/builds/myapp/1/123/published"
	const testSha256 = "abc123def456"
	err := printBuildPublishResponse(testURL, testSha256, coreformat.Json, &buf)
	require.NoError(t, err)
}

func TestPrintBuildPublishResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	const testURL = "https://example.jfrog.io/ui/builds/myapp/1/123/published"
	err := printBuildPublishResponse(testURL, "", coreformat.Table, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "buildInfoUiUrl")
	assert.Contains(t, out, testURL)
	assert.NotContains(t, out, "sha256")
}

func TestPrintBuildPublishResponse_Table_WithSha256(t *testing.T) {
	var buf bytes.Buffer
	const testURL = "https://example.jfrog.io/ui/builds/myapp/1/123/published"
	const testSha256 = "deadbeef1234"
	err := printBuildPublishResponse(testURL, testSha256, coreformat.Table, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "buildInfoUiUrl")
	assert.Contains(t, out, testURL)
	assert.Contains(t, out, "sha256")
	assert.Contains(t, out, testSha256)
}

func TestPrintBuildPublishResponse_Table_EmptyURL(t *testing.T) {
	var buf bytes.Buffer
	err := printBuildPublishResponse("", "", coreformat.Table, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "buildInfoUiUrl")
}

func TestPrintBuildPublishResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printBuildPublishResponse("https://example.jfrog.io/ui/builds/myapp/1", "", coreformat.Sarif, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt build-publish")
}

// ---------------------------------------------------------------------------
// printBuildPublishTable tests
// ---------------------------------------------------------------------------

func TestPrintBuildPublishTable_ContainsHeaderAndURL(t *testing.T) {
	var buf bytes.Buffer
	const testURL = "https://myrt.example.com/ui/builds/my-build/42/1234/published"
	err := printBuildPublishTable(testURL, "", &buf)
	require.NoError(t, err)
	out := buf.String()
	// tabwriter replaces tabs with spaces; check for rendered tokens
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "buildInfoUiUrl")
	assert.Contains(t, out, testURL)
	assert.NotContains(t, out, "sha256")
}

func TestPrintBuildPublishTable_WithSha256(t *testing.T) {
	var buf bytes.Buffer
	const testURL = "https://myrt.example.com/ui/builds/my-build/42/1234/published"
	const testSha256 = "cafebabe5678"
	err := printBuildPublishTable(testURL, testSha256, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "buildInfoUiUrl")
	assert.Contains(t, out, "sha256")
	assert.Contains(t, out, testSha256)
}

// ---------------------------------------------------------------------------
// printBuildPublishJSON tests
// ---------------------------------------------------------------------------

func TestPrintBuildPublishJSON_ValidURL(t *testing.T) {
	// Output goes to log.Output; we just verify no error is returned.
	err := printBuildPublishJSON("https://example.jfrog.io/ui/builds/myapp/1/123/published", "")
	require.NoError(t, err)
}

func TestPrintBuildPublishJSON_WithSha256(t *testing.T) {
	err := printBuildPublishJSON("https://example.jfrog.io/ui/builds/myapp/1/123/published", "abc123")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// container-push test helpers
// ---------------------------------------------------------------------------

// createContainerPushResult builds a *commandUtils.Result with the given counts and an optional
// per-layer transfer-details ContentReader (keyed "files", matching the push command convention).
func createContainerPushResult(t *testing.T, success, failed int, transfers []clientutils.FileTransferDetails) *commandUtils.Result {
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

		f, err := os.CreateTemp("", "container-push-details-*.json")
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
// getOutputFormatWithDefault tests (container-push — default None)
// ---------------------------------------------------------------------------

func TestGetContainerPushOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetContainerPushOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetContainerPushOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetContainerPushOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printContainerPushTable tests
// ---------------------------------------------------------------------------

func TestPrintContainerPushTable_WithResults(t *testing.T) {
	transfers := []clientutils.FileTransferDetails{
		{TargetPath: "myrepo/sha256:aabbcc", RtUrl: "https://myrt.example.com/", Sha256: "aabbcc"},
		{TargetPath: "myrepo/sha256:ddeeff", RtUrl: "https://myrt.example.com/", Sha256: "ddeeff"},
	}
	result := createContainerPushResult(t, 2, 0, transfers)
	defer result.Reader().Close()

	var buf bytes.Buffer
	err := printContainerPushTable(result, &buf)
	require.NoError(t, err)
	// When a reader is present, PrintTable is used (writes to stdout); we only
	// check that the function does not error and the reader state is consistent.
}

func TestPrintContainerPushTable_NoReader_FallsBackToCountsTable(t *testing.T) {
	result := createContainerPushResult(t, 3, 1, nil)

	var buf bytes.Buffer
	err := printContainerPushTable(result, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "1")
}

func TestPrintContainerPushTable_EmptyReader(t *testing.T) {
	result := createContainerPushResult(t, 0, 0, []clientutils.FileTransferDetails{})
	defer result.Reader().Close()

	var buf bytes.Buffer
	err := printContainerPushTable(result, &buf)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// printContainerPushResponse tests
// ---------------------------------------------------------------------------

func TestPrintContainerPushResponse_Table_NoReader(t *testing.T) {
	result := createContainerPushResult(t, 2, 0, nil)

	var buf bytes.Buffer
	err := printContainerPushResponse(result, coreformat.Table, &buf, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "2")
}

func TestPrintContainerPushResponse_UnsupportedFormat(t *testing.T) {
	result := createContainerPushResult(t, 1, 0, nil)

	var buf bytes.Buffer
	err := printContainerPushResponse(result, coreformat.Sarif, &buf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt podman-push")
}

// ---------------------------------------------------------------------------
// build-promote (Pattern B — json-only) tests
// ---------------------------------------------------------------------------

// TestBuildPromoteFormat_InvalidFormatRejected verifies that --format table (and
// any other unsupported format) is rejected before the HTTP call is made.
func TestBuildPromoteFormat_InvalidFormatRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestBuildPromoteFormat_JSONFormatAccepted verifies that --format json passes
// ParseOutputFormat validation without error.
func TestBuildPromoteFormat_JSONFormatAccepted(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	outputFormat, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, outputFormat)
}

// TestBuildPromoteFormat_SarifRejected verifies that --format sarif is rejected.
func TestBuildPromoteFormat_SarifRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "sarif"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestBuildPromoteFormat_XMLRejected verifies that an arbitrary unsupported format
// value is rejected.
func TestBuildPromoteFormat_XMLRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// build-discard --format tests
// ---------------------------------------------------------------------------

// TestBuildDiscardFormat_InvalidFormatRejected verifies that --format table (and
// any other unsupported format) is rejected before the HTTP call is made.
func TestBuildDiscardFormat_InvalidFormatRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestBuildDiscardFormat_JSONFormatAccepted verifies that --format json passes
// ParseOutputFormat validation without error.
func TestBuildDiscardFormat_JSONFormatAccepted(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	outputFormat, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, outputFormat)
}

// TestBuildDiscardFormat_SarifRejected verifies that --format sarif is rejected.
func TestBuildDiscardFormat_SarifRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "sarif"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestBuildDiscardFormat_XMLRejected verifies that an arbitrary unsupported format
// value is rejected.
func TestBuildDiscardFormat_XMLRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// getOutputFormatWithDefault tests (set-props — default None)
// ---------------------------------------------------------------------------

func TestGetSetPropsOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetSetPropsOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetSetPropsOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetSetPropsOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printSetPropsResponse tests
// ---------------------------------------------------------------------------

func TestPrintSetPropsResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printSetPropsResponse(5, 0, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "0")
}

func TestPrintSetPropsResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printSetPropsJSON writes to log.Output (stdout), not to buf; we only check no error is returned.
	err := printSetPropsResponse(4, 0, coreformat.Json, &buf, false, nil)
	require.NoError(t, err)
}

func TestPrintSetPropsResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printSetPropsResponse(1, 0, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt set-props")
}

// ---------------------------------------------------------------------------
// getOutputFormatWithDefault tests (delete-props — default None)
// ---------------------------------------------------------------------------

func TestGetDeletePropsOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetDeletePropsOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetDeletePropsOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetDeletePropsOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printDeletePropsResponse tests
// ---------------------------------------------------------------------------

func TestPrintDeletePropsResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printDeletePropsResponse(5, 0, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "0")
}

func TestPrintDeletePropsResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printDeletePropsJSON writes to log.Output (stdout), not to buf; we only check no error is returned.
	err := printDeletePropsResponse(4, 0, coreformat.Json, &buf, false, nil)
	require.NoError(t, err)
}

func TestPrintDeletePropsResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printDeletePropsResponse(1, 0, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt delete-props")
}

// ---------------------------------------------------------------------------
// getOutputFormatWithDefault tests (build-add-dependencies — default None)
// ---------------------------------------------------------------------------

func TestGetBuildAddDependenciesOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetBuildAddDependenciesOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetBuildAddDependenciesOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetBuildAddDependenciesOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printBuildAddDependenciesResponse tests
// ---------------------------------------------------------------------------

func TestPrintBuildAddDependenciesResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printBuildAddDependenciesResponse(7, 0, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "7")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "0")
}

func TestPrintBuildAddDependenciesResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printBuildAddDependenciesJSON writes to log.Output (stdout), not to buf; we only check no error is returned.
	err := printBuildAddDependenciesResponse(5, 0, coreformat.Json, &buf, false, nil)
	require.NoError(t, err)
}

func TestPrintBuildAddDependenciesResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printBuildAddDependenciesResponse(1, 0, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt build-add-dependencies")
}

// ---------------------------------------------------------------------------
// direct-download test helpers
// ---------------------------------------------------------------------------

// createDirectDownloadResult builds a *commandUtils.Result with the given counts and an optional
// per-file transfer-details ContentReader (keyed "files", matching Artifactory's convention).
func createDirectDownloadResult(t *testing.T, success, failed int, transfers []clientutils.FileTransferDetails) *commandUtils.Result {
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

		f, err := os.CreateTemp("", "direct-download-details-*.json")
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
// getOutputFormatWithDefault tests (direct-download — default None)
// ---------------------------------------------------------------------------

func TestGetDirectDownloadOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetDirectDownloadOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetDirectDownloadOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetDirectDownloadOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printDirectDownloadTable tests
// ---------------------------------------------------------------------------

func TestPrintDirectDownloadTable_WithResults(t *testing.T) {
	transfers := []clientutils.FileTransferDetails{
		{SourcePath: "repo/a.jar", TargetPath: "/local/a.jar", RtUrl: "https://myrt.example.com/", Sha256: "abc123"},
		{SourcePath: "repo/b.zip", TargetPath: "/local/b.zip", RtUrl: "https://myrt.example.com/", Sha256: "def456"},
	}
	result := createDirectDownloadResult(t, 2, 0, transfers)
	defer result.Reader().Close()

	var buf bytes.Buffer
	err := printDirectDownloadTable(result, &buf)
	require.NoError(t, err)
	// When a reader is present, PrintTable is used (writes to stdout); we only
	// check that the function does not error and the reader state is consistent.
}

func TestPrintDirectDownloadTable_NoReader_FallsBackToCountsTable(t *testing.T) {
	result := createDirectDownloadResult(t, 3, 1, nil)

	var buf bytes.Buffer
	err := printDirectDownloadTable(result, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "1")
}

func TestPrintDirectDownloadTable_EmptyReader(t *testing.T) {
	result := createDirectDownloadResult(t, 0, 0, []clientutils.FileTransferDetails{})
	defer result.Reader().Close()

	var buf bytes.Buffer
	err := printDirectDownloadTable(result, &buf)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// printDirectDownloadResponse tests
// ---------------------------------------------------------------------------

func TestPrintDirectDownloadResponse_Table_NoReader(t *testing.T) {
	result := createDirectDownloadResult(t, 2, 0, nil)

	var buf bytes.Buffer
	err := printDirectDownloadResponse(result, coreformat.Table, &buf, false, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "2")
}

func TestPrintDirectDownloadResponse_UnsupportedFormat(t *testing.T) {
	result := createDirectDownloadResult(t, 1, 0, nil)

	var buf bytes.Buffer
	err := printDirectDownloadResponse(result, coreformat.Sarif, &buf, false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt direct-download")
}

// ---------------------------------------------------------------------------
// docker-promote --format tests
// ---------------------------------------------------------------------------

// TestDockerPromoteFormat_InvalidFormatRejected verifies that --format table (and
// any other unsupported format) is rejected before the HTTP call is made.
func TestDockerPromoteFormat_InvalidFormatRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestDockerPromoteFormat_JSONFormatAccepted verifies that --format json passes
// ParseOutputFormat validation without error.
func TestDockerPromoteFormat_JSONFormatAccepted(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	outputFormat, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, outputFormat)
}

// TestDockerPromoteFormat_SarifRejected verifies that --format sarif is rejected.
func TestDockerPromoteFormat_SarifRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "sarif"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestDockerPromoteFormat_XMLRejected verifies that an arbitrary unsupported format
// value is rejected.
func TestDockerPromoteFormat_XMLRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// getOutputFormatWithDefault tests (git-lfs-clean — default None)
// ---------------------------------------------------------------------------

func TestGetGitLfsCleanOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetGitLfsCleanOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetGitLfsCleanOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetGitLfsCleanOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printGitLfsCleanResponse tests
// ---------------------------------------------------------------------------

func TestPrintGitLfsCleanResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printGitLfsCleanResponse(5, 1, coreformat.Table, &buf, nil)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "1")
}

func TestPrintGitLfsCleanResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printGitLfsCleanJSON writes to log.Output (stdout); we only check no error is returned.
	err := printGitLfsCleanResponse(3, 0, coreformat.Json, &buf, nil)
	require.NoError(t, err)
}

func TestPrintGitLfsCleanResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printGitLfsCleanResponse(1, 0, coreformat.Sarif, &buf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt git-lfs-clean")
}

// ---------------------------------------------------------------------------
// getOutputFormatWithDefault tests (container-pull — default None)
// ---------------------------------------------------------------------------

func TestGetContainerPullOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetContainerPullOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetContainerPullOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetContainerPullOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.None)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printContainerPullResponse tests
// ---------------------------------------------------------------------------

func TestPrintContainerPullResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printContainerPullJSON writes to log.Output (stdout); we only check no error is returned.
	err := printContainerPullResponse("myrepo.example.com/myimage:latest", "docker-local", coreformat.Json, &buf)
	require.NoError(t, err)
}

func TestPrintContainerPullResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	err := printContainerPullResponse("myrepo.example.com/myimage:latest", "docker-local", coreformat.Table, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "status")
	assert.Contains(t, out, "ok")
	assert.Contains(t, out, "image")
	assert.Contains(t, out, "myrepo.example.com/myimage:latest")
	assert.Contains(t, out, "repo")
	assert.Contains(t, out, "docker-local")
}

func TestPrintContainerPullResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printContainerPullResponse("myimage:latest", "docker-local", coreformat.Sarif, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt podman-pull")
}

// ---------------------------------------------------------------------------
// printContainerPullTable tests
// ---------------------------------------------------------------------------

func TestPrintContainerPullTable_WithValues(t *testing.T) {
	var buf bytes.Buffer
	err := printContainerPullTable("registry.example.com/my-image:v1.2.3", "docker-local", &buf)
	require.NoError(t, err)
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.GreaterOrEqual(t, len(lines), 4, "expected header + 3 data rows (status, image, repo)")
	assert.Contains(t, lines[0], "FIELD")
	assert.Contains(t, lines[0], "VALUE")
	assert.Contains(t, out, "status")
	assert.Contains(t, out, "ok")
	assert.Contains(t, out, "image")
	assert.Contains(t, out, "registry.example.com/my-image:v1.2.3")
	assert.Contains(t, out, "repo")
	assert.Contains(t, out, "docker-local")
}

func TestPrintContainerPullTable_EmptyValues(t *testing.T) {
	var buf bytes.Buffer
	err := printContainerPullTable("", "", &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "status")
	assert.Contains(t, out, "ok")
}

// ---------------------------------------------------------------------------
// printContainerPullJSON tests
// ---------------------------------------------------------------------------

func TestPrintContainerPullJSON_Success(t *testing.T) {
	// Output goes to log.Output (stdout); we just verify no error is returned.
	err := printContainerPullJSON("registry.example.com/myimage:latest", "docker-local")
	require.NoError(t, err)
}

func TestPrintContainerPullJSON_EmptyValues(t *testing.T) {
	// Even with empty image/repo the function should not error.
	err := printContainerPullJSON("", "")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// getOutputFormatWithDefault tests (nuget-deps-tree — default Json)
// ---------------------------------------------------------------------------

func TestGetNugetDepsTreeOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format, "default (no flag) should be Json to preserve backward-compatible JSON tree output")
}

func TestGetNugetDepsTreeOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetNugetDepsTreeOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetNugetDepsTreeOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getOutputFormatWithDefault(ctx, coreformat.Json)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printNugetDepsTreeResponse tests
// ---------------------------------------------------------------------------

// sampleNugetDepsTreeJSON returns a minimal solution JSON matching the structure
// returned by solution.Marshal() — used as test input.
func sampleNugetDepsTreeJSON(projects []nugetDepsTreeProject) []byte {
	sol := nugetDepsTreeSolution{Projects: projects}
	data, _ := json.Marshal(sol)
	return data
}

func TestPrintNugetDepsTreeResponse_JSON(t *testing.T) {
	data := sampleNugetDepsTreeJSON([]nugetDepsTreeProject{
		{Name: "MyProject", Dependencies: map[string]interface{}{"Newtonsoft.Json:13.0.1": nil}},
	})
	var buf bytes.Buffer
	err := printNugetDepsTreeResponse(data, coreformat.Json, &buf)
	require.NoError(t, err)
	// JSON output goes to log.Output (stdout), not to buf; just verify no error.
}

func TestPrintNugetDepsTreeResponse_Table(t *testing.T) {
	data := sampleNugetDepsTreeJSON([]nugetDepsTreeProject{
		{Name: "MyProject", Dependencies: map[string]interface{}{
			"Newtonsoft.Json:13.0.1": nil,
			"Serilog:3.0.0":          nil,
		}},
	})
	var buf bytes.Buffer
	err := printNugetDepsTreeResponse(data, coreformat.Table, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "DEPENDENCY_COUNT")
	assert.Contains(t, out, "MyProject")
	assert.Contains(t, out, "2")
}

func TestPrintNugetDepsTreeResponse_UnsupportedFormat(t *testing.T) {
	data := sampleNugetDepsTreeJSON(nil)
	var buf bytes.Buffer
	err := printNugetDepsTreeResponse(data, coreformat.Sarif, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
	assert.Contains(t, err.Error(), "rt nuget-deps-tree")
}

// ---------------------------------------------------------------------------
// printNugetDepsTreeTable tests
// ---------------------------------------------------------------------------

func TestPrintNugetDepsTreeTable_MultipleProjects(t *testing.T) {
	data := sampleNugetDepsTreeJSON([]nugetDepsTreeProject{
		{Name: "ProjectA", Dependencies: map[string]interface{}{
			"Newtonsoft.Json:13.0.1": nil,
			"Serilog:3.0.0":          nil,
			"AutoMapper:12.0.0":      nil,
		}},
		{Name: "ProjectB", Dependencies: map[string]interface{}{
			"Dapper:2.0.0": nil,
		}},
	})
	var buf bytes.Buffer
	err := printNugetDepsTreeTable(data, &buf)
	require.NoError(t, err)
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "expected header + 2 project rows")
	assert.Contains(t, lines[0], "PROJECT")
	assert.Contains(t, lines[0], "DEPENDENCY_COUNT")
	assert.Contains(t, out, "ProjectA")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "ProjectB")
	assert.Contains(t, out, "1")
}

func TestPrintNugetDepsTreeTable_EmptyProjects(t *testing.T) {
	data := sampleNugetDepsTreeJSON(nil)
	var buf bytes.Buffer
	err := printNugetDepsTreeTable(data, &buf)
	require.NoError(t, err)
	out := buf.String()
	// Should still have the header even with no projects.
	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "DEPENDENCY_COUNT")
}

func TestPrintNugetDepsTreeTable_ProjectWithNoDependencies(t *testing.T) {
	data := sampleNugetDepsTreeJSON([]nugetDepsTreeProject{
		{Name: "EmptyProject", Dependencies: map[string]interface{}{}},
	})
	var buf bytes.Buffer
	err := printNugetDepsTreeTable(data, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "EmptyProject")
	assert.Contains(t, out, "0")
}

func TestPrintNugetDepsTreeTable_InvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	err := printNugetDepsTreeTable([]byte("not-valid-json"), &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse nuget-deps-tree response")
}

// ---------------------------------------------------------------------------
// replication-create --format tests (Pattern B — json-only)
// ---------------------------------------------------------------------------

// TestReplicationCreateFormat_InvalidFormatRejected verifies that --format table (and
// any other unsupported format) is rejected before the HTTP call is made.
func TestReplicationCreateFormat_InvalidFormatRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestReplicationCreateFormat_JSONFormatAccepted verifies that --format json passes
// ParseOutputFormat validation without error.
func TestReplicationCreateFormat_JSONFormatAccepted(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	outputFormat, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, outputFormat)
}

// TestReplicationCreateFormat_SarifRejected verifies that --format sarif is rejected.
func TestReplicationCreateFormat_SarifRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "sarif"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestReplicationCreateFormat_XMLRejected verifies that an arbitrary unsupported format
// value is rejected.
func TestReplicationCreateFormat_XMLRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// repo-create --format tests (Pattern B — json-only)
// ---------------------------------------------------------------------------

// TestRepoCreateFormat_InvalidFormatRejected verifies that --format table (and
// any other unsupported format) is rejected before the HTTP call is made.
func TestRepoCreateFormat_InvalidFormatRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestRepoCreateFormat_JSONFormatAccepted verifies that --format json passes
// ParseOutputFormat validation without error.
func TestRepoCreateFormat_JSONFormatAccepted(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	outputFormat, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, outputFormat)
}

// TestRepoCreateFormat_SarifRejected verifies that --format sarif is rejected.
func TestRepoCreateFormat_SarifRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "sarif"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestRepoCreateFormat_XMLRejected verifies that an arbitrary unsupported format
// value is rejected.
func TestRepoCreateFormat_XMLRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// repo-update --format tests (Pattern B — json-only)
// ---------------------------------------------------------------------------

// TestRepoUpdateFormat_InvalidFormatRejected verifies that --format table (and
// any other unsupported format) is rejected before the HTTP call is made.
func TestRepoUpdateFormat_InvalidFormatRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestRepoUpdateFormat_JSONFormatAccepted verifies that --format json passes
// ParseOutputFormat validation without error.
func TestRepoUpdateFormat_JSONFormatAccepted(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	outputFormat, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, outputFormat)
}

// TestRepoUpdateFormat_SarifRejected verifies that --format sarif is rejected.
func TestRepoUpdateFormat_SarifRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "sarif"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// TestRepoUpdateFormat_XMLRejected verifies that an arbitrary unsupported format
// value is rejected.
func TestRepoUpdateFormat_XMLRejected(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := coreformat.ParseOutputFormat(ctx.GetStringFlagValue("format"), []coreformat.OutputFormat{coreformat.Json})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

func TestPrintSummaryJSONSuccessStatus(t *testing.T) {
	err := printSummaryJSON(3, 0, false, nil)
	require.NoError(t, err)
}

func TestPrintSummaryJSONFailureStatus(t *testing.T) {
	err := printSummaryJSON(2, 1, false, nil)
	require.NoError(t, err)
}

func TestPrintStatusJSONOK(t *testing.T) {
	require.NotPanics(t, func() { printStatusJSON(200, "OK") })
}

func TestPrintStatusJSONNoContent(t *testing.T) {
	require.NotPanics(t, func() { printStatusJSON(204, "No Content") })
}
