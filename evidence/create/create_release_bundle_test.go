package create

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

import (
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
)

type mockReleaseBundleArtifactoryServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockReleaseBundleArtifactoryServicesManager) FileInfo(_ string) (*utils.FileInfo, error) {
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
		name                 string
		project              string
		releaseBundle        string
		releaseBundleVersion string
		expectedPath         string
		expectedCheckSum     string
		expectError          bool
	}{
		{
			name:                 "Valid release bundle with project",
			project:              "myProject",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "myProject-release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectError:          false,
		},
		{
			name:                 "Valid release bundle default project",
			project:              "default",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectError:          false,
		},
		{
			name:                 "Valid release bundle empty project",
			project:              "default",
			releaseBundle:        "bundleName",
			releaseBundleVersion: "1.0.0",
			expectedPath:         "release-bundles-v2/bundleName/1.0.0/release-bundle.json.evd",
			expectedCheckSum:     "dummy_sha256",
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &createEvidenceReleaseBundle{
				project:              tt.project,
				releaseBundle:        tt.releaseBundle,
				releaseBundleVersion: tt.releaseBundleVersion,
			}
			aa := &mockReleaseBundleArtifactoryServicesManager{}
			path, sha256, err := c.buildReleaseBundleSubjectPath(aa)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, sha256)
				assert.Empty(t, path)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
				assert.Equal(t, tt.expectedCheckSum, sha256)
			}
		})
	}
}
