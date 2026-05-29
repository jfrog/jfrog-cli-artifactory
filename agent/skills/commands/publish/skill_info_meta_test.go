package publish

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadInstalledSkillVersion_PrefersManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-skill")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	skillMd := "---\nname: my-skill\nversion: 1.0.0\ndescription: x\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMd), 0o644))

	require.NoError(t, common.WriteSkillInfoManifest(dir, common.SkillInfoManifest{
		Repo:             "r",
		Slug:             "my-skill",
		InstalledVersion: "2.0.0",
		Scope:            "project",
		Agent:            "cursor",
	}))

	v, err := ReadInstalledSkillVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", v)
}

func TestReadInstalledSkillVersion_FallsBackToSkillMd(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "only-meta")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	skillMd := "---\nname: only-meta\nversion: 3.1.4\ndescription: x\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMd), 0o644))

	v, err := ReadInstalledSkillVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "3.1.4", v)
}

func TestReadInstalledSkillVersion_CorruptManifestFallsBackToSkillMd(t *testing.T) {
	buf := &bytes.Buffer{}
	prev := log.GetLogger()
	log.SetLogger(log.NewLogger(log.INFO, buf))
	t.Cleanup(func() { log.SetLogger(prev) })

	dir := filepath.Join(t.TempDir(), "corrupt-manifest")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".jfrog"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".jfrog", "skill-info.json"), []byte("{not json"), 0o644))
	skillMd := "---\nname: corrupt-manifest\nversion: 9.8.7\ndescription: x\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMd), 0o644))

	v, err := ReadInstalledSkillVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "9.8.7", v)
	assert.Contains(t, buf.String(), "Invalid skill-info manifest")
}

func TestReadInstalledSkillVersion_ManifestEmptyUsesMeta(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty-manifest-ver")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	skillMd := "---\nname: empty-manifest-ver\nversion: 0.1.0\ndescription: x\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMd), 0o644))
	require.NoError(t, common.WriteSkillInfoManifest(dir, common.SkillInfoManifest{
		Repo:             "r",
		Slug:             "empty-manifest-ver",
		InstalledVersion: "   ",
		Scope:            "project",
		Agent:            "cursor",
	}))

	v, err := ReadInstalledSkillVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "0.1.0", v)
}

func TestReadInstalledSkillVersion_NoVersionField(t *testing.T) {
	dir := skillDirForVersionTest(t, "no-ver", "---\nname: no-ver\ndescription: No version here\n---\n")
	version, err := ReadInstalledSkillVersion(dir)
	require.NoError(t, err)
	assert.Equal(t, "", version, "empty version when SKILL.md has no version field")
}

func TestReadInstalledSkillVersion_NotInstalled(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nonexistent-skill")
	_, err := ReadInstalledSkillVersion(missing)
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "missing SKILL.md must surface fs.ErrNotExist")
}

func TestReadInstalledSkillVersion_InvalidFrontmatter(t *testing.T) {
	dir := skillDirForVersionTest(t, "bad-skill", "# No frontmatter at all\n")
	_, err := ReadInstalledSkillVersion(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse SKILL.md")
}

func skillDirForVersionTest(t *testing.T, slug, skillMD string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, slug)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0o644))
	return dir
}
