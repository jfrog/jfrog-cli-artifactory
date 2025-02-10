package utils

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/tests"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"strings"
	"testing"
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
