package civcs

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCIVcsPropsString(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "not in CI",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "GitHub Actions with all fields",
			envVars: map[string]string{
				"CI":                        "true",
				"GITHUB_ACTIONS":            "true",
				"GITHUB_WORKFLOW":           "test",
				"GITHUB_RUN_ID":             "123",
				"GITHUB_REPOSITORY_OWNER":   "myorg",
				"GITHUB_REPOSITORY":         "myorg/myrepo",
			},
			expected: "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name: "CI without GitHub Actions",
			envVars: map[string]string{
				"CI": "true",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear CI-related env vars
			clearCIEnvVars()
			defer clearCIEnvVars()

			// Set test env vars
			for k, v := range tt.envVars {
				_ = os.Setenv(k, v)
			}

			result := GetCIVcsPropsString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeWithUserProps(t *testing.T) {
	tests := []struct {
		name      string
		userProps string
		ciProps   string
		expected  string
	}{
		{
			name:      "no user props, no CI props",
			userProps: "",
			ciProps:   "",
			expected:  "",
		},
		{
			name:      "user props only, no CI",
			userProps: "foo=bar",
			ciProps:   "",
			expected:  "foo=bar",
		},
		{
			name:      "CI props only, no user props",
			userProps: "",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name:      "both user and CI props",
			userProps: "foo=bar",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "foo=bar;vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name:      "user already has vcs.provider - adds other CI props",
			userProps: "vcs.provider=custom",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "vcs.provider=custom;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name:      "user already has vcs.org - adds other CI props",
			userProps: "foo=bar;vcs.org=customorg",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "foo=bar;vcs.org=customorg;vcs.provider=github;vcs.repo=myrepo",
		},
		{
			name:      "user has all vcs props - no CI props added",
			userProps: "vcs.provider=custom;vcs.org=customorg;vcs.repo=customrepo",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "vcs.provider=custom;vcs.org=customorg;vcs.repo=customrepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set CI env vars based on ciProps
			clearCIEnvVars()
			defer clearCIEnvVars()

			if tt.ciProps != "" {
				// Setup GitHub Actions environment
				_ = os.Setenv("CI", "true")
				_ = os.Setenv("GITHUB_ACTIONS", "true")
				_ = os.Setenv("GITHUB_WORKFLOW", "test")
				_ = os.Setenv("GITHUB_RUN_ID", "123")
				_ = os.Setenv("GITHUB_REPOSITORY_OWNER", "myorg")
				_ = os.Setenv("GITHUB_REPOSITORY", "myorg/myrepo")
			}

			result := MergeWithUserProps(tt.userProps)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func clearCIEnvVars() {
	envVars := []string{
		"CI",
		"GITHUB_ACTIONS",
		"GITHUB_WORKFLOW",
		"GITHUB_RUN_ID",
		"GITHUB_REPOSITORY_OWNER",
		"GITHUB_REPOSITORY",
		"GITLAB_CI",
		"CI_PROJECT_PATH",
	}
	for _, v := range envVars {
		_ = os.Unsetenv(v)
	}
}
