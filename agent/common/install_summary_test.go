package common

import (
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

func TestPrintUpdateAllSummary_Empty(t *testing.T) {
	require.NoError(t, PrintUpdateAllSummary("Plugin", nil, "table"))
}
