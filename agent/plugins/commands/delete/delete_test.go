package delete

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeleteCommand_MissingVersion(t *testing.T) {
	cmd := NewDeleteCommand().
		SetSlug("test-plugin").
		SetRepoKey("repo")
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--version is required")
}

func TestDeleteCommand_DryRun(t *testing.T) {
	cmd := NewDeleteCommand().
		SetSlug("test-plugin").
		SetRepoKey("repo").
		SetVersion("1.0.0").
		SetDryRun(true)
	// Dry run with no server should succeed without any network call.
	err := cmd.Run()
	assert.NoError(t, err)
}
