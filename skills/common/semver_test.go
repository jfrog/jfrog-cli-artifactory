package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple ordering",
			versions: []string{"1.0.0", "2.0.0", "1.5.0"},
			expected: "2.0.0",
		},
		{
			name:     "patch ordering",
			versions: []string{"1.0.1", "1.0.3", "1.0.2"},
			expected: "1.0.3",
		},
		{
			name:     "minor ordering",
			versions: []string{"1.2.0", "1.10.0", "1.3.0"},
			expected: "1.10.0",
		},
		{
			name:     "single version",
			versions: []string{"1.0.0"},
			expected: "1.0.0",
		},
		{
			name:     "empty list",
			versions: []string{},
			wantErr:  true,
		},
		{
			name:     "with invalid versions",
			versions: []string{"invalid", "1.0.0", "also-invalid"},
			expected: "1.0.0",
		},
		{
			name:     "all invalid",
			versions: []string{"invalid", "nope"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := LatestVersion(tt.versions)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
