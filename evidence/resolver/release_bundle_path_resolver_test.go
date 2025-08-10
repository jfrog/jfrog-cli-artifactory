package resolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReleaseBundlePathResolver(t *testing.T) {
	r := NewReleaseBundlePathResolver("proj", "rb", "1.0.0")
	p, err := r.ResolveSubjectRepoPath()
	assert.NoError(t, err)
	assert.Equal(t, "proj-release-bundles-v2/rb/1.0.0/release-bundle.json.evd", p)
}
