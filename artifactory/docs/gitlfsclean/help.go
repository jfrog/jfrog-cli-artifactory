package gitlfsclean

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{"rt glc [command options] [path to .git]"}

func GetDescription() string {
	return "Clean files from a Git LFS repository. This command deletes all files from a Git LFS repository that are no longer available in the corresponding Git repository."
}

func GetAIDescription() string {
	return `Garbage-collect a Git LFS repository in Artifactory: delete LFS objects that are no longer referenced by any commit on the configured refs of the local Git repo.

When to use:
- Reclaiming storage in an Artifactory LFS repo after large files have been removed from history.
- Periodic maintenance of an LFS mirror.

Prerequisites:
- Local clone of the Git repo (path-to-.git) with the relevant refs fetched.
- Configured server with delete permission on the LFS repo.

Common patterns:
  $ jf rt git-lfs-clean
  $ jf rt git-lfs-clean /workspace/myrepo --repo=my-lfs-repo
  $ jf rt git-lfs-clean --refs="refs/heads/main;refs/heads/release/*" --dry-run

Gotchas:
- Defaults to scanning refs/remotes/* of the local clone; ensure you fetched the branches you care about.
- Interactive confirmation by default; use --quiet in scripts.
- Always run --dry-run first; deletes are permanent.

Related: jf rt delete, jf rt build-discard`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path to .git",
			Description: "Path to a directory containing the .git directory. If not specified, the .git directory is assumed to be in the current directory.",
		},
	}
}
