package zerotouchremediation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	artutils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/jfrog/jfrog-client-go/xray/services"
)

// readVcs returns the git repository and commit projectRoot is checked out from, so
// Xray can record where the remediated lockfile lives and later open a fix pull
// request against it. Best-effort like the rest of remediation: outside a git
// checkout, without an origin remote, or when git details cannot be read, it returns
// nil and the request is sent without them.
func readVcs(projectRoot string) *services.ComponentResolutionVcs {
	// The npm project is often a subdirectory of the repository, so FindDotGit walks
	// up from it rather than expecting .git in the project root.
	gitRoot, err := artutils.FindDotGit(projectRoot)
	if err != nil {
		log.Debug("Zero Touch Remediation: could not look for a git checkout, sending no vcs details: ", err.Error())
		return nil
	}
	if gitRoot == "" {
		log.Debug("Zero Touch Remediation: project is not in a git checkout; sending no vcs details")
		return nil
	}
	manager := clientutils.NewGitManager(gitRoot)
	if err = manager.ReadConfig(); err != nil {
		log.Debug("Zero Touch Remediation: could not read git details, sending none: ", err.Error())
		return nil
	}
	// GitManager appends ".git" to whatever origin URL it found, so a checkout with
	// no origin remote reports the bare suffix rather than an empty URL.
	url := manager.GetUrl()
	if url == "" || url == ".git" {
		log.Debug("Zero Touch Remediation: git checkout has no origin remote; sending no vcs details")
		return nil
	}
	branch, revision := manager.GetBranch(), manager.GetRevision()
	if isLinkedCheckout(gitRoot) {
		if branch, revision, err = checkedOutRef(gitRoot); err != nil {
			log.Debug("Zero Touch Remediation: could not read the checked-out commit, sending no vcs details: ", err.Error())
			return nil
		}
	}
	return &services.ComponentResolutionVcs{Url: url, Branch: branch, Revision: revision}
}

// isLinkedCheckout reports whether gitRoot's .git is a file pointing elsewhere, as in a
// worktree or a submodule.
func isLinkedCheckout(gitRoot string) bool {
	info, err := os.Lstat(filepath.Join(gitRoot, ".git"))
	return err == nil && !info.IsDir()
}

// checkedOutRef asks git for the branch and commit checked out in dir. GitManager reads
// HEAD from the repository's shared .git directory, which for a linked worktree is the
// main checkout's HEAD, not the worktree's. A detached HEAD has no branch.
func checkedOutRef(dir string) (branch, revision string, err error) {
	if revision, err = gitOutput(dir, "rev-parse", "HEAD"); err != nil {
		return "", "", err
	}
	if branch, err = gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD"); err != nil {
		return "", "", err
	}
	if branch == "HEAD" {
		branch = ""
	}
	return branch, revision, nil
}

// gitLocalEnvVars are the variables git binds to one repository, as listed by
// `git rev-parse --local-env-vars`. They take precedence over the working directory,
// so inherited from a git hook or a script run in another checkout, they would make
// git answer for that checkout instead of dir.
var gitLocalEnvVars = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
	"GIT_OBJECT_DIRECTORY", "GIT_DIR", "GIT_WORK_TREE", "GIT_IMPLICIT_WORK_TREE", "GIT_GRAFT_FILE",
	"GIT_INDEX_FILE", "GIT_NO_REPLACE_OBJECTS", "GIT_REPLACE_REF_BASE", "GIT_PREFIX",
	"GIT_SHALLOW_FILE", "GIT_COMMON_DIR",
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = withoutGitLocalEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func withoutGitLocalEnv(env []string) []string {
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(gitLocalEnvVars, name) {
			kept = append(kept, kv)
		}
	}
	return kept
}
