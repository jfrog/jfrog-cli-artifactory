package vscode

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
)

// ... existing code ...

func TestHasServerConfigFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    map[string]string
		expected bool
	}{
		{
			name:     "No flags",
			flags:    map[string]string{},
			expected: false,
		},
		{
			name:     "Only password flag",
			flags:    map[string]string{"password": "mypass"},
			expected: false,
		},
		{
			name:     "Password and URL flags",
			flags:    map[string]string{"password": "mypass", "url": "https://example.com"},
			expected: true,
		},
		{
			name:     "Password and server-id flags",
			flags:    map[string]string{"password": "mypass", "server-id": "my-server"},
			expected: true,
		},
		{
			name:     "URL flag only",
			flags:    map[string]string{"url": "https://example.com"},
			expected: true,
		},
		{
			name:     "User flag only",
			flags:    map[string]string{"user": "myuser"},
			expected: true,
		},
		{
			name:     "Access token flag only",
			flags:    map[string]string{"access-token": "mytoken"},
			expected: true,
		},
		{
			name:     "Server ID flag only",
			flags:    map[string]string{"server-id": "my-server"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &components.Context{}
			for flag, value := range tt.flags {
				ctx.AddStringFlag(flag, value)
			}

			result := ide.HasServerConfigFlags(ctx)
			assert.Equal(t, tt.expected, result, "Test case: %s", tt.name)
		})
	}
}
