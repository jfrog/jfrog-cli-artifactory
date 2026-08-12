package repodelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rdel <repository pattern>"}

func GetDescription() string {
	return "Permanently delete repositories with all of their content from Artifactory."
}

func GetAIDescription() string {
	return `Permanently delete one or more Artifactory repositories AND all artifacts inside them. Irreversible. Use only for cleanup of decommissioned repos.

When to use:
- Removing scratch/sandbox repos at the end of a test run.
- Bulk-deleting old repos matching a naming pattern.

Prerequisites:
- Admin credentials on the configured server.
- Confirmation: interactive prompt unless --quiet is set.

Common patterns:
  $ jf rt repo-delete tmp-test-repo
  $ jf rt repo-delete "ci-sandbox-*" --quiet

Gotchas:
- Pattern supports wildcards; double-check before adding --quiet in CI.
- All artifacts in matched repos are deleted, not just the repo definition.
- No grace period; rely on backups for recovery.

Related: jf rt repo-create, jf rt repo-update, jf rt delete

QA:
Q: Could you guide me through the process of deleting a repository in Artifactory considering that the repo name is 'repo1'?
A: jf rt rdel repo1

Q: Could you elucidate the approach for eradicating a repository in Artifactory provided the repo name is 'repo3'?
A: jf rt rdel repo3

Q: Could you outline the procedure for discarding a repository in Artifactory with the repo name being 'repo5'?
A: jf rt rdel repo5
`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "repository pattern",
			Description: "Specifies the repositories that should be removed. You can use wildcards to specify multiple repositories.",
		},
	}
}
