package ocicontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// By default (env var unset) Id() must short-circuit, returning "" without
// invoking daemon.Image / docker save. This is the performance fix for #3382.
// The test does NOT require a running Docker daemon — proof that no daemon
// call is made.
func TestDockerClientId_DefaultSkipsDaemon(t *testing.T) {
	// Force the env var to "false" to protect against a caller having exported
	// it from the shell.
	t.Setenv(EnforceDockerImageIdVerificationEnv, "false")

	cm := &containerManager{Type: DockerClient}
	id, err := cm.Id(NewImage("any-image-that-does-not-exist:latest"))
	require.NoError(t, err, "Id() must not error when the daemon is not consulted")
	assert.Empty(t, id, "Id() must return an empty string when the enforcement env var is disabled")
}

// Any non-"true" value should behave as the default — skip.
func TestDockerClientId_CaseInsensitiveFalseSkipsDaemon(t *testing.T) {
	t.Setenv(EnforceDockerImageIdVerificationEnv, "anything-other-than-true")

	cm := &containerManager{Type: DockerClient}
	id, err := cm.Id(NewImage("any-image:latest"))
	require.NoError(t, err)
	assert.Empty(t, id)
}
