package common

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintSearchResults_Table(t *testing.T) {
	rows := []SearchResultRow{
		{Name: "my-item", Version: "1.0.0", Repository: "repo-a", Description: "desc"},
	}
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintSearchResults(rows, PrintSearchResultsOptions{
		Query:           "q",
		Format:          "table",
		TableTitle:      "Items",
		EmptyTableLabel: "No items found",
		NotFoundMessage: "No items for '%s'.",
	})
	require.NoError(t, err)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	assert.Contains(t, buf.String(), "my-item")
}

func TestPrintSearchResults_JSON(t *testing.T) {
	rows := []SearchResultRow{{Name: "a", Version: "1.0.0", Repository: "r"}}
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintSearchResults(rows, PrintSearchResultsOptions{Query: "a", Format: "json", NotFoundMessage: "%s"})
	require.NoError(t, err)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	var parsed []SearchResultRow
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Len(t, parsed, 1)
}
