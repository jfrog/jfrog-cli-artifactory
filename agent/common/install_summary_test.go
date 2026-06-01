package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendUpdateAllSummaryRows(t *testing.T) {
	rows := []SummaryRow{
		{Agent: "cursor", Scope: "project", Path: "/a/foo", Status: SummaryStatusOK, Detail: SummaryDetailOKInstall},
		{Agent: "cursor", Scope: "project", Path: "/a/bar", Status: SummaryStatusSkipped, Detail: "already at version"},
	}
	combined := AppendUpdateAllSummaryRows(nil, "foo", "1.0.0", rows)
	require.Len(t, combined, 2)
	assert.Equal(t, "cursor", combined[0].Agent)
	assert.Equal(t, "foo", combined[0].Name)
	assert.Equal(t, "project", combined[0].Scope)
	assert.Equal(t, "/a/foo", combined[0].Path)
	assert.Equal(t, "1.0.0", combined[0].Version)
	assert.Equal(t, "foo", combined[1].Name)
	assert.Equal(t, "/a/bar", combined[1].Path)
}

func TestAppendUpdateAllSummaryRows_PreservesFailedRows(t *testing.T) {
	rows := []SummaryRow{
		{
			Agent:  "claude",
			Scope:  "global",
			Path:   "/home/.claude/plugins/web",
			Status: SummaryStatusFailed,
			Detail: "download failed: connection reset",
		},
		{
			Agent:  "cursor",
			Scope:  "project",
			Path:   "/proj/.cursor/plugins/web",
			Status: SummaryStatusFailed,
			Detail: "plugin not installed at /proj/.cursor/plugins/web; run 'jf agent plugins install' first",
		},
	}
	combined := AppendUpdateAllSummaryRows(nil, "web", "2.0.0", rows)
	require.Len(t, combined, 2)

	assert.Equal(t, "web", combined[0].Name)
	assert.Equal(t, "2.0.0", combined[0].Version)
	assert.Equal(t, SummaryStatusFailed, combined[0].Status)
	assert.Equal(t, "download failed: connection reset", combined[0].Detail)

	assert.Equal(t, "web", combined[1].Name)
	assert.Equal(t, SummaryStatusFailed, combined[1].Status)
	assert.Contains(t, combined[1].Detail, "not installed")
}

func TestAppendUpdateAllSummaryRows_EmptyInput(t *testing.T) {
	combined := AppendUpdateAllSummaryRows(nil, "foo", "1.0.0", nil)
	assert.Empty(t, combined)

	combined = AppendUpdateAllSummaryRows(combined, "foo", "1.0.0", []SummaryRow{})
	assert.Empty(t, combined)
}

func TestAppendUpdateAllSummaryRows_AppendsToExistingDest(t *testing.T) {
	existing := []UpdateAllSummaryRow{
		{Agent: "cursor", Name: "alpha", Scope: "project", Path: "/a/alpha", Status: SummaryStatusOK, Version: "1.0.0"},
	}
	rows := []SummaryRow{
		{Agent: "claude", Scope: "global", Path: "/g/beta", Status: SummaryStatusSkipped, Detail: "already at version"},
	}
	combined := AppendUpdateAllSummaryRows(existing, "beta", "2.0.0", rows)
	require.Len(t, combined, 2)
	assert.Equal(t, "alpha", combined[0].Name)
	assert.Equal(t, "beta", combined[1].Name)
	assert.Equal(t, "2.0.0", combined[1].Version)
	assert.Equal(t, SummaryStatusSkipped, combined[1].Status)
}

func TestAppendUpdateAllSummaryRows_MultipleSlugs(t *testing.T) {
	var combined []UpdateAllSummaryRow
	combined = AppendUpdateAllSummaryRows(combined, "foo", "1.0.0", []SummaryRow{
		{Agent: "cursor", Scope: "project", Path: "/a/foo", Status: SummaryStatusOK, Detail: SummaryDetailOKInstall},
	})
	combined = AppendUpdateAllSummaryRows(combined, "bar", "3.0.0", []SummaryRow{
		{Agent: "cursor", Scope: "project", Path: "/a/bar", Status: SummaryStatusFailed, Detail: "could not move current plugin aside"},
	})
	require.Len(t, combined, 2)
	assert.Equal(t, "foo", combined[0].Name)
	assert.Equal(t, "1.0.0", combined[0].Version)
	assert.Equal(t, SummaryStatusOK, combined[0].Status)
	assert.Equal(t, "bar", combined[1].Name)
	assert.Equal(t, "3.0.0", combined[1].Version)
	assert.Equal(t, SummaryStatusFailed, combined[1].Status)
}

func TestPrintUpdateAllSummary_Empty(t *testing.T) {
	require.NoError(t, PrintUpdateAllSummary("Plugin", nil, "table"))
}

func TestPrintUpdateAllSummary_MixedStatuses_NoError(t *testing.T) {
	results := AppendUpdateAllSummaryRows(nil, "web", "2.0.0", []SummaryRow{
		{Agent: "cursor", Scope: "project", Path: "/p/web", Status: SummaryStatusOK, Detail: SummaryDetailOKInstall},
		{Agent: "claude", Scope: "global", Path: "/g/web", Status: SummaryStatusFailed, Detail: "download failed"},
	})
	require.NoError(t, PrintUpdateAllSummary("Plugin", results, "table"))
	require.NoError(t, PrintUpdateAllSummary("Plugin", results, "json"))
}

func TestPrintUpdateAllSummary_JSONRoundTrip_MixedStatuses(t *testing.T) {
	results := AppendUpdateAllSummaryRows(nil, "slug-a", "1.2.3", []SummaryRow{
		{Agent: "a1", Scope: "project", Path: "/p/a", Status: SummaryStatusOK, Detail: SummaryDetailOKInstall},
		{Agent: "a2", Scope: "project", Path: "/p/b", Status: SummaryStatusSkipped, Detail: "version already 1.2.3"},
		{Agent: "a3", Scope: "project", Path: "/p/c", Status: SummaryStatusFailed, Detail: "evidence verification failed"},
	})
	data, err := json.MarshalIndent(updateAllSummaryJSON{Results: results}, "", "  ")
	require.NoError(t, err)

	var payload updateAllSummaryJSON
	require.NoError(t, json.Unmarshal(data, &payload))
	require.Len(t, payload.Results, 3)
	assert.Equal(t, "slug-a", payload.Results[0].Name)
	assert.Equal(t, SummaryStatusFailed, payload.Results[2].Status)
	assert.Contains(t, payload.Results[2].Detail, "evidence")
}
