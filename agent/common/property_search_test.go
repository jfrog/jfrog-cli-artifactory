package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePropertySearchURI_Valid(t *testing.T) {
	uri := "https://example.com/artifactory/api/storage/plugins-local/my-plugin/1.0.0/my-plugin-1.0.0.zip"
	got, ok := parsePropertySearchURI(uri)
	require.True(t, ok)
	assert.Equal(t, "plugins-local", got.Repo)
	assert.Equal(t, "my-plugin", got.Name)
	assert.Equal(t, "1.0.0", got.Version)
	assert.Equal(t, uri, got.URI)
}

func TestParsePropertySearchURI_Invalid(t *testing.T) {
	_, ok := parsePropertySearchURI("https://example.com/artifactory/plugins-local/foo")
	assert.False(t, ok)
}

func TestValidatePropertySearchOpts(t *testing.T) {
	_, err := validatePropertySearchOpts(PropertySearchOptions{})
	require.Error(t, err)

	got, err := validatePropertySearchOpts(PropertySearchOptions{
		NamePropertyKey: "agentplugins.name",
		Query:           " my-plugin ",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-plugin", got)
}

func TestPropertySearchRequestURL(t *testing.T) {
	opts := PropertySearchOptions{
		NamePropertyKey: "agentplugins.name",
		Query:           "demo",
		RepoKey:         "plugins-local",
	}
	got := propertySearchRequestURL("https://example.com/artifactory/", opts, "demo")
	assert.Contains(t, got, "https://example.com/artifactory/"+artifactoryPropertySearchAPI+"?agentplugins.name=demo")
	assert.Contains(t, got, "repos=plugins-local")
}
