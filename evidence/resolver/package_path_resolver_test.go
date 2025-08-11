package resolver

import (
	"fmt"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/metadata"
	"github.com/stretchr/testify/assert"
)

type mockArtifactoryServicesManagerRepo struct {
	artifactory.EmptyArtifactoryServicesManager
	repoType string
}

func (m *mockArtifactoryServicesManagerRepo) GetRepository(_ string, out interface{}) error {
	if d, ok := out.(*services.RepositoryDetails); ok {
		d.PackageType = m.repoType
	}
	return nil
}

func (m *mockArtifactoryServicesManagerRepo) GetPackageLeadFile(_ services.LeadFileParams) ([]byte, error) {
	return nil, fmt.Errorf("not found")
}

type mockMetadataManager struct {
	metadata.Manager
	body []byte
	err  error
}

func (m *mockMetadataManager) GraphqlQuery(_ []byte) ([]byte, error) { return m.body, m.err }

func TestPackagePathResolver(t *testing.T) {
	sd := &config.ServerDetails{Url: "url"}
	r, err := NewPackagePathResolver("pkg", "1.0.0", "repo", sd)
	assert.NoError(t, err)

	r.artifactoryClient = &mockArtifactoryServicesManagerRepo{repoType: "nuget"}
	r.metadataClient = &mockMetadataManager{body: []byte(`{"data":{"versions":{"edges":[{"node":{"repos":[{"name":"repo","leadFilePath":"pkg/1.0.0/pkg-1.0.0.nupkg"}]}}]}}}`)}
	r.packageService = evidence.NewPackageService("pkg", "1.0.0", "repo")

	got, err := r.ResolveSubjectRepoPath()
	assert.NoError(t, err)
	assert.Equal(t, "repo/pkg/1.0.0/pkg-1.0.0.nupkg", got)
}
