package npm

import (
	"testing"

	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

func TestAddCIVcsProps_MergesLocalGitFromWorkingDirectory(t *testing.T) {
	nru := &npmRtUpload{NpmPublishCommand: &NpmPublishCommand{
		NpmPublishCommandArgs: &NpmPublishCommandArgs{workingDirectory: "/tmp/npm-project"},
	}}

	mergeCalledWith := ""
	originalMerge := mergeVcsPropsForNpmPublish
	mergeVcsPropsForNpmPublish = func(userProps, searchDir string) string {
		mergeCalledWith = searchDir
		return userProps + ";vcs.revision=abc123"
	}
	defer func() { mergeVcsPropsForNpmPublish = originalMerge }()

	params := &specutils.CommonParams{}
	assert.NoError(t, nru.addCIVcsProps(params))
	assert.Equal(t, "/tmp/npm-project", mergeCalledWith)
	revisions := params.TargetProps.ToMap()["vcs.revision"]
	assert.Contains(t, revisions, "abc123")
}

func TestAddCIVcsProps_NoChangeWhenMergeUnchanged(t *testing.T) {
	nru := &npmRtUpload{NpmPublishCommand: &NpmPublishCommand{
		NpmPublishCommandArgs: &NpmPublishCommandArgs{workingDirectory: "/tmp/npm-project"},
	}}
	originalMerge := mergeVcsPropsForNpmPublish
	mergeVcsPropsForNpmPublish = func(userProps, _ string) string {
		return userProps
	}
	defer func() { mergeVcsPropsForNpmPublish = originalMerge }()

	params := &specutils.CommonParams{}
	assert.NoError(t, nru.addCIVcsProps(params))
	assert.Nil(t, params.TargetProps)
}
