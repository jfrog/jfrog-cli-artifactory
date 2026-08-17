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
