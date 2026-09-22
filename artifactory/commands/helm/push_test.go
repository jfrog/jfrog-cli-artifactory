package helm

import (
	"errors"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockRepositoryServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	mock.Mock
}

func (m *mockRepositoryServicesManager) GetRepository(repoKey string, repoDetails interface{}) error {
	args := m.Called(repoKey, repoDetails)
	return args.Error(0)
}

func TestResolvePropertiesRepository(t *testing.T) {
	tests := []struct {
		name        string
		repoName    string
		setupMock   func(m *mockRepositoryServicesManager)
		expectedRes string
	}{
		{
			name:     "local repository is used as-is",
			repoName: "helm-local",
			setupMock: func(m *mockRepositoryServicesManager) {
				m.On("GetRepository", "helm-local", mock.Anything).Run(func(args mock.Arguments) {
					ptr := args.Get(1).(**ocicontainer.DockerRepositoryDetails)
					*ptr = &ocicontainer.DockerRepositoryDetails{Key: "helm-local", RepoType: "local"}
				}).Return(nil)
			},
			expectedRes: "helm-local",
		},
		{
			name:     "virtual repository resolves to its default deployment repository",
			repoName: "helm-virtual",
			setupMock: func(m *mockRepositoryServicesManager) {
				m.On("GetRepository", "helm-virtual", mock.Anything).Run(func(args mock.Arguments) {
					ptr := args.Get(1).(**ocicontainer.DockerRepositoryDetails)
					*ptr = &ocicontainer.DockerRepositoryDetails{
						Key:                   "helm-virtual",
						RepoType:              "virtual",
						DefaultDeploymentRepo: "helm-local",
					}
				}).Return(nil)
			},
			expectedRes: "helm-local",
		},
		{
			name:     "virtual repository without a default deployment repository skips property setting",
			repoName: "helm-virtual",
			setupMock: func(m *mockRepositoryServicesManager) {
				m.On("GetRepository", "helm-virtual", mock.Anything).Run(func(args mock.Arguments) {
					ptr := args.Get(1).(**ocicontainer.DockerRepositoryDetails)
					*ptr = &ocicontainer.DockerRepositoryDetails{Key: "helm-virtual", RepoType: "virtual"}
				}).Return(nil)
			},
			expectedRes: "",
		},
		{
			name:     "repository lookup failure falls back to the original repo name",
			repoName: "helm-repo",
			setupMock: func(m *mockRepositoryServicesManager) {
				m.On("GetRepository", "helm-repo", mock.Anything).Return(errors.New("connection refused"))
			},
			expectedRes: "helm-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSM := new(mockRepositoryServicesManager)
			tt.setupMock(mockSM)

			result := resolvePropertiesRepository(mockSM, tt.repoName)

			assert.Equal(t, tt.expectedRes, result)
			mockSM.AssertExpectations(t)
		})
	}
}
