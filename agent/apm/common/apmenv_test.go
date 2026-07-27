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
			t.Setenv("HOME", home)
			if tt.configJSON != "" {
				apmDir := filepath.Join(home, ".apm")
				require.NoError(t, os.MkdirAll(apmDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(apmDir, "config.json"), []byte(tt.configJSON), 0o644))
			}

			sd := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}
			assert.Equal(t, tt.want, ResolveRepoNameFromRegistry(sd, ""))
		})
	}
}

func TestResolveRepoNameFromRegistry_NilServerDetails(t *testing.T) {
	assert.Empty(t, ResolveRepoNameFromRegistry(nil, ""))
}
