package list

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// expandHome
// ---------------------------------------------------------------------------

func TestExpandHome_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	expanded := expandHome("~/.claude/skills")
	assert.Equal(t, filepath.Join(home, ".claude/skills"), expanded)
}

func TestExpandHome_NoTilde(t *testing.T) {
	path := "/absolute/path/skills"
	assert.Equal(t, path, expandHome(path))
}

func TestExpandHome_TildeOnly(t *testing.T) {
	// "~" without slash should be returned as-is (not a valid prefix)
	assert.Equal(t, "~", expandHome("~"))
}

// ---------------------------------------------------------------------------
// ListCommand.Run — validation
// ---------------------------------------------------------------------------

func TestListCommand_MissingBothFlags(t *testing.T) {
	cmd := &ListCommand{}
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--repo")
	assert.Contains(t, err.Error(), "--agent")
}

func TestListCommand_MutuallyExclusive(t *testing.T) {
	cmd := &ListCommand{}
	cmd.SetRepoKey("my-repo").SetAgentName("cursor")
	err := cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
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

func TestListLocalSkills_ReadsVersionFromSKILLMd(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))
	makeSkillDir(t, skillsPath, "skill-alpha", "1.0.0", "Alpha skill")
	makeSkillDir(t, skillsPath, "skill-beta", "2.3.1", "Beta skill")

	cmd := &ListCommand{agentName: "cursor"}
	cmd.SetProjectDir(projectRoot).SetFormat("json")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Run()

	_ = w.Close()
	os.Stdout = old

	require.NoError(t, err)
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var results []listResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
	assert.Len(t, results, 2)

	// Sorted alphabetically by name (default)
	assert.Equal(t, "skill-alpha", results[0].Name)
	assert.Equal(t, "1.0.0", results[0].Version)
	assert.Equal(t, "skill-beta", results[1].Name)
	assert.Equal(t, "2.3.1", results[1].Version)
}

func TestListLocalSkills_SkipsMissingSKILLMd(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))

	// Skill with SKILL.md
	makeSkillDir(t, skillsPath, "with-meta", "1.0.0", "Has metadata")
	// Skill directory without SKILL.md — should be skipped
	require.NoError(t, os.MkdirAll(filepath.Join(skillsPath, "no-meta"), 0o755))

	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot).SetFormat("json")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Run()

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var results []listResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
	// only the skill with SKILL.md is returned
	assert.Len(t, results, 1)
	assert.Equal(t, "with-meta", results[0].Name)
}

func TestListLocalSkills_EmptyDirectory(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))

	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Run()

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	assert.Contains(t, buf.String(), "No skills found")
}

func TestListLocalSkills_NonExistentDirectory(t *testing.T) {
	projectRoot := t.TempDir()
	// Don't create .cursor/skills — directory does not exist

	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Run()

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err) // should not error, just print message

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	assert.Contains(t, buf.String(), "No skills directory found")
}

func TestListLocalSkills_LimitApplied(t *testing.T) {
	projectRoot := t.TempDir()
	skillsPath := filepath.Join(projectRoot, ".cursor", "skills")
	require.NoError(t, os.MkdirAll(skillsPath, 0o755))
	makeSkillDir(t, skillsPath, "aaa", "1.0.0", "First")
	makeSkillDir(t, skillsPath, "bbb", "1.0.0", "Second")
	makeSkillDir(t, skillsPath, "ccc", "1.0.0", "Third")

	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot).SetFormat("json").SetLimit(2)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Run()

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var results []listResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
	assert.Len(t, results, 2)
}

func TestListLocalSkills_GlobalDir(t *testing.T) {
	tmpDir := t.TempDir()
	makeSkillDir(t, tmpDir, "global-skill", "3.0.0", "Global skill")

	original := common.Agents["cursor"]
	common.Agents["cursor"] = common.AgentConfig{GlobalDir: tmpDir, ProjectDir: original.ProjectDir}
	t.Cleanup(func() { common.Agents["cursor"] = original })

	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetFormat("json")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Run()

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var results []listResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
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

	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot).SetFormat("json").SetSortOrder("asc")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Run()

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var results []listResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
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

	cmd := &ListCommand{}
	cmd.SetAgentName("cursor").SetProjectDir(projectRoot).SetFormat("json").SetSortOrder("desc")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Run()

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var results []listResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
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
	results := []listResult{
		{Name: "my-skill", Version: "1.0.0", Description: "A skill", Source: "Repo: repo-a"},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.printResults(results)

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
	results := []listResult{
		{Name: "skill-a", Version: "2.0.0", Description: "Desc", Source: "/some/path/skill-a"},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.printResults(results)

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var parsed []listResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Len(t, parsed, 1)
	assert.Equal(t, "skill-a", parsed[0].Name)
	assert.Equal(t, "2.0.0", parsed[0].Version)
}

func TestPrintResults_Empty(t *testing.T) {
	cmd := &ListCommand{}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.printResults([]listResult{})

	_ = w.Close()
	os.Stdout = old
	require.NoError(t, err)

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	assert.Contains(t, buf.String(), "No skills found")
}

// ---------------------------------------------------------------------------
// Agent directory map coverage
// ---------------------------------------------------------------------------

func TestAgents_AllKnownAgents(t *testing.T) {
	expected := []string{"claude-code", "cursor", "github-copilot", "windsurf"}
	for _, name := range expected {
		cfg, ok := common.Agents[name]
		assert.True(t, ok, "expected agent %q to be in Agents", name)
		assert.NotEmpty(t, cfg.GlobalDir, "expected agent %q to have a GlobalDir", name)
		assert.NotEmpty(t, cfg.ProjectDir, "expected agent %q to have a ProjectDir", name)
	}
}
