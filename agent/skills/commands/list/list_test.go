package list

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLog redirects the jfrog logger to a buffer for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := log.GetLogger()
	log.SetLogger(log.NewLogger(log.INFO, buf))
	t.Cleanup(func() { log.SetLogger(prev) })
	return buf
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns captured bytes.
func captureStdout(t *testing.T, fn func()) []byte {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return bytes.TrimSpace(buf.Bytes())
}

// ---------------------------------------------------------------------------
// ListCommand.Run — validation
// ---------------------------------------------------------------------------

func TestListCommand_MissingBothFlags(t *testing.T) {
	cmd := &ListCommand{}
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jf agent skills list requires")
	assert.Contains(t, err.Error(), "--repo")
	assert.Contains(t, err.Error(), "--harness")
}

func TestListCommand_MutuallyExclusive(t *testing.T) {
	cmd := &ListCommand{}
	cmd.SetRepoKey("my-repo").SetAgentName("cursor")
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestListCommand_CommaSeparatedHarnessRejected(t *testing.T) {
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor,claude-code")
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "one harness name")
}

func TestListCommand_UnknownAgent(t *testing.T) {
	cmd := &ListCommand{}
	cmd.SetAgentName("unknown-editor")
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
	assert.Contains(t, err.Error(), "unknown-editor")
}

// ---------------------------------------------------------------------------
// listLocalSkills — filesystem scanning
// ---------------------------------------------------------------------------

func makeSkillDir(t *testing.T, parent, slug, version, description string) {
	t.Helper()
	dir := filepath.Join(parent, slug)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + slug + "\nversion: " + version + "\ndescription: " + description + "\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func TestListLocalSkills_UsesManifestRepo(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	makeSkillDir(t, skillsPath, "web-search", "1.0.3", "ws")
	man := common.SkillInfoManifest{
		Repo:             "skills-local",
		Slug:             "web-search",
		InstalledVersion: "1.0.3",
		Scope:            "project",
		Agent:            "cursor",
		ProjectDir:       projectRoot,
	}
	require.NoError(t, agentcommon.WriteInstallInfoManifest(filepath.Join(skillsPath, "web-search"), common.SkillInfoManifestFile, man))

	captureLog(t)
	cmd := &ListCommand{agentName: "cursor"}
	cmd.SetProjectDir(projectRoot).SetFormat("json")

	jsonOut := captureStdout(t, func() {
		require.NoError(t, cmd.Run())
	})

	var results []localListRow
	require.NoError(t, json.Unmarshal(jsonOut, &results))
	require.Len(t, results, 1)
	assert.Equal(t, "skills-local", results[0].Repo)
}

func TestListLocalSkills_InstalledVersionPrefersManifest(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	makeSkillDir(t, skillsPath, "dual-ver", "1.0.0", "desc")
	require.NoError(t, agentcommon.WriteInstallInfoManifest(filepath.Join(skillsPath, "dual-ver"), common.SkillInfoManifestFile, common.SkillInfoManifest{
		Repo:             "skills-local",
		Slug:             "dual-ver",
		InstalledVersion: "5.0.0",
		Scope:            "project",
		Agent:            "cursor",
		ProjectDir:       projectRoot,
	}))

	captureLog(t)
	cmd := &ListCommand{agentName: "cursor"}
	cmd.SetProjectDir(projectRoot).SetFormat("json")

	jsonOut := captureStdout(t, func() {
		require.NoError(t, cmd.Run())
	})

	var results []localListRow
	require.NoError(t, json.Unmarshal(jsonOut, &results))
	require.Len(t, results, 1)
	assert.Equal(t, "5.0.0", results[0].Version)
}

func TestListCommand_CheckUpdatesRequiresServer(t *testing.T) {
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").
		SetProjectDir(t.TempDir()).
		SetCheckUpdates(true)
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--check-updates requires")
}

func TestCheckUpdateStatusFromSemverComparison(t *testing.T) {
	tests := []struct {
		cmp  int
		want string
	}{
		{-1, listCheckStatusBehind},
		{0, listCheckStatusCurrent},
		{1, listCheckStatusAhead},
		{2, listCheckStatusAhead},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, checkUpdateStatusFromSemverComparison(tt.cmp), "cmp=%d", tt.cmp)
	}
}

func TestListLocalSkills_ReadsVersionFromSKILLMd(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))
	makeSkillDir(t, skillsPath, "skill-alpha", "1.0.0", "Alpha skill")
	makeSkillDir(t, skillsPath, "skill-beta", "2.3.1", "Beta skill")

	captureLog(t)
	cmd := &ListCommand{agentName: "cursor"}
	cmd.SetProjectDir(projectRoot).SetFormat("json")

	jsonOut := captureStdout(t, func() {
		require.NoError(t, cmd.Run())
	})

	var results []localListRow
	require.NoError(t, json.Unmarshal(jsonOut, &results))
	assert.Len(t, results, 2)

	// Sorted alphabetically by name (default)
	assert.Equal(t, "skill-alpha", results[0].Name)
	assert.Equal(t, "1.0.0", results[0].Version)
	assert.Equal(t, "(unknown)", results[0].Repo)
	assert.Equal(t, ".cursor/skills/skill-alpha", results[0].Path)
	assert.Equal(t, "skill-beta", results[1].Name)
	assert.Equal(t, "2.3.1", results[1].Version)
	assert.Equal(t, ".cursor/skills/skill-beta", results[1].Path)
}

func TestListLocalSkills_SkipsMissingSKILLMd(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))

	makeSkillDir(t, skillsPath, "with-meta", "1.0.0", "Has metadata")
	require.NoError(t, os.MkdirAll(filepath.Join(skillsPath, "no-meta"), 0o755))

	captureLog(t)
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot).SetFormat("json")

	jsonOut := captureStdout(t, func() {
		require.NoError(t, cmd.Run())
	})

	var results []localListRow
	require.NoError(t, json.Unmarshal(jsonOut, &results))
	assert.Len(t, results, 1)
	assert.Equal(t, "with-meta", results[0].Name)
}

func TestListLocalSkills_EmptyDirectory(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))

	buf := captureLog(t)
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot)

	require.NoError(t, cmd.Run())
	assert.Contains(t, buf.String(), "No skills found")
}

func TestListLocalSkills_NonExistentDirectory(t *testing.T) {
	projectRoot := t.TempDir()

	buf := captureLog(t)
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot)

	require.NoError(t, cmd.Run())
	assert.Contains(t, buf.String(), "No skills directory found")
}

func TestListLocalSkills_LimitApplied(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))
	makeSkillDir(t, skillsPath, "aaa", "1.0.0", "First")
	makeSkillDir(t, skillsPath, "bbb", "1.0.0", "Second")
	makeSkillDir(t, skillsPath, "ccc", "1.0.0", "Third")

	captureLog(t)
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot).SetFormat("json").SetLimit(2)

	jsonOut := captureStdout(t, func() {
		require.NoError(t, cmd.Run())
	})

	var results []localListRow
	require.NoError(t, json.Unmarshal(jsonOut, &results))
	assert.Len(t, results, 2)
}

func TestListLocalSkills_GlobalDir(t *testing.T) {
	tmpDir := t.TempDir()
	makeSkillDir(t, tmpDir, "global-skill", "3.0.0", "Global skill")

	original := common.Agents["cursor"]
	common.Agents["cursor"] = common.AgentConfig{GlobalDir: tmpDir, ProjectDir: original.ProjectDir}
	t.Cleanup(func() { common.Agents["cursor"] = original })

	captureLog(t)
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetGlobal(true).SetFormat("json")

	jsonOut := captureStdout(t, func() {
		require.NoError(t, cmd.Run())
	})

	var results []localListRow
	require.NoError(t, json.Unmarshal(jsonOut, &results))
	require.Len(t, results, 1)
	assert.Equal(t, "global-skill", results[0].Name)
	assert.Equal(t, "3.0.0", results[0].Version)
}

func TestListLocalSkills_SortAscending(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))
	makeSkillDir(t, skillsPath, "zzz", "1.0.0", "Z")
	makeSkillDir(t, skillsPath, "aaa", "1.0.0", "A")
	makeSkillDir(t, skillsPath, "mmm", "1.0.0", "M")

	captureLog(t)
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot).SetFormat("json").SetSortOrder("asc")

	jsonOut := captureStdout(t, func() {
		require.NoError(t, cmd.Run())
	})

	var results []localListRow
	require.NoError(t, json.Unmarshal(jsonOut, &results))
	require.Len(t, results, 3)
	assert.Equal(t, "aaa", results[0].Name)
	assert.Equal(t, "mmm", results[1].Name)
	assert.Equal(t, "zzz", results[2].Name)
}

func TestListLocalSkills_SortDescending(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))
	makeSkillDir(t, skillsPath, "aaa", "1.0.0", "A")
	makeSkillDir(t, skillsPath, "zzz", "2.0.0", "Z")
	makeSkillDir(t, skillsPath, "mmm", "1.5.0", "M")

	captureLog(t)
	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot).SetFormat("json").SetSortOrder("desc")

	jsonOut := captureStdout(t, func() {
		require.NoError(t, cmd.Run())
	})

	var results []localListRow
	require.NoError(t, json.Unmarshal(jsonOut, &results))
	require.Len(t, results, 3)
	assert.Equal(t, "zzz", results[0].Name)
	assert.Equal(t, "mmm", results[1].Name)
	assert.Equal(t, "aaa", results[2].Name)
}

// ---------------------------------------------------------------------------
// printResults — output formatting
// ---------------------------------------------------------------------------

func TestPrintResults_Table(t *testing.T) {
	cmd := &ListCommand{format: "table"}
	results := []repoListRow{
		{Name: "my-skill", Version: "1.0.0", Description: "A skill", Source: "Repo: repo-a"},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.printRepoResults(results)

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "my-skill")
	assert.Contains(t, output, "1.0.0")
}

func TestPrintResults_JSON(t *testing.T) {
	cmd := &ListCommand{format: "json"}
	results := []repoListRow{
		{Name: "skill-a", Version: "2.0.0", Description: "Desc", Source: "/some/path/skill-a"},
	}

	out := captureStdout(t, func() {
		require.NoError(t, cmd.printRepoResults(results))
	})

	var parsed []repoListRow
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Len(t, parsed, 1)
	assert.Equal(t, "skill-a", parsed[0].Name)
	assert.Equal(t, "2.0.0", parsed[0].Version)
}

func TestPrintResults_Empty_Table(t *testing.T) {
	buf := captureLog(t)
	cmd := &ListCommand{}

	require.NoError(t, cmd.printRepoResults([]repoListRow{}))
	assert.Contains(t, buf.String(), "No skills found")
}

func TestPrintResults_Empty_JSON(t *testing.T) {
	cmd := &ListCommand{}
	cmd.SetFormat("json")

	var nilResults []repoListRow
	out := captureStdout(t, func() {
		require.NoError(t, cmd.printRepoResults(nilResults))
	})

	var parsed []repoListRow
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Len(t, parsed, 0)
	assert.Contains(t, string(out), "[]")
}

// ---------------------------------------------------------------------------
// Agent directory map coverage
// ---------------------------------------------------------------------------

func TestAgents_AllKnownAgents(t *testing.T) {
	expected := []string{
		"claude-code",
		"codex",
		"cross-agent",
		"cursor",
		"github-copilot",
		"windsurf",
	}
	for _, name := range expected {
		cfg, ok := common.Agents[name]
		assert.True(t, ok, "expected agent %q to be in Agents", name)
		assert.NotEmpty(t, cfg.GlobalDir, "expected agent %q to have a GlobalDir", name)
		assert.NotEmpty(t, cfg.ProjectDir, "expected agent %q to have a ProjectDir", name)
	}
}
