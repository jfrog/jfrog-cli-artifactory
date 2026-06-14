package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPlainGitLogFromLastVcsRevision(t *testing.T) {
	// Create git folder with files
	originalFolder := "git_issues2_.git_suffix"
	baseDir, dotGitPath := tests.PrepareDotGitDir(t, originalFolder, filepath.Join("..", "commands", "testdata"))
	defer tests.RenamePath(dotGitPath, filepath.Join(baseDir, originalFolder), t)

	gitDetails := GitLogDetails{DotGitPath: dotGitPath, LogLimit: 3, PrettyFormat: "oneline"}

	// Expect all commits without providing a revision.
	runGitLogAndCountCommits(t, gitDetails, "", 3)
	// Expect only commits in range when providing a revision.
	runGitLogAndCountCommits(t, gitDetails, "6198a6294722fdc75a570aac505784d2ec0d1818", 2)
	// Expect an RevisionRangeError error when revision doesn't exist.
	_, err := getPlainGitLogFromLastVcsRevision(gitDetails, "1111111111111111111111111111111111111111")
	assert.ErrorAs(t, err, &RevisionRangeError{})
}

func runGitLogAndCountCommits(t *testing.T, gitDetails GitLogDetails, vcsRevision string, expectedCommits int) {
	gitLog, err := getPlainGitLogFromLastVcsRevision(gitDetails, vcsRevision)
	assert.NoError(t, err)
	commits := strings.Split(strings.TrimSpace(gitLog), "\n")
	assert.Len(t, commits, expectedCommits)
}

func TestFindDotGit(t *testing.T) {
	repoDir, _, _ := setupGitRepoFixture(t, "git_test_.git_suffix")
	testFile := filepath.Join(repoDir, "file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))

	testCases := []struct {
		name      string
		start     string
		wantFound bool
		chdirTo   string
	}{
		{name: "from repo root", start: repoDir, wantFound: true},
		{name: "from file in repo", start: testFile, wantFound: true},
		{name: "from dot", start: ".", wantFound: true, chdirTo: repoDir},
		{name: "from empty delegates to cwd", start: "", wantFound: true, chdirTo: repoDir},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.chdirTo != "" {
				t.Chdir(tt.chdirTo)
			}
			repoRoot, err := FindDotGit(tt.start)
			require.NoError(t, err)
			if tt.wantFound {
				assertPathsEqual(t, repoDir, repoRoot)
			} else {
				assert.Empty(t, repoRoot)
			}
		})
	}
}

func TestFindDotGit_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot, err := FindDotGit(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, repoRoot)
}

func TestNormalizeGitRemoteUrl(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "https with .git suffix", input: "https://github.com/jfrog/jfrog-cli.git", expected: "https://github.com/jfrog/jfrog-cli"},
		{name: "https without .git suffix", input: "https://github.com/jfrog/jfrog-cli", expected: "https://github.com/jfrog/jfrog-cli"},
		{name: "scp-style ssh", input: "git@github.com:jfrog/jfrog-cli.git", expected: "https://github.com/jfrog/jfrog-cli"},
		{name: "scp-style ssh without .git", input: "git@github.com:jfrog/jfrog-cli", expected: "https://github.com/jfrog/jfrog-cli"},
		{name: "ssh protocol", input: "ssh://git@github.com/jfrog/jfrog-cli.git", expected: "https://github.com/jfrog/jfrog-cli"},
		{name: "ssh protocol with port", input: "ssh://git@git.example.com:7999/org/repo.git", expected: "https://git.example.com:7999/org/repo"},
		// #nosec G101 -- test fixture: verifies userinfo is stripped from URL, not a real credential
		{name: "https with credentials", input: "https://user:pass@github.com/jfrog/jfrog-cli.git", expected: "https://github.com/jfrog/jfrog-cli"},
		{name: "azure devops", input: "https://dev.azure.com/myorg/myproject/_git/myrepo", expected: "https://dev.azure.com/myorg/myproject/_git/myrepo"},
		{name: "empty", input: "", expected: ""},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeGitRemoteUrl(tt.input))
		})
	}
}

func TestGetLocalGitVcsInfo(t *testing.T) {
	repoDir, expectedRev, expectedBranch := setupGitRepoFixture(t, "git_test_.git_suffix")

	info, err := GetLocalGitVcsInfo(repoDir)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/jfrog/jfrog-cli-go", info.Url)
	assert.Equal(t, expectedRev, info.Revision)
	assert.Equal(t, expectedBranch, info.Branch)

	repoDirNoSuffix, expectedRevNoSuffix, expectedBranchNoSuffix := setupGitRepoFixture(t, "git_test_no_.git_suffix")
	info, err = GetLocalGitVcsInfo(repoDirNoSuffix)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/jfrog/jfrog-cli-go", info.Url)
	assert.Equal(t, expectedRevNoSuffix, info.Revision)
	assert.Equal(t, expectedBranchNoSuffix, info.Branch)

	tmpDir := t.TempDir()
	info, err = GetLocalGitVcsInfo(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, info.Url)
	assert.Empty(t, info.Revision)
	assert.Empty(t, info.Branch)
}

func TestGetLocalGitVcsInfo_FromSubdirectory(t *testing.T) {
	repoDir, expectedRev, expectedBranch := setupGitRepoFixture(t, "git_test_.git_suffix")
	subDir := filepath.Join(repoDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	info, err := GetLocalGitVcsInfo(subDir)
	require.NoError(t, err)
	assert.Equal(t, expectedRev, info.Revision)
	assert.Equal(t, expectedBranch, info.Branch)
}

func assertPathsEqual(t *testing.T, expected, actual string) {
	t.Helper()
	evalExpected, err := filepath.EvalSymlinks(expected)
	require.NoError(t, err)
	evalActual, err := filepath.EvalSymlinks(actual)
	require.NoError(t, err)
	assert.Equal(t, evalExpected, evalActual)
}

func setupGitRepoFixture(t *testing.T, fixtureName string) (repoDir, revision, branch string) {
	t.Helper()
	repoDir = t.TempDir()
	src := filepath.Join("..", "commands", "testdata", fixtureName)
	dst := filepath.Join(repoDir, ".git")
	require.NoError(t, biutils.CopyDir(src, dst, true, nil))

	info, err := GetLocalGitVcsInfo(repoDir)
	require.NoError(t, err)
	return repoDir, info.Revision, info.Branch
}
