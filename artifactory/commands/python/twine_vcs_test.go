package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTwineBuildProps_MergesVcs(t *testing.T) {
	originalMerge := mergeVcsPropsForTwine
	mergeVcsPropsForTwine = func(userProps, searchDir string) string {
		assert.Equal(t, "/py/project", searchDir)
		return userProps + ";vcs.revision=twine-sha"
	}
	defer func() { mergeVcsPropsForTwine = originalMerge }()

	base := "build.name=b;build.number=1;build.timestamp=123"
	got := mergeVcsPropsForTwine(base, "/py/project")
	assert.Contains(t, got, "vcs.revision=twine-sha")
}

func TestUvBuildProps_MergesVcs(t *testing.T) {
	originalMerge := mergeVcsPropsForUv
	mergeVcsPropsForUv = func(userProps, searchDir string) string {
		return userProps + ";vcs.revision=uv-sha"
	}
	defer func() { mergeVcsPropsForUv = originalMerge }()

	got := mergeVcsPropsForUv("build.name=b;build.number=1", ".")
	assert.Contains(t, got, "vcs.revision=uv-sha")
}
