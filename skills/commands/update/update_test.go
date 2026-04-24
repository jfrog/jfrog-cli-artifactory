package update

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeSkillDir(t *testing.T, parent, slug, version, description string) {
	t.Helper()
	dir := filepath.Join(parent, slug)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + slug + "\nversion: " + version + "\ndescription: " + description + "\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

// ---------------------------------------------------------------------------
// parseFrontmatterVersion
// ---------------------------------------------------------------------------

func TestParseFrontmatterVersion_Valid(t *testing.T) {
	content := "---\nname: my-skill\nversion: 1.2.3\ndescription: A skill\n---\n"
	assert.Equal(t, "1.2.3", parseFrontmatterVersion(content))
}

func TestParseFrontmatterVersion_QuotedValue(t *testing.T) {
	content := "---\nname: my-skill\nversion: \"2.0.0\"\n---\n"
	assert.Equal(t, "2.0.0", parseFrontmatterVersion(content))
}

func TestParseFrontmatterVersion_SingleQuotedValue(t *testing.T) {
	content := "---\nname: my-skill\nversion: '3.1.0'\n---\n"
	assert.Equal(t, "3.1.0", parseFrontmatterVersion(content))
}

func TestParseFrontmatterVersion_NoVersionField(t *testing.T) {
	content := "---\nname: my-skill\ndescription: no version here\n---\n"
	assert.Equal(t, "", parseFrontmatterVersion(content))
}

func TestParseFrontmatterVersion_NoFrontmatter(t *testing.T) {
	content := "Just plain markdown content, no frontmatter."
	assert.Equal(t, "", parseFrontmatterVersion(content))
}

func TestParseFrontmatterVersion_EmptyContent(t *testing.T) {
	assert.Equal(t, "", parseFrontmatterVersion(""))
}

// ---------------------------------------------------------------------------
// common.ResolveInstallPath
// ---------------------------------------------------------------------------

func TestResolveInstallPath_CrossAgent_NoCWD(t *testing.T) {
	path, err := common.ResolveInstallPath(common.CrossAgentName, "", false)
	require.NoError(t, err)
	assert.Equal(t, ".agents/skills", path)
}

func TestResolveInstallPath_CrossAgent_WithProjectDir(t *testing.T) {
	path, err := common.ResolveInstallPath(common.CrossAgentName, "/my/project", false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/my/project", ".agents/skills"), path)
}

func TestResolveInstallPath_CrossAgent_Global(t *testing.T) {
	path, err := common.ResolveInstallPath(common.CrossAgentName, "", true)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path), "global cross-agent path should be absolute")
}

func TestResolveInstallPath_KnownAgent_ProjectLevel_DefaultCWD(t *testing.T) {
	path, err := common.ResolveInstallPath("cursor", "", false)
	require.NoError(t, err)
	assert.Equal(t, ".cursor/skills", path)
}

func TestResolveInstallPath_KnownAgent_ProjectLevel_WithProjectDir(t *testing.T) {
	path, err := common.ResolveInstallPath("cursor", "/my/project", false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/my/project", ".cursor/skills"), path)
}

func TestResolveInstallPath_KnownAgent_Global(t *testing.T) {
	path, err := common.ResolveInstallPath("claude-code", "", true)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path), "global path should be absolute after home expansion")
}

func TestResolveInstallPath_UnknownAgent(t *testing.T) {
	_, err := common.ResolveInstallPath("unknown-bot", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
	assert.Contains(t, err.Error(), "unknown-bot")
}

// ---------------------------------------------------------------------------
// readInstalledVersion
// ---------------------------------------------------------------------------

func TestReadInstalledVersion_Exists(t *testing.T) {
	base := t.TempDir()
	makeSkillDir(t, base, "web-search", "1.0.0", "A web search skill")

	cmd := NewUpdateCommand().SetSlug("web-search").SetAgentName("cursor").SetProjectDir(base)
	skillDir := filepath.Join(base, ".cursor/skills", "web-search")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	content := "---\nname: web-search\nversion: 1.0.0\ndescription: A web search skill\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	assert.Equal(t, "1.0.0", cmd.readInstalledVersion(skillDir))
}

func TestReadInstalledVersion_MissingFile(t *testing.T) {
	base := t.TempDir()
	skillDir := filepath.Join(base, ".cursor/skills", "web-search")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	cmd := NewUpdateCommand().SetSlug("web-search").SetAgentName("cursor").SetProjectDir(base)
	assert.Equal(t, "", cmd.readInstalledVersion(skillDir))
}

func TestReadInstalledVersion_DirDoesNotExist(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("web-search").SetAgentName("cursor")
	assert.Equal(t, "", cmd.readInstalledVersion("/nonexistent/path/web-search"))
}

// ---------------------------------------------------------------------------
// resolveInstallBase
// ---------------------------------------------------------------------------

func TestResolveInstallBase_CrossAgent_ProjectLevel(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("web-search").SetAgentName(common.CrossAgentName)
	base, err := cmd.resolveInstallBase()
	require.NoError(t, err)
	assert.Equal(t, ".agents/skills", base)
}

func TestResolveInstallBase_CrossAgent_Global(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("web-search").SetAgentName(common.CrossAgentName).SetGlobal(true)
	base, err := cmd.resolveInstallBase()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(base), "global cross-agent path should be absolute")
}

func TestResolveInstallBase_MissingAgent(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("web-search")
	_, err := cmd.resolveInstallBase()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestResolveInstallBase_KnownAgent_ProjectLevel(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("web-search").SetAgentName("cursor")
	base, err := cmd.resolveInstallBase()
	require.NoError(t, err)
	assert.Equal(t, ".cursor/skills", base)
}

func TestResolveInstallBase_KnownAgent_GlobalLevel(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("web-search").SetAgentName("cursor").SetGlobal(true)
	base, err := cmd.resolveInstallBase()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(base), "global path should be absolute after home expansion")
}

// ---------------------------------------------------------------------------
// resolveTargetVersion
// ---------------------------------------------------------------------------

func TestResolveTargetVersion_EmptyUsesLatest(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("my-skill").SetVersion("")
	v, err := cmd.resolveTargetVersion([]string{"1.0.0", "1.1.0", "2.0.0"})
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", v)
}

func TestResolveTargetVersion_LatestKeyword(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("my-skill").SetVersion("latest")
	v, err := cmd.resolveTargetVersion([]string{"1.0.0", "1.5.0", "2.3.0"})
	require.NoError(t, err)
	assert.Equal(t, "2.3.0", v)
}

func TestResolveTargetVersion_ExactVersionFound(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("my-skill").SetVersion("1.1.0")
	v, err := cmd.resolveTargetVersion([]string{"1.0.0", "1.1.0", "2.0.0"})
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", v)
}

func TestResolveTargetVersion_VersionNotFound(t *testing.T) {
	cmd := NewUpdateCommand().SetSlug("my-skill").SetVersion("9.9.9").SetQuiet(true)
	_, err := cmd.resolveTargetVersion([]string{"1.0.0", "1.1.0", "2.0.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "9.9.9")
	assert.Contains(t, err.Error(), "Available versions")
	assert.Contains(t, err.Error(), "1.0.0")
	assert.Contains(t, err.Error(), "2.0.0")
}

// ---------------------------------------------------------------------------
// Run — error paths that don't need a server
// ---------------------------------------------------------------------------

func TestUpdateCommand_NotInstalled(t *testing.T) {
	base := t.TempDir()
	cmd := NewUpdateCommand().
		SetSlug("missing-skill").
		SetAgentName("cursor").
		SetProjectDir(base).
		SetRepoKey("repo")

	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
	assert.Contains(t, err.Error(), "jf skills install")
}

func TestUpdateCommand_NotInstalled_NoSKILLMd(t *testing.T) {
	base := t.TempDir()
	// skill dir hierarchy exists but SKILL.md is absent
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".cursor/skills", "partial-skill"), 0o755))

	cmd := NewUpdateCommand().
		SetSlug("partial-skill").
		SetAgentName("cursor").
		SetProjectDir(base).
		SetRepoKey("repo")

	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}
