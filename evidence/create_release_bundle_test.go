package evidence

import (
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

type mockArtifactoryServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockArtifactoryServicesManager) FileInfo(relativePath string) (*utils.FileInfo, error) {
	fi := &utils.FileInfo{
		Checksums: struct {
			Sha1   string `json:"sha1,omitempty"`
			Sha256 string `json:"sha256,omitempty"`
			Md5    string `json:"md5,omitempty"`
		}{
			Sha256: "dummy_sha256",
		},
	}
	return fi, nil
}

func TestReleaseBundle(t *testing.T) {
	tests := []struct {
		name          string
		project       string
		releaseBundle string
		expectedPath  string
		expectError   bool
	}{
		{
			name:          "Valid release bundle with project",
			project:       "myProject",
			releaseBundle: "bundleName:1.0.0",
			expectedPath:  "myProject-release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd@dummy_sha256",
			expectError:   false,
		},
		{
			name:          "Valid release bundle default project",
			project:       "default",
			releaseBundle: "bundleName:1.0.0",
			expectedPath:  "release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd@dummy_sha256",
			expectError:   false,
		},
		{
			name:          "Valid release bundle empty project",
			project:       "default",
			releaseBundle: "bundleName:1.0.0",
			expectedPath:  "release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd@dummy_sha256",
			expectError:   false,
		},
		{
			name:          "Invalid release bundle format 1",
			project:       "myProject",
			releaseBundle: "bundleName:1.0.0:111",
			expectedPath:  "",
			expectError:   true,
		},
		{
			name:          "Invalid release bundle format 2",
			project:       "myProject",
			releaseBundle: "bundleName111",
			expectedPath:  "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &createEvidenceReleaseBundle{
				project:       tt.project,
				releaseBundle: tt.releaseBundle,
			}
			aa := &mockArtifactoryServicesManager{}
			path, err := c.buildReleaseBundleSubjectPath(aa)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, path)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
			}
		})
	}
}
