package maven

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveResolutionCommand(t *testing.T) {
	assert.Equal(t, "resolve", DeriveResolutionCommand([]string{"install"}))
	assert.Equal(t, "resolve", DeriveResolutionCommand([]string{"clean", "verify"}))
	assert.Equal(t, "", DeriveResolutionCommand([]string{"help"}))
	assert.Equal(t, "", DeriveResolutionCommand([]string{"clean"}))
	assert.Equal(t, "", DeriveResolutionCommand([]string{"dependency:tree"}))
}

func TestShouldSkipResolution(t *testing.T) {
	assert.True(t, ShouldSkipResolution([]string{"help"}))
	assert.False(t, ShouldSkipResolution([]string{"install"}))
}
