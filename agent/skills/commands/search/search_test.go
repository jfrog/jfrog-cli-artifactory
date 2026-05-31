package search

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintTable(t *testing.T) {
	results := []searchResult{
		{Name: "my-skill", Version: "1.0.0", Repository: "repo-a", Description: "A great skill"},
		{Name: "another-skill", Version: "2.1.0", Repository: "repo-b", Description: "Another one"},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printTable(results)
	require.NoError(t, err)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "REPOSITORY")
	assert.Contains(t, output, "DESCRIPTION")
	assert.Contains(t, output, "my-skill")
	assert.Contains(t, output, "1.0.0")
	assert.Contains(t, output, "repo-a")
	assert.Contains(t, output, "another-skill")
}

func TestPrintJSON(t *testing.T) {
	results := []searchResult{
		{Name: "skill-a", Version: "1.0.0", Repository: "repo", Description: "desc"},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printJSON(results)
	require.NoError(t, err)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var parsed []searchResult
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 1)
	assert.Equal(t, "skill-a", parsed[0].Name)
	assert.Equal(t, "1.0.0", parsed[0].Version)
}

func TestPrintTableEmptyResults(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printTable([]searchResult{})
	require.NoError(t, err)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "No skills found")
}
