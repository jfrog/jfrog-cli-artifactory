package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectPackageVersion_ExactMatchReturnsIt(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "2.0.0"}
	got, err := SelectPackageVersion(available, "1.1.0", "skills-local", true)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", got)
}

func TestSelectPackageVersion_LatestEmpty(t *testing.T) {
	available := []string{"1.0.0", "1.1.0", "2.0.0"}
	got, err := SelectPackageVersion(available, "", "plugins-local", true)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got)
}

func TestSelectPackageVersion_LatestKeyword(t *testing.T) {
	available := []string{"1.0.0", "3.0.0", "2.0.0"}
	got, err := SelectPackageVersion(available, "latest", "plugins-local", true)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", got)
}

func TestSelectPackageVersion_NotFoundQuiet(t *testing.T) {
	available := []string{"1.0.0", "1.1.0"}
	_, err := SelectPackageVersion(available, "9.9.9", "plugins-local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSelectPackageVersion_EmptyAvailableList(t *testing.T) {
	_, err := SelectPackageVersion([]string{}, "", "plugins-local", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "latest version")
}
