package helm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandlePushCommand_BuildPropsIncludeVcs(t *testing.T) {
	originalMerge := mergeVcsPropsForHelmPush
	calledSearchDir := ""
	mergeVcsPropsForHelmPush = func(userProps, searchDir string) string {
		calledSearchDir = searchDir
		return userProps + ";vcs.revision=abc123"
	}
	defer func() { mergeVcsPropsForHelmPush = originalMerge }()

	chartDir := "/charts/myapp"
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", "b", "1", "123")
	merged := mergeVcsPropsForHelmPush(buildProps, chartDir)
	assert.Equal(t, chartDir, calledSearchDir)
	assert.Contains(t, merged, "vcs.revision=abc123")
}
