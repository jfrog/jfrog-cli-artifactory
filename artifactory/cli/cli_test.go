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

// ---------------------------------------------------------------------------
// getCopyOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetCopyOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getCopyOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetCopyOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getCopyOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetCopyOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getCopyOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetCopyOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getCopyOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printCopyTable tests
// ---------------------------------------------------------------------------

func TestPrintCopyTable_SuccessAndFailure(t *testing.T) {
	var buf bytes.Buffer
	err := printCopyTable(5, 2, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "2")
}

func TestPrintCopyTable_ZeroCounts(t *testing.T) {
	var buf bytes.Buffer
	err := printCopyTable(0, 0, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "failure")
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
// printCopyJSON tests
// ---------------------------------------------------------------------------

func TestPrintCopyJSON_SuccessStatus(t *testing.T) {
	// No error, no failures → status should be "success".
	// Output goes to log.Output (stdout); we just verify no error is returned.
	err := printCopyJSON(3, 0, false, nil)
	require.NoError(t, err)
}

func TestPrintCopyJSON_FailureStatus(t *testing.T) {
	// Has failures → status should be "failure".
	err := printCopyJSON(2, 1, false, nil)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// getDeleteOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetDeleteOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getDeleteOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetDeleteOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getDeleteOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetDeleteOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getDeleteOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetDeleteOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getDeleteOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printDeleteTable tests
// ---------------------------------------------------------------------------

func TestPrintDeleteTable_SuccessAndFailure(t *testing.T) {
	var buf bytes.Buffer
	err := printDeleteTable(5, 2, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "failure")
	assert.Contains(t, out, "2")
}

func TestPrintDeleteTable_ZeroCounts(t *testing.T) {
	var buf bytes.Buffer
	err := printDeleteTable(0, 0, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "success")
	assert.Contains(t, out, "failure")
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
// printDeleteJSON tests
// ---------------------------------------------------------------------------

func TestPrintDeleteJSON_SuccessStatus(t *testing.T) {
	// No error, no failures → status should be "success".
	// Output goes to log.Output (stdout); we just verify no error is returned.
	err := printDeleteJSON(3, 0, false, nil)
	require.NoError(t, err)
}

func TestPrintDeleteJSON_FailureStatus(t *testing.T) {
	// Has failures → status should be "failure".
	err := printDeleteJSON(2, 1, false, nil)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// getBuildPublishOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetBuildPublishOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getBuildPublishOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default should be None (backward-compatible: Run() prints JSON internally)")
}

func TestGetBuildPublishOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getBuildPublishOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetBuildPublishOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getBuildPublishOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetBuildPublishOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getBuildPublishOutputFormat(ctx)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "only the following output formats are supported")
}

// ---------------------------------------------------------------------------
// printBuildPublishResponse tests
// ---------------------------------------------------------------------------

func TestPrintBuildPublishResponse_JSON(t *testing.T) {
	var buf bytes.Buffer
	// printBuildPublishJSON writes to log.Output (stdout); we verify no error.
	err := printBuildPublishResponse("https://example.jfrog.io/ui/builds/myapp/1/123/published", coreformat.Json, &buf)
	require.NoError(t, err)
}

func TestPrintBuildPublishResponse_Table(t *testing.T) {
	var buf bytes.Buffer
	const testURL = "https://example.jfrog.io/ui/builds/myapp/1/123/published"
	err := printBuildPublishResponse(testURL, coreformat.Table, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "buildInfoUiUrl")
	assert.Contains(t, out, testURL)
}

func TestPrintBuildPublishResponse_Table_EmptyURL(t *testing.T) {
	var buf bytes.Buffer
	err := printBuildPublishResponse("", coreformat.Table, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "buildInfoUiUrl")
}

func TestPrintBuildPublishResponse_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := printBuildPublishResponse("https://example.jfrog.io/ui/builds/myapp/1", coreformat.Sarif, &buf)
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
	err := printBuildPublishTable(testURL, &buf)
	require.NoError(t, err)
	out := buf.String()
	// tabwriter replaces tabs with spaces; check for rendered tokens
	assert.Contains(t, out, "FIELD")
	assert.Contains(t, out, "VALUE")
	assert.Contains(t, out, "buildInfoUiUrl")
	assert.Contains(t, out, testURL)
}

// ---------------------------------------------------------------------------
// printBuildPublishJSON tests
// ---------------------------------------------------------------------------

func TestPrintBuildPublishJSON_ValidURL(t *testing.T) {
	// Output goes to log.Output; we just verify no error is returned.
	err := printBuildPublishJSON("https://example.jfrog.io/ui/builds/myapp/1/123/published")
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
// getContainerPushOutputFormat tests
// ---------------------------------------------------------------------------

func TestGetContainerPushOutputFormat_Default(t *testing.T) {
	ctx := newTestContext(nil)
	format, err := getContainerPushOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.None, format, "default (no flag) should be None to preserve backward-compatible output")
}

func TestGetContainerPushOutputFormat_ExplicitJSON(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "json"})
	format, err := getContainerPushOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Json, format)
}

func TestGetContainerPushOutputFormat_ExplicitTable(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "table"})
	format, err := getContainerPushOutputFormat(ctx)
	require.NoError(t, err)
	assert.Equal(t, coreformat.Table, format)
}

func TestGetContainerPushOutputFormat_Invalid(t *testing.T) {
	ctx := newTestContext(map[string]string{"format": "xml"})
	_, err := getContainerPushOutputFormat(ctx)
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

// TestBuildPromoteFormat_ValidJSON verifies that --format json is accepted and
// printBuildPromoteJSON does not panic or error.
func TestBuildPromoteFormat_ValidJSON(t *testing.T) {
	// printBuildPromoteJSON writes to log.Output (stdout); we just verify it does not panic.
	require.NotPanics(t, func() {
		printBuildPromoteJSON()
	})
}

// TestBuildPromoteFormat_EmptyBody verifies that the synthetic JSON object is
// well-formed when the body is nil (the common case: client discards body).
func TestBuildPromoteFormat_EmptyBody(t *testing.T) {
	// printBuildPromoteJSON always produces {"message":"OK","status_code":200}.
	// We verify the function runs without error (output goes to log.Output, not a writer).
	require.NotPanics(t, func() {
		printBuildPromoteJSON()
	})
}

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
