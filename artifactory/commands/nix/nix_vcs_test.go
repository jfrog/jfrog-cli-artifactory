package nix

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNixBuildProps_MergesVcs(t *testing.T) {
	originalMerge := mergeVcsPropsForNix
	mergeVcsPropsForNix = func(userProps, searchDir string) string {
		assert.Equal(t, "/nix/flake", searchDir)
		return userProps + ";vcs.revision=nix-sha"
	}
	defer func() { mergeVcsPropsForNix = originalMerge }()

	base := "build.name=b;build.number=1;build.timestamp=123"
	got := mergeVcsPropsForNix(base, "/nix/flake")
	assert.Contains(t, got, "vcs.revision=nix-sha")
}
