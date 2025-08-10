package resolver

import (
	"bytes"
	"io"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
)

type mockArtifactoryServicesManagerAql struct {
	artifactory.EmptyArtifactoryServicesManager
	rc  io.ReadCloser
	err error
}

func (m *mockArtifactoryServicesManagerAql) Aql(_ string) (io.ReadCloser, error) {
	return m.rc, m.err
}

func TestBuildPathResolver_Success(t *testing.T) {
	sd := &config.ServerDetails{Url: "http://x"}
	r, err := NewBuildPathResolver("proj", "bname", "bnum", sd)
	assert.NoError(t, err)

	r.artifactoryClient = &mockArtifactoryServicesManagerAql{
		rc: io.NopCloser(bytes.NewBufferString(`{"results":[{"name":"bnum-123"}]}`)),
	}

	expected := utils.BuildBuildInfoRepoKey("proj") + "/" + "bname" + "/" + "bnum-123"
	got, err := r.ResolveSubjectRepoPath()
	assert.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestBuildPathResolver_EmptyResults(t *testing.T) {
	sd := &config.ServerDetails{Url: "http://x"}
	r, err := NewBuildPathResolver("proj", "bname", "bnum", sd)
	assert.NoError(t, err)

	r.artifactoryClient = &mockArtifactoryServicesManagerAql{
		rc: io.NopCloser(bytes.NewBufferString(`{"results":[]}`)),
	}
	_, err = r.ResolveSubjectRepoPath()
	assert.Error(t, err)
}
