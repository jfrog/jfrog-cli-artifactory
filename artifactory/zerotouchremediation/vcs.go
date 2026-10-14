package zerotouchremediation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	gitRoot, found := findGitRoot(projectRoot)
	if !found {
		log.Debug("Zero Touch Remediation: project is not in a git checkout; sending no vcs details")
		return nil
	}
	manager := clientutils.NewGitManager(gitRoot)
	if err := readGitConfig(manager); err != nil {
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
		var err error
		if branch, revision, err = checkedOutRef(gitRoot); err != nil {
			log.Debug("Zero Touch Remediation: could not read the checked-out commit, sending no vcs details: ", err.Error())
			return nil
		}
	}
	return &services.ComponentResolutionVcs{Url: url, Branch: branch, Revision: revision}
}

// readGitConfig turns a panic in GitManager.ReadConfig into an error. It indexes into
// .git/config lines without checking their shape, so a malformed one would otherwise
// crash the install that remediation is only a best-effort part of.
func readGitConfig(manager *clientutils.GitManager) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("reading the git config failed: %v", r)
		}
	}()
	return manager.ReadConfig()
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

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// findGitRoot walks up from dir to the directory holding .git (a directory, or a file
// for worktrees and submodules). The npm project is often a subdirectory of the
// repository, so the project root itself is usually not it.
func findGitRoot(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, err = os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
