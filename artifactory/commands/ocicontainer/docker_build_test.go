package ocicontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyBuildProps_MergesVcsFromSearchDir(t *testing.T) {
	originalMerge := mergeVcsPropsForDockerBuild
	mergeVcsPropsForDockerBuild = func(userProps, searchDir string) string {
		assert.Equal(t, "/tmp/docker-project", searchDir)
		return userProps + ";vcs.revision=deadbeef"
	}
	defer func() { mergeVcsPropsForDockerBuild = originalMerge }()

	got := mergeBuildPropsWithVcs("build.name=foo;build.number=1", "/tmp/docker-project")
	assert.Contains(t, got, "vcs.revision=deadbeef")
	assert.Contains(t, got, "build.name=foo")
}
