// Unit test for extractRepoName function
package npm

import (
	"testing"
)

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid URL",
			input:       "https://example.com/artifactory/repo-name",
			expected:    "repo-name",
			expectError: false,
		},
		{
			name:        "Valid URL",
			input:       "example.com/artifactory/repo-name",
			expected:    "repo-name",
			expectError: false,
		},
		{
			name:        "Empty URL",
			input:       "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Invalid URL with no parts",
			input:       "https://",
			expected:    "",
			expectError: true,
		},
		{
			name:        "URL with trailing slash",
			input:       "https://example.com/artifactory/repo-name/",
			expected:    "repo-name",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractRepoName(tt.input)
			if (err != nil) != tt.expectError {
				t.Errorf("extractRepoName(%q) error = %v, expectError %v", tt.input, err, tt.expectError)
				return
			}
			if !tt.expectError && result != tt.expected {
				t.Errorf("extractRepoName(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}
