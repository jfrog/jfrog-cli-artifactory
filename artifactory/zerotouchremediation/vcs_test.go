package zerotouchremediation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jfrog/jfrog-client-go/xray/services"
)

// gitCheckout creates a repository with one commit on main, the given origin (none
// when empty), and an npm project in a subdirectory, which is where lockfiles
// usually live. It returns the repository root and the project directory.
func gitCheckout(t *testing.T, origin string) (repoRoot, projectDir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repoRoot = t.TempDir()
	projectDir = filepath.Join(repoRoot, "services", "web")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "package-lock.json"), []byte("{}"), 0644))
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q", "-b", "main")
	if origin != "" {
		git("remote", "add", "origin", origin)
	}
	git("add", ".")
	git("commit", "-q", "-m", "init")
	return repoRoot, projectDir
}

func headRevision(t *testing.T, repoRoot string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	require.NoError(t, err)
	return string(out[:len(out)-1])
}

func TestReadVcs(t *testing.T) {
	t.Run("reads the repository the project is checked out from", func(t *testing.T) {
		repoRoot, projectDir := gitCheckout(t, "https://github.com/org/repo.git")
		assert.Equal(t, &services.ComponentResolutionVcs{
			Url: "https://github.com/org/repo.git", Branch: "main", Revision: headRevision(t, repoRoot),
		}, readVcs(projectDir))
	})
	t.Run("strips credentials from the origin URL", func(t *testing.T) {
		_, projectDir := gitCheckout(t, "https://user:secret-token@github.com/org/repo.git")
		vcs := readVcs(projectDir)
		require.NotNil(t, vcs)
		assert.NotContains(t, vcs.Url, "secret-token")
		assert.Equal(t, "https://github.com/org/repo.git", vcs.Url)
	})
	t.Run("sends nothing for a checkout without an origin remote", func(t *testing.T) {
		_, projectDir := gitCheckout(t, "")
		assert.Nil(t, readVcs(projectDir))
	})
	t.Run("sends nothing outside a git checkout", func(t *testing.T) {
		assert.Nil(t, readVcs(t.TempDir()))
	})
	t.Run("reads a linked worktree's own branch and commit", func(t *testing.T) {
		repoRoot, _ := gitCheckout(t, "https://github.com/org/repo.git")
		worktree := filepath.Join(t.TempDir(), "feature")
		runGit(t, repoRoot, "worktree", "add", "-q", "-b", "feature", worktree)
		runGit(t, worktree, "commit", "-q", "--allow-empty", "-m", "feature work")
		assert.Equal(t, &services.ComponentResolutionVcs{
			Url: "https://github.com/org/repo.git", Branch: "feature", Revision: headRevision(t, worktree),
		}, readVcs(filepath.Join(worktree, "services", "web")))
	})
	t.Run("reads a linked worktree's own commit when git variables point elsewhere", func(t *testing.T) {
		repoRoot, _ := gitCheckout(t, "https://github.com/org/repo.git")
		otherRoot, _ := gitCheckout(t, "https://github.com/org/other.git")
		runGit(t, otherRoot, "commit", "-q", "--allow-empty", "-m", "other work")
		worktree := filepath.Join(t.TempDir(), "feature")
		runGit(t, repoRoot, "worktree", "add", "-q", "-b", "feature", worktree)
		// As inside a git hook or a script run from another checkout.
		t.Setenv("GIT_DIR", filepath.Join(otherRoot, ".git"))
		t.Setenv("GIT_WORK_TREE", otherRoot)
		vcs := readVcs(filepath.Join(worktree, "services", "web"))
		require.NotNil(t, vcs)
		assert.Equal(t, "feature", vcs.Branch)
		assert.Equal(t, revisionOf(t, worktree), vcs.Revision)
	})
}

// revisionOf reads dir's HEAD with git's repository variables cleared, so it answers
// for dir even while a test has them pointing elsewhere.
func revisionOf(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_DIR=") && !strings.HasPrefix(kv, "GIT_WORK_TREE=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestRunIfEnabled_SendsTheLockfilesRepository(t *testing.T) {
	enableZTR(t)
	repoRoot, projectDir := gitCheckout(t, "https://github.com/org/repo.git")
	client := &mockClient{}
	tool := mockTool{root: projectDir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("{}")}}}

	_, _, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", projectDir, nil)
	require.NoError(t, err)
	require.Equal(t, 1, client.callCount)
	assert.Equal(t, &services.ComponentResolutionVcs{
		Url: "https://github.com/org/repo.git", Branch: "main", Revision: headRevision(t, repoRoot),
	}, client.lastReq.Vcs)
}
