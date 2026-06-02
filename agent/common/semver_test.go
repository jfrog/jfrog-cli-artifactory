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

func TestCompareSemver(t *testing.T) {
	t.Parallel()
	comparison, err := CompareSemver("1.0.0", "1.0.5")
	require.NoError(t, err)
	assert.Less(t, comparison, 0)

	comparison, err = CompareSemver("1.0.5", "1.0.5")
	require.NoError(t, err)
	assert.Zero(t, comparison)

	comparison, err = CompareSemver("2.0.0", "1.9.9")
	require.NoError(t, err)
	assert.Greater(t, comparison, 0)

	_, err = CompareSemver("notsemver", "1.0.0")
	assert.Error(t, err)
}

func TestValidateSemver(t *testing.T) {
	valid := []string{"1.0.0", "1.2.3-rc.1", "2.3.4-beta", "0.1.0+build.1", "v2.0.0", "1.0.0+build.123"}
	for _, version := range valid {
		assert.NoError(t, ValidateSemver(version), "version %q should be valid", version)
	}
	invalid := []string{
		"", "..", "1.0/.0", "not-a-version", "1.0..0", "../etc/passwd",
		"1.0.0/../../etc", "valid..version", "has space", "/leading-slash", "-leading-hyphen",
	}
	for _, version := range invalid {
		assert.Error(t, ValidateSemver(version), "version %q should be invalid", version)
	}
}

func TestNextMinorVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{name: "basic", input: "1.2.3", expected: "1.3.0"},
		{name: "zero minor", input: "2.0.0", expected: "2.1.0"},
		{name: "high minor", input: "0.99.5", expected: "0.100.0"},
		{name: "with v prefix", input: "v1.0.0", expected: "1.1.0"},
		{name: "invalid", input: "not-a-version", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NextMinorVersion(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
