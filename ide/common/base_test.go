package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitApiURL(t *testing.T) {
	tests := []struct {
		name        string
		rawURL      string
		apiType     string
		wantBase    string
		wantRepoKey string
		wantOk      bool
	}{
		{
			name:        "with repo only",
			rawURL:      "https://acme.jfrog.io/artifactory/api/aieditorextensions/repo",
			apiType:     "aieditorextensions",
			wantBase:    "https://acme.jfrog.io/artifactory",
			wantRepoKey: "repo",
			wantOk:      true,
		},
		{
			name:        "with trailing slash",
			rawURL:      "https://acme.jfrog.io/artifactory/api/aieditorextensions/repo/",
			apiType:     "aieditorextensions",
			wantBase:    "https://acme.jfrog.io/artifactory",
			wantRepoKey: "repo",
			wantOk:      true,
		},
		{
			name:        "with gallery suffix",
			rawURL:      "https://acme.jfrog.io/artifactory/api/aieditorextensions/repo/_apis/public/gallery",
			apiType:     "aieditorextensions",
			wantBase:    "https://acme.jfrog.io/artifactory",
			wantRepoKey: "repo",
			wantOk:      true,
		},
		{
			name:        "with token suffix",
			rawURL:      "https://acme.jfrog.io/artifactory/api/aieditorextensions/repo/_apis/public/gallery/TOKEN",
			apiType:     "aieditorextensions",
			wantBase:    "https://acme.jfrog.io/artifactory",
			wantRepoKey: "repo",
			wantOk:      true,
		},
		{
			name:    "wrong apiType",
			rawURL:  "https://acme.jfrog.io/artifactory/api/vscode/repo",
			apiType: "aieditorextensions",
			wantOk:  false,
		},
		{
			name:    "base only",
			rawURL:  "https://acme.jfrog.io/artifactory",
			apiType: "aieditorextensions",
			wantOk:  false,
		},
		{
			name:    "no api segment",
			rawURL:  "https://acme.jfrog.io/artifactory/aieditorextensions/repo",
			apiType: "aieditorextensions",
			wantOk:  false,
		},
		{
			name:        "with query string is stripped from repo key",
			rawURL:      "https://acme.jfrog.io/artifactory/api/aieditorextensions/repo?source=setup",
			apiType:     "aieditorextensions",
			wantBase:    "https://acme.jfrog.io/artifactory",
			wantRepoKey: "repo",
			wantOk:      true,
		},
		{
			name:        "with fragment is stripped from repo key",
			rawURL:      "https://acme.jfrog.io/artifactory/api/aieditorextensions/repo#anchor",
			apiType:     "aieditorextensions",
			wantBase:    "https://acme.jfrog.io/artifactory",
			wantRepoKey: "repo",
			wantOk:      true,
		},
		{
			name:        "with query string after gallery",
			rawURL:      "https://acme.jfrog.io/artifactory/api/aieditorextensions/repo/_apis/public/gallery?x=1",
			apiType:     "aieditorextensions",
			wantBase:    "https://acme.jfrog.io/artifactory",
			wantRepoKey: "repo",
			wantOk:      true,
		},
		{
			name:    "not a URL (no scheme)",
			rawURL:  "acme.jfrog.io/artifactory/api/aieditorextensions/repo",
			apiType: "aieditorextensions",
			wantOk:  false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base, key, ok := SplitApiURL(tc.rawURL, tc.apiType)
			assert.Equal(t, tc.wantOk, ok)
			if tc.wantOk {
				assert.Equal(t, tc.wantBase, base)
				assert.Equal(t, tc.wantRepoKey, key)
			}
		})
	}
}

func TestURLsHaveSameHost(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", "https://acme.jfrog.io/artifactory", "https://acme.jfrog.io/artifactory", true},
		{"same host different path", "https://acme.jfrog.io/artifactory", "https://acme.jfrog.io/other", true},
		{"case-insensitive host", "https://ACME.jfrog.io/artifactory", "https://acme.jfrog.io/artifactory", true},
		{"different host", "https://staging.jfrog.io/artifactory", "https://production.jfrog.io/artifactory", false},
		{"empty a", "", "https://acme.jfrog.io/artifactory", false},
		{"empty b", "https://acme.jfrog.io/artifactory", "", false},
		{"invalid a", "not a url", "https://acme.jfrog.io/artifactory", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, URLsHaveSameHost(tc.a, tc.b))
		})
	}
}
