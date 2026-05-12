package install

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnzipFile(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	zipPath := filepath.Join(srcDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"SKILL.md":        "---\nname: test\n---",
		"main.py":         "print('hello')",
		"utils/helper.py": "pass",
	})

	err := unzipFile(zipPath, destDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(destDir, "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: test")

	data, err = os.ReadFile(filepath.Join(destDir, "main.py"))
	require.NoError(t, err)
	assert.Equal(t, "print('hello')", string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "utils", "helper.py"))
	require.NoError(t, err)
	assert.Equal(t, "pass", string(data))
}

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dest")

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
	subDir := filepath.Join(srcDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("content2"), 0644))

	err := copyDir(srcDir, dstDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(data))

	data, err = os.ReadFile(filepath.Join(dstDir, "sub", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(data))
}

func TestResolveAgentTargetDirectories_ProjectScope(t *testing.T) {
	projectRoot := t.TempDir()
	cmd := NewInstallCommand().
		SetSlug("my-skill").
		SetAgents([]common.AgentSpec{
			{Name: "cursor", Config: common.AgentConfig{ProjectDir: ".cursor/skills"}},
			{Name: "claude-code", Config: common.AgentConfig{ProjectDir: ".claude/skills"}},
		}).
		SetGlobal(false).
		SetProjectDir(projectRoot)

	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, filepath.Join(projectRoot, ".cursor", "skills", "my-skill"), targets[0].DestinationDir)
	assert.Equal(t, filepath.Join(projectRoot, ".claude", "skills", "my-skill"), targets[1].DestinationDir)
}

func TestResolveAgentTargetDirectories_GlobalScope(t *testing.T) {
	globalBase := filepath.Join(t.TempDir(), "global", ".cursor", "skills")
	wantBase, err := filepath.Abs(globalBase)
	require.NoError(t, err)

	cmd := NewInstallCommand().
		SetSlug("alpha").
		SetAgents([]common.AgentSpec{
			{Name: "cursor", Config: common.AgentConfig{GlobalDir: globalBase}},
		}).
		SetGlobal(true)

	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(wantBase, "alpha"), targets[0].DestinationDir)
}

func TestResolveAgentTargetDirectories_LegacyInstallPath(t *testing.T) {
	tmp := t.TempDir()
	cmd := NewInstallCommand().SetSlug("legacy").SetInstallPath(tmp)
	targets, err := cmd.resolveAgentTargetDirectories()
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, filepath.Join(tmp, "legacy"), targets[0].DestinationDir)
}

func TestEnsureDestinationDir_CreatesUnderExistingParent(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "skill-x")
	require.NoError(t, ensureDestinationDir(dest))
	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureDestinationDir_CreatesNestedPath(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, ".cursor", "skills", "alpha")
	require.NoError(t, ensureDestinationDir(dest))
	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureDestinationDir_RejectsFileAtDestination(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "blocker")
	require.NoError(t, os.WriteFile(dest, []byte("hi"), 0o644))
	err := ensureDestinationDir(dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestFetchExtractInvocationCountIncrements(t *testing.T) {
	ResetFetchExtractInvocationCount()
	ic := NewInstallCommand().
		SetServerDetails(&config.ServerDetails{Url: "http://127.0.0.1:59999"}).
		SetRepoKey("skills-local").
		SetSlug("noop-skill").
		SetVersion("1.0.0").
		SetQuiet(true)
	tmp := t.TempDir()
	_, err := ic.FetchAndExtractTo(tmp)
	require.Error(t, err)
	assert.Equal(t, 1, FetchExtractInvocationCount)
}

func createTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	w := zip.NewWriter(f)
	defer func() {
		_ = w.Close()
	}()

	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
}
