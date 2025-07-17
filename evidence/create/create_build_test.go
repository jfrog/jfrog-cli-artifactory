package create

import (
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

import (
	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
)

type mockArtifactoryServicesManagerBuild struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockArtifactoryServicesManagerBuild) FileInfo(_ string) (*utils.FileInfo, error) {
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

func (m *mockArtifactoryServicesManagerBuild) GetBuildInfo(services.BuildInfoParams) (*buildinfo.PublishedBuildInfo, bool, error) {
	buildInfo := &buildinfo.PublishedBuildInfo{
		BuildInfo: buildinfo.BuildInfo{
			Started: "2024-01-17T15:04:05.000-0700",
		},
	}
	return buildInfo, true, nil
}

func TestBuildInfo(t *testing.T) {
	tests := []struct {
		name             string
		project          string
		buildName        string
		buildNumber      string
		expectedPath     string
		expectedChecksum string
		expectError      bool
	}{
		{
			name:             "Valid buildName with project",
			project:          "myProject",
			buildName:        "buildName",
			buildNumber:      "1",
			expectedPath:     "myProject-build-info/buildName/1-1705529045000.json",
			expectedChecksum: "dummy_sha256",
			expectError:      false,
		},
		{
			name:             "Valid buildName default project",
			project:          "default",
			buildName:        "buildName",
			buildNumber:      "1",
			expectedPath:     "artifactory-build-info/buildName/1-1705529045000.json",
			expectedChecksum: "dummy_sha256",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &createEvidenceBuild{
				project:     tt.project,
				buildName:   tt.buildName,
				buildNumber: tt.buildNumber,
			}
			aa := &mockArtifactoryServicesManagerBuild{}
			path, sha256, err := c.buildBuildInfoSubjectPath(aa)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, path)
				assert.Empty(t, sha256)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
				assert.Equal(t, tt.expectedChecksum, sha256)
			}
		})
	}
}
