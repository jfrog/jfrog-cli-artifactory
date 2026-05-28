package common

import (
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReadSkillInfoManifest(t *testing.T) {
	skillDir := filepath.Join(t.TempDir(), "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, agentcommon.InstallDirMode))

	manifest := SkillInfoManifest{
		Repo:             "skills-local",
		Slug:             "my-skill",
		InstalledVersion: "1.0.3",
		Scope:            "project",
		Agent:            "cursor",
		ProjectDir:       "/abs/project",
	}
	require.NoError(t, WriteSkillInfoManifest(skillDir, manifest))

	assert.Equal(t, "skill-info.json", filepath.Base(SkillInfoManifestPath(skillDir)))

	got, err := ReadSkillInfoManifest(skillDir)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, SkillInfoManifestSchemaVersion, got.SchemaVersion)
	assert.Equal(t, "skills-local", got.Repo)
	assert.Equal(t, "my-skill", got.Slug)
	assert.Equal(t, "1.0.3", got.InstalledVersion)
	assert.Equal(t, "project", got.Scope)
	assert.Equal(t, "cursor", got.Agent)
	assert.Equal(t, "/abs/project", got.ProjectDir)
}

func TestReadSkillInfoManifest_MissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadSkillInfoManifest(dir)
	require.NoError(t, err)
	assert.Nil(t, got)
}
