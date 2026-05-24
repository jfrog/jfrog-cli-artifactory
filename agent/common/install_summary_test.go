package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintInstallSummary_EmptyResultsNoOp(t *testing.T) {
	require.NoError(t, PrintInstallSummary("plugin", "x", "1.0.0", nil, "json"))
}

func TestPrintInstallSummary_JSON(t *testing.T) {
	rows := []SummaryRow{{Agent: "cursor", Scope: "project", Path: "/x/y", Status: SummaryStatusOK, Detail: SummaryDetailOKInstall}}
	require.NoError(t, PrintInstallSummary("plugin", "my-plugin", "1.2.3", rows, "JSON"))
}

func TestCapitalizeFirst(t *testing.T) {
	assert.Equal(t, "Plugin", capitalizeFirst("plugin"))
	assert.Equal(t, "", capitalizeFirst(""))
	assert.Equal(t, "X", capitalizeFirst("x"))
}
