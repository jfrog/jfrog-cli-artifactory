package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	buildinfo "github.com/jfrog/build-info-go/entities"
	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/tests"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
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

func TestIsValidGitCommitSha(t *testing.T) {
	testCases := []struct {
		name     string
		revision string
		valid    bool
	}{
		{name: "abbreviated sha", revision: "6198a62", valid: true},
		{name: "sha-1", revision: "6198a6294722fdc75a570aac505784d2ec0d1818", valid: true},
		{name: "sha-256", revision: strings.Repeat("a1", 32), valid: true},
		{name: "upper case sha", revision: "ABCDEF1234567890123456789012345678901234", valid: true},
		{name: "empty", revision: "", valid: false},
		{name: "whitespace", revision: "       ", valid: false},
		{name: "too short", revision: "6198a6", valid: false},
		{name: "too long", revision: strings.Repeat("a", 65), valid: false},
		{name: "output option", revision: "--output=/tmp/x", valid: false},
		{name: "pickaxe option", revision: "-S foo", valid: false},
		{name: "dash prefixed sha", revision: "-6198a6294722fdc75a570aac505784d2ec0d1818", valid: false},
		{name: "symbolic ref", revision: "HEAD", valid: false},
		{name: "revision range", revision: "6198a62..HEAD", valid: false},
		{name: "non hex", revision: "6198a6294722fdc75a570aac505784d2ec0d181g", valid: false},
		{name: "trailing newline", revision: "6198a6294722fdc75a570aac505784d2ec0d1818\n", valid: false},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidGitCommitSha(tt.revision))
		})
	}
}

func TestGetMatchingRevisionFromBuild(t *testing.T) {
	vcsUrl := "https://github.com/jfrog/jfrog-cli-artifactory.git"
	sha := "6198a6294722fdc75a570aac505784d2ec0d1818"

	testCases := []struct {
		name     string
		vcsList  []buildinfo.Vcs
		expected string
	}{
		{name: "matching url", vcsList: []buildinfo.Vcs{{Url: "https://github.com/jfrog/other.git", Revision: "abcdef1"}, {Url: vcsUrl, Revision: sha}}, expected: sha},
		{name: "no matching url", vcsList: []buildinfo.Vcs{{Url: "https://github.com/jfrog/other.git", Revision: sha}}, expected: ""},
		{name: "matching url without revision", vcsList: []buildinfo.Vcs{{Url: vcsUrl}}, expected: ""},
		{name: "matching url with option revision", vcsList: []buildinfo.Vcs{{Url: vcsUrl, Revision: "--output=/tmp/x"}}, expected: ""},
		{name: "matching url with symbolic revision", vcsList: []buildinfo.Vcs{{Url: vcsUrl, Revision: "HEAD"}}, expected: ""},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getMatchingRevisionFromBuild(&buildinfo.BuildInfo{VcsList: tt.vcsList}, vcsUrl))
		})
	}
}

func TestGitLogRejectsInvalidRevision(t *testing.T) {
	repoDir, _, _, _ := setupGitRepoFixture(t, "git_issues2_.git_suffix")
	gitDetails := GitLogDetails{DotGitPath: repoDir, LogLimit: 3, PrettyFormat: "oneline"}
	outputPath := filepath.Join(t.TempDir(), "x")
	wd, err := os.Getwd()
	require.NoError(t, err)

	testCases := []struct {
		name     string
		revision string
	}{
		{name: "output option", revision: "--output=" + outputPath},
		{name: "pickaxe option", revision: "-S foo"},
		{name: "symbolic ref", revision: "HEAD"},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			logCmd, cleanupFunc, err := prepareGitLogCommand(gitDetails, tt.revision)
			assert.ErrorContains(t, err, "invalid VCS revision")
			assert.Nil(t, logCmd)
			assert.Nil(t, cleanupFunc)

			_, err = getPlainGitLogFromLastVcsRevision(gitDetails, tt.revision)
			assert.ErrorContains(t, err, "invalid VCS revision")

			err = ParseGitLogFromLastVcsRevision(gitDetails, nil, tt.revision)
			assert.ErrorContains(t, err, "invalid VCS revision")

			// Git must not have run: no output file, and the working directory is unchanged.
			assert.NoFileExists(t, outputPath)
			assert.NoFileExists(t, outputPath+"..")
			currentWd, err := os.Getwd()
			require.NoError(t, err)
			assert.Equal(t, wd, currentWd)
		})
	}
}

func TestGetPlainGitLogIgnoresInvalidRevisionFromBuild(t *testing.T) {
	repoDir, _, _, _ := setupGitRepoFixture(t, "git_issues2_.git_suffix")
	gitDetails := GitLogDetails{DotGitPath: repoDir, LogLimit: 3, PrettyFormat: "oneline"}
	outputPath := filepath.Join(t.TempDir(), "x")
	vcsUrl := "https://github.com/jfrog/jfrog-cli-artifactory.git"
	buildInfo := &buildinfo.BuildInfo{VcsList: []buildinfo.Vcs{{Url: vcsUrl, Revision: "--output=" + outputPath}}}

	// The invalid revision is dropped, so git log runs without a revision range and returns all commits.
	revision := getMatchingRevisionFromBuild(buildInfo, vcsUrl)
	assert.Empty(t, revision)
	runGitLogAndCountCommits(t, gitDetails, revision, 3)
	assert.NoFileExists(t, outputPath)
	assert.NoFileExists(t, outputPath+"..")
}

func TestFindDotGit(t *testing.T) {
	repoDir, _, _, _ := setupGitRepoFixture(t, "git_test_.git_suffix")
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

func TestGetLocalGitVcsInfo(t *testing.T) {
	repoDir, expectedURL, expectedRev, expectedBranch := setupGitRepoFixture(t, "git_test_.git_suffix")

	info, err := GetLocalGitVcsInfo(repoDir)
	require.NoError(t, err)
	assert.Equal(t, expectedURL, info.Url)
	assert.Equal(t, expectedRev, info.Revision)
	assert.Equal(t, expectedBranch, info.Branch)

	tmpDir := t.TempDir()
	info, err = GetLocalGitVcsInfo(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, info.Url)
	assert.Empty(t, info.Revision)
	assert.Empty(t, info.Branch)
}

func TestGetLocalGitVcsInfo_FromSubdirectory(t *testing.T) {
	repoDir, _, expectedRev, expectedBranch := setupGitRepoFixture(t, "git_test_.git_suffix")
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

func setupGitRepoFixture(t *testing.T, fixtureName string) (repoDir, url, revision, branch string) {
	t.Helper()
	repoDir = t.TempDir()
	src := filepath.Join("..", "commands", "testdata", fixtureName)
	dst := filepath.Join(repoDir, ".git")
	require.NoError(t, biutils.CopyDir(src, dst, true, nil))

	gitManager := clientutils.NewGitManager(repoDir)
	require.NoError(t, gitManager.ReadConfig())
	return repoDir, gitManager.GetUrl(), gitManager.GetRevision(), gitManager.GetBranch()
}
