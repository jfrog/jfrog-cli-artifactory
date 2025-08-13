package delete

import (
	"errors"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
)

type mockResolver struct {
	path string
	err  error
}

func (m *mockResolver) ResolveSubjectRepoPath() (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.path, nil
}

func TestDeleteEvidenceHandler_CommandName_And_ServerDetails(t *testing.T) {
	sd := &config.ServerDetails{Url: "http://test.com"}
	h := NewDeleteEvidenceBase(sd, "evd-name", &mockResolver{path: "repo/path"})

	gotSd, err := h.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, sd, gotSd)
	assert.Equal(t, "delete-evidence", h.CommandName())
}

func TestDeleteEvidenceHandler_Run_PathResolverError(t *testing.T) {
	sd := &config.ServerDetails{Url: "http://test.com"}
	h := NewDeleteEvidenceBase(sd, "evd-name", &mockResolver{err: errors.New("resolve failed")})

	err := h.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resolve failed")
}
