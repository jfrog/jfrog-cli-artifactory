package npm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePackageJSON_WorkspacesArray(t *testing.T) {
	p, err := parsePackageJSON([]byte(`{"workspaces":["packages/*"]}`))
	require.NoError(t, err)
	assert.True(t, p.hasWorkspaces())
}

func TestParsePackageJSON_WorkspacesObject(t *testing.T) {
	p, err := parsePackageJSON([]byte(`{"workspaces":{"packages":["apps/*"],"nohoist":["**/react-native"]}}`))
	require.NoError(t, err)
	assert.True(t, p.hasWorkspaces())
}

func TestParsePackageJSON_NoWorkspaces(t *testing.T) {
	p, err := parsePackageJSON([]byte(`{"name":"app"}`))
	require.NoError(t, err)
	assert.False(t, p.hasWorkspaces())
}

func TestParsePackageJSON_EmptyWorkspacesArray(t *testing.T) {
	p, err := parsePackageJSON([]byte(`{"workspaces":[]}`))
	require.NoError(t, err)
	assert.False(t, p.hasWorkspaces())
}
