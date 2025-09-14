package resolvers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveSubject_Docker(t *testing.T) {
	// Test Docker subject resolution
	subject := "docker://nginx:latest"
	checksum := "sha256:1234567890abcdef"

	// This test would require a more complex mock setup for the full resolver chain
	// For now, we'll test the basic functionality
	result, err := ResolveSubject(subject, checksum, nil)

	// Should return error due to nil client
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "artifactory client cannot be nil")
}

func TestResolveSubject_OCI(t *testing.T) {
	// Test OCI subject resolution
	subject := "oci://nginx:latest"
	checksum := "sha256:1234567890abcdef"

	result, err := ResolveSubject(subject, checksum, nil)

	// Should return error due to nil client
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "artifactory client cannot be nil")
}

func TestResolveSubject_NoProtocol(t *testing.T) {
	// Test subject without protocol
	subject := "nginx:latest"
	checksum := "sha256:1234567890abcdef"

	result, err := ResolveSubject(subject, checksum, nil)

	// Should return original subject without error
	assert.NoError(t, err)
	assert.Equal(t, []string{subject}, result)
}

func TestResolveSubject_UnsupportedProtocol(t *testing.T) {
	// Test unsupported protocol
	subject := "unsupported://nginx:latest"
	checksum := "sha256:1234567890abcdef"

	result, err := ResolveSubject(subject, checksum, nil)

	// Should return original subject without error
	assert.NoError(t, err)
	assert.Equal(t, []string{subject}, result)
}

func TestResolveSubject_EmptySubject(t *testing.T) {
	// Test empty subject
	subject := ""
	checksum := "sha256:1234567890abcdef"

	result, err := ResolveSubject(subject, checksum, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "subject cannot be empty")
}

func TestResolveSubject_NilClient(t *testing.T) {
	// Test nil client
	subject := "docker://nginx:latest"
	checksum := "sha256:1234567890abcdef"

	result, err := ResolveSubject(subject, checksum, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "artifactory client cannot be nil")
}

func TestResolveSubject_EmptyProtocol(t *testing.T) {
	// Test subject with empty protocol
	subject := "://nginx:latest"
	checksum := "sha256:1234567890abcdef"

	result, err := ResolveSubject(subject, checksum, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "protocol prefix cannot be empty")
}

func TestResolveSubject_WhitespaceProtocol(t *testing.T) {
	// Test subject with whitespace in protocol
	subject := "  docker  ://nginx:latest"
	checksum := "sha256:1234567890abcdef"

	result, err := ResolveSubject(subject, checksum, nil)

	// Should return error due to nil client, but protocol should be trimmed
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "artifactory client cannot be nil") // Protocol should be recognized after trimming
}
