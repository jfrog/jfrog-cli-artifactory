package conan

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
)

func TestExtractRemoteName(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "Extract with -r flag",
			args:     []string{"package/1.0", "-r", "my-remote", "--confirm"},
			expected: "my-remote",
		},
		{
			name:     "Extract with --remote flag",
			args:     []string{"package/1.0", "--remote", "another-remote"},
			expected: "another-remote",
		},
		{
			name:     "Extract with -r= format",
			args:     []string{"package/1.0", "-r=inline-remote"},
			expected: "inline-remote",
		},
		{
			name:     "Extract with --remote= format",
			args:     []string{"package/1.0", "--remote=inline-remote2"},
			expected: "inline-remote2",
		},
		{
			name:     "No remote specified",
			args:     []string{"package/1.0", "--confirm"},
			expected: "",
		},
		{
			name:     "Empty args",
			args:     []string{},
			expected: "",
		},
		{
			name:     "-r flag at end without value",
			args:     []string{"package/1.0", "-r"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractRemoteName(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		expected  string
	}{
		{
			name:      "Standard Artifactory URL with api/conan path",
			remoteURL: "https://myserver.jfrog.io/artifactory/api/conan/conan-local",
			expected:  "https://myserver.jfrog.io",
		},
		{
			name:      "Artifactory URL without trailing path",
			remoteURL: "https://myserver.jfrog.io/artifactory/",
			expected:  "https://myserver.jfrog.io",
		},
		{
			name:      "Artifactory URL without trailing slash",
			remoteURL: "https://myserver.jfrog.io/artifactory",
			expected:  "https://myserver.jfrog.io",
		},
		{
			name:      "Platform URL only",
			remoteURL: "https://myserver.jfrog.io/",
			expected:  "https://myserver.jfrog.io",
		},
		{
			name:      "Platform URL without slash",
			remoteURL: "https://myserver.jfrog.io",
			expected:  "https://myserver.jfrog.io",
		},
		{
			name:      "Self-hosted Artifactory",
			remoteURL: "https://artifactory.company.com/artifactory/api/conan/conan-repo",
			expected:  "https://artifactory.company.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBaseURL(tt.remoteURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with trailing slash",
			input:    "https://example.com/",
			expected: "https://example.com",
		},
		{
			name:     "URL without trailing slash",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "URL with uppercase",
			input:    "HTTPS://EXAMPLE.COM/",
			expected: "https://example.com",
		},
		{
			name:     "Mixed case URL",
			input:    "https://MyServer.JFrog.IO/",
			expected: "https://myserver.jfrog.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesServer(t *testing.T) {
	tests := []struct {
		name             string
		serverDetails    *config.ServerDetails
		normalizedTarget string
		expected         bool
	}{
		{
			name: "Match by Artifactory URL",
			serverDetails: &config.ServerDetails{
				ArtifactoryUrl: "https://myserver.jfrog.io/artifactory/",
			},
			normalizedTarget: "https://myserver.jfrog.io",
			expected:         true,
		},
		{
			name: "Match by Platform URL",
			serverDetails: &config.ServerDetails{
				Url: "https://myserver.jfrog.io/",
			},
			normalizedTarget: "https://myserver.jfrog.io",
			expected:         true,
		},
		{
			name: "No match - different server",
			serverDetails: &config.ServerDetails{
				ArtifactoryUrl: "https://other-server.jfrog.io/artifactory/",
			},
			normalizedTarget: "https://myserver.jfrog.io",
			expected:         false,
		},
		{
			name: "Match with both URLs set - Artifactory matches",
			serverDetails: &config.ServerDetails{
				Url:            "https://other.jfrog.io/",
				ArtifactoryUrl: "https://myserver.jfrog.io/artifactory/",
			},
			normalizedTarget: "https://myserver.jfrog.io",
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesServer(tt.serverDetails, tt.normalizedTarget)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCredentials(t *testing.T) {
	tests := []struct {
		name             string
		serverDetails    *config.ServerDetails
		expectedUsername string
		expectedPassword string
		expectError      bool
	}{
		{
			name: "Extract with access token",
			serverDetails: &config.ServerDetails{
				User:        "myuser",
				AccessToken: "my-access-token",
			},
			expectedUsername: "myuser",
			expectedPassword: "my-access-token",
			expectError:      false,
		},
		{
			name: "Extract with access token but no user",
			serverDetails: &config.ServerDetails{
				AccessToken: "my-access-token",
			},
			expectedUsername: "admin", // Defaults to "admin" when no user specified
			expectedPassword: "my-access-token",
			expectError:      false,
		},
		{
			name: "Extract with password",
			serverDetails: &config.ServerDetails{
				User:     "myuser",
				Password: "mypassword",
			},
			expectedUsername: "myuser",
			expectedPassword: "mypassword",
			expectError:      false,
		},
		{
			name: "Prefer password over access token",
			serverDetails: &config.ServerDetails{
				User:        "myuser",
				Password:    "mypassword",
				AccessToken: "my-access-token",
			},
			expectedUsername: "myuser",
			expectedPassword: "mypassword", // Password is preferred for Conan (API keys work more reliably)
			expectError:      false,
		},
		{
			name: "No credentials",
			serverDetails: &config.ServerDetails{
				ServerId: "test-server",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, password, err := extractCredentials(tt.serverDetails)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "no credentials")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUsername, username)
				assert.Equal(t, tt.expectedPassword, password)
			}
		})
	}
}

func TestFormatServerIDs(t *testing.T) {
	tests := []struct {
		name     string
		configs  []*config.ServerDetails
		expected string
	}{
		{
			name: "Single server",
			configs: []*config.ServerDetails{
				{ServerId: "server1"},
			},
			expected: "server1",
		},
		{
			name: "Multiple servers",
			configs: []*config.ServerDetails{
				{ServerId: "server1"},
				{ServerId: "server2"},
				{ServerId: "server3"},
			},
			expected: "server1, server2, server3",
		},
		{
			name:     "Empty list",
			configs:  []*config.ServerDetails{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatServerIDs(tt.configs)
			assert.Equal(t, tt.expected, result)
		})
	}
}
