package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHuggingFaceBuildProps_MergesVcs(t *testing.T) {
	originalMerge := mergeVcsPropsForHuggingFace
	mergeVcsPropsForHuggingFace = func(userProps, searchDir string) string {
		assert.Equal(t, "/models/bert", searchDir)
		return userProps + ";vcs.revision=hf-sha"
	}
	defer func() { mergeVcsPropsForHuggingFace = originalMerge }()

	base := "build.name=b;build.number=1;build.timestamp=123"
	got := mergeVcsPropsForHuggingFace(base, "/models/bert")
	assert.Contains(t, got, "vcs.revision=hf-sha")
}
