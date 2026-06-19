package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseHarnessList verifies that a comma-separated harness list is split,
// trimmed, and lowercased correctly.
func TestParseHarnessList(t *testing.T) {
	got, err := ParseHarnessList("cursor, Claude")
	require.NoError(t, err)
	assert.Equal(t, []string{"cursor", "claude"}, got)
}

// TestParseHarnessList_EmptyAndDuplicates verifies that an empty string, a
// segment that resolves to an empty name after trimming, and a repeated harness
// name are all rejected with descriptive errors.
func TestParseHarnessList_EmptyAndDuplicates(t *testing.T) {
	_, err := ParseHarnessList("")
	require.Error(t, err)

	_, err = ParseHarnessList("cursor,,claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty name")

	_, err = ParseHarnessList("cursor,cursor")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")
}
