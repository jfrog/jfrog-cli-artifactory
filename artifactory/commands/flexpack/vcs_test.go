package flexpack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlexpackMavenBuildProps_MergesVcs(t *testing.T) {
	originalMerge := mergeVcsPropsForFlexpack
	mergeVcsPropsForFlexpack = func(userProps, searchDir string) string {
		return userProps + ";vcs.revision=flex-sha"
	}
	defer func() { mergeVcsPropsForFlexpack = originalMerge }()

	got := mergeVcsPropsForFlexpack("build.name=b;build.number=1;build.timestamp=123", "/flex/project")
	assert.Contains(t, got, "vcs.revision=flex-sha")
}
