package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectPackageVersion_ExactMatchReturnsIt(t *testing.T) {
	got, err := SelectPackageVersion(SelectPackageVersionOpts{
		Available: []string{"1.0.0", "1.1.0", "2.0.0"},
		Requested: "1.1.0",
		RepoKey:   "skills-local",
		Quiet:     true,
	})
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", got)
}

func TestSelectPackageVersion_LatestEmpty(t *testing.T) {
	got, err := SelectPackageVersion(SelectPackageVersionOpts{
		Available: []string{"1.0.0", "1.1.0", "2.0.0"},
		Requested: "",
		RepoKey:   "plugins-local",
		Quiet:     true,
	})
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got)
}

func TestSelectPackageVersion_LatestKeyword(t *testing.T) {
	got, err := SelectPackageVersion(SelectPackageVersionOpts{
		Available: []string{"1.0.0", "3.0.0", "2.0.0"},
		Requested: "latest",
		RepoKey:   "plugins-local",
		Quiet:     true,
	})
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", got)
}

func TestSelectPackageVersion_NotFoundQuiet(t *testing.T) {
	_, err := SelectPackageVersion(SelectPackageVersionOpts{
		Available: []string{"1.0.0", "1.1.0"},
		Requested: "9.9.9",
		RepoKey:   "plugins-local",
		Quiet:     true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSelectPackageVersion_EmptyAvailableList(t *testing.T) {
	_, err := SelectPackageVersion(SelectPackageVersionOpts{
		Available: nil,
		Requested: "",
		RepoKey:   "plugins-local",
		Quiet:     true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "latest version")
}

func TestFindPackageVersion(t *testing.T) {
	version, found := findPackageVersion([]string{"1.0.0", "2.0.0"}, "2.0.0")
	assert.True(t, found)
	assert.Equal(t, "2.0.0", version)

	_, found = findPackageVersion([]string{"1.0.0"}, "9.9.9")
	assert.False(t, found)
}

func TestIsLatestVersionRequest(t *testing.T) {
	assert.True(t, isLatestVersionRequest(""))
	assert.True(t, isLatestVersionRequest("latest"))
	assert.False(t, isLatestVersionRequest("1.0.0"))
}
