package evidence

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

func (m *mockArtifactoryServicesManagerBuild) FileInfo(relativePath string) (*utils.FileInfo, error) {
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
		name         string
		project      string
		build        string
		expectedPath string
		expectError  bool
	}{
		{
			name:         "Valid build with project",
			project:      "myProject",
			build:        "buildName:1",
			expectedPath: "myProject-build-info/buildName/1-1705529045000.json@dummy_sha256",
			expectError:  false,
		},
		{
			name:         "Valid build default project",
			project:      "default",
			build:        "buildName:1",
			expectedPath: "artifactory-build-info/buildName/1-1705529045000.json@dummy_sha256",
			expectError:  false,
		},
		{
			name:         "Invalid build format",
			project:      "myProject",
			build:        "buildName-1",
			expectedPath: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &createEvidenceBuild{
				project: tt.project,
				build:   tt.build,
			}
			aa := &mockArtifactoryServicesManagerBuild{}
			path, err := c.buildBuildInfoSubjectPath(aa)
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
