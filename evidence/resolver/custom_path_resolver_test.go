package resolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomPathResolver(t *testing.T) {
	r := NewCustomPathResolver("repo/path/file")
	p, err := r.ResolveSubjectRepoPath()
	assert.NoError(t, err)
	assert.Equal(t, "repo/path/file", p)
}

func TestCustomPathResolver_Empty(t *testing.T) {
	r := NewCustomPathResolver("")
	_, err := r.ResolveSubjectRepoPath()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}
