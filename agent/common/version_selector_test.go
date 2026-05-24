package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectPackageVersion_ExactMatch(t *testing.T) {
	got, err := SelectPackageVersion([]string{"1.0.0", "1.1.0", "2.0.0"}, "1.1.0", "plugins-local", true)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", got)
}

func TestSelectPackageVersion_LatestEmpty(t *testing.T) {
	got, err := SelectPackageVersion([]string{"1.0.0", "1.1.0", "2.0.0"}, "", "plugins-local", true)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got)
}

func TestSelectPackageVersion_LatestKeyword(t *testing.T) {
	got, err := SelectPackageVersion([]string{"1.0.0", "3.0.0", "2.0.0"}, "latest", "plugins-local", true)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", got)
}

func TestSelectPackageVersion_NotFoundQuiet(t *testing.T) {
	_, err := SelectPackageVersion([]string{"1.0.0"}, "9.9.9", "plugins-local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSelectPackageVersion_EmptyAvailableList(t *testing.T) {
	_, err := SelectPackageVersion(nil, "", "plugins-local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "latest version")
}
