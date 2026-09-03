package apmcommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeApmEnvName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "hyphen", in: "corp-main", want: "CORP_MAIN"},
		{name: "dot", in: "corp.main", want: "CORP_MAIN"},
		{name: "already uppercase with hyphen", in: "Corp-Main", want: "CORP_MAIN"},
		{name: "no special chars", in: "corpmain", want: "CORPMAIN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeApmEnvName(tt.in))
		})
	}
}

func TestCheckSanitizationCollisions(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		wantErr bool
	}{
		{name: "no collision", names: []string{"corp-main", "other-repo"}, wantErr: false},
		{
			name:    "collision - hyphen vs dot vs case",
			names:   []string{"corp-main", "corp.main", "Corp-Main"},
			wantErr: true,
		},
		{name: "single name", names: []string{"corp-main"}, wantErr: false},
		{name: "empty", names: nil, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSanitizationCollisions(tt.names)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApmEnvVarNames(t *testing.T) {
	assert.Equal(t, "APM_REGISTRY_TOKEN_CORP_MAIN", apmTokenEnvVar("corp-main"))
	assert.Equal(t, "APM_REGISTRY_USER_CORP_MAIN", apmUserEnvVar("corp-main"))
	assert.Equal(t, "APM_REGISTRY_PASS_CORP_MAIN", apmPassEnvVar("corp-main"))
}

func TestResolveRepoNameFromRegistry(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       string
	}{
		{
			name:       "single matching registry resolves to its name",
			configJSON: `{"registries":{"buk-apm":{"url":"https://acme.jfrog.io/artifactory/api/agentpackages/buk-apm/"}}}`,
			want:       "buk-apm",
		},
		{
			name:       "no matching registry returns empty",
			configJSON: `{"registries":{"other":{"url":"https://different.jfrog.io/artifactory/api/agentpackages/other/"}}}`,
			want:       "",
		},
		{
			name:       "multiple matching registries is ambiguous - returns empty rather than guessing",
			configJSON: `{"registries":{"a":{"url":"https://acme.jfrog.io/artifactory/api/agentpackages/a/"},"b":{"url":"https://acme.jfrog.io/artifactory/api/agentpackages/b/"}}}`,
			want:       "",
		},
		{
			name:       "no config file at all returns empty",
			configJSON: "",
			want:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			// Set HOME for Unix and USERPROFILE for Windows (os.UserHomeDir reads USERPROFILE on Windows)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			if tt.configJSON != "" {
				apmDir := filepath.Join(home, ".apm")
				require.NoError(t, os.MkdirAll(apmDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(apmDir, "config.json"), []byte(tt.configJSON), 0o644))
			}

			sd := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}
			assert.Equal(t, tt.want, ResolveRepoNameFromRegistry(sd, "", nil))
		})
	}
}

func TestResolveRepoNameFromRegistry_NilServerDetails(t *testing.T) {
	assert.Empty(t, ResolveRepoNameFromRegistry(nil, "", nil))
}

func TestResolveRepoNameFromRegistry_ExplicitRegistryFlag(t *testing.T) {
	// Two registries share the same host (would be ambiguous for the old host-matching
	// heuristic), but an explicit --registry picks one unambiguously.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	apmDir := filepath.Join(home, ".apm")
	require.NoError(t, os.MkdirAll(apmDir, 0o755))
	configJSON := `{"registries":{
		"a":{"url":"https://acme.jfrog.io/artifactory/api/agentpackages/a-repo/"},
		"b":{"url":"https://acme.jfrog.io/artifactory/api/agentpackages/b-repo/","default":true}
	}}`
	require.NoError(t, os.WriteFile(filepath.Join(apmDir, "config.json"), []byte(configJSON), 0o644))

	sd := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}

	// --registry a wins over b's "default": true.
	assert.Equal(t, "a-repo", ResolveRepoNameFromRegistry(sd, "", []string{"--package", "acme/pkg", "--registry", "a"}))
	// --registry=b (equals form) also works.
	assert.Equal(t, "b-repo", ResolveRepoNameFromRegistry(sd, "", []string{"--registry=b"}))
	// Unknown --registry name falls back to host-matching, which is ambiguous here (2 matches).
	assert.Equal(t, "", ResolveRepoNameFromRegistry(sd, "", []string{"--registry", "unknown"}))
}

func TestResolveRepoNameFromRegistry_DefaultRegistryNoFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	apmDir := filepath.Join(home, ".apm")
	require.NoError(t, os.MkdirAll(apmDir, 0o755))
	configJSON := `{"registries":{
		"a":{"url":"https://acme.jfrog.io/artifactory/api/agentpackages/a-repo/"},
		"b":{"url":"https://acme.jfrog.io/artifactory/api/agentpackages/b-repo/","default":true}
	}}`
	require.NoError(t, os.WriteFile(filepath.Join(apmDir, "config.json"), []byte(configJSON), 0o644))

	sd := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}

	// No --registry flag: falls back to whichever registry has "default": true.
	assert.Equal(t, "b-repo", ResolveRepoNameFromRegistry(sd, "", []string{"--package", "acme/pkg"}))
}

func TestRepoKeyFromRegistryURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "trailing slash", url: "https://acme.jfrog.io/artifactory/api/agentpackages/my-repo/", want: "my-repo"},
		{name: "no trailing slash", url: "https://acme.jfrog.io/artifactory/api/agentpackages/my-repo", want: "my-repo"},
		{name: "virtual package sub-path ignored", url: "https://acme.jfrog.io/artifactory/api/agentpackages/my-repo/some/subpath", want: "my-repo"},
		{name: "missing prefix", url: "https://acme.jfrog.io/artifactory/api/other/my-repo/", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, repoKeyFromRegistryURL(tt.url))
		})
	}
}

func TestRegistryNameFromArgs(t *testing.T) {
	assert.Equal(t, "corp-main", registryNameFromArgs([]string{"--package", "acme/pkg", "--registry", "corp-main"}))
	assert.Equal(t, "corp-main", registryNameFromArgs([]string{"--registry=corp-main"}))
	assert.Equal(t, "", registryNameFromArgs([]string{"--package", "acme/pkg"}))
	assert.Equal(t, "", registryNameFromArgs(nil))
}

func TestBuildRegistryEntry(t *testing.T) {
	tests := []struct {
		name          string
		serverDetails *config.ServerDetails
		repoName      string
		expectURL     string
		expectToken   string
	}{
		{
			name: "access token takes priority",
			serverDetails: &config.ServerDetails{
				ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
				AccessToken:    "my-existing-token",
				User:           "admin",
				Password:       "password",
			},
			repoName:    "apm-local",
			expectURL:   "https://acme.jfrog.io/artifactory/api/agentpackages/apm-local/",
			expectToken: "my-existing-token",
		},
		{
			name: "no auth returns URL only",
			serverDetails: &config.ServerDetails{
				ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
			},
			repoName:    "apm-local",
			expectURL:   "https://acme.jfrog.io/artifactory/api/agentpackages/apm-local/",
			expectToken: "",
		},
		{
			name: "user without password returns URL only",
			serverDetails: &config.ServerDetails{
				ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
				User:           "admin",
			},
			repoName:    "apm-local",
			expectURL:   "https://acme.jfrog.io/artifactory/api/agentpackages/apm-local/",
			expectToken: "",
		},
		{
			name: "trailing slash handling",
			serverDetails: &config.ServerDetails{
				ArtifactoryUrl: "https://acme.jfrog.io/artifactory",
				AccessToken:    "token",
			},
			repoName:    "apm-local",
			expectURL:   "https://acme.jfrog.io/artifactory/api/agentpackages/apm-local/",
			expectToken: "token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, token, err := BuildRegistryEntry(tt.serverDetails, tt.repoName)
			require.NoError(t, err)
			assert.Equal(t, tt.expectURL, url)
			assert.Equal(t, tt.expectToken, token)
		})
	}
}

func TestGenerateAccessToken_NoAuth(t *testing.T) {
	tests := []struct {
		name          string
		serverDetails *config.ServerDetails
	}{
		{
			name:          "empty user and password",
			serverDetails: &config.ServerDetails{},
		},
		{
			name: "user without password",
			serverDetails: &config.ServerDetails{
				User: "admin",
			},
		},
		{
			name: "password without user",
			serverDetails: &config.ServerDetails{
				Password: "secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := generateAccessToken(tt.serverDetails)
			assert.Error(t, err, "generateAccessToken should error for incomplete credentials")
			assert.Empty(t, token, "generateAccessToken should return empty token for incomplete credentials")
		})
	}
}

func TestIsDryRunArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "--dry-run present", args: []string{"--package", "jfrog/proj3", "--dry-run"}, want: true},
		{name: "--dry-run present, different position", args: []string{"--dry-run", "--package", "jfrog/proj3"}, want: true},
		{name: "no --dry-run", args: []string{"--package", "jfrog/proj3"}, want: false},
		{name: "empty args", args: []string{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsDryRunArg(tt.args))
		})
	}
}
