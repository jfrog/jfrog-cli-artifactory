package replicationdelete

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rpldel <repository key>"}

func GetDescription() string {
	return "Remove a replication repository from Artifactory."
}

func GetAIDescription() string {
	return `Delete all replication configurations attached to the given repository key. The repository itself is not deleted.

When to use:
- Decommissioning a replication relationship between two Artifactory instances.
- Resetting replication config before re-applying a template.

Prerequisites:
- Admin credentials on the configured server.

Common patterns:
  $ jf rt replication-delete libs-local
  $ jf rt replication-delete libs-local --quiet

Gotchas:
- Interactive confirmation by default; pass --quiet in CI.
- Removes ALL replication configs on the repo (push and pull), not just one.

Related: jf rt replication-create, jf rt replication-template, jf rt repo-delete

QA:
Q: Could you guide me through the process of deleting a replication job in Artifactory considering that the repo name is 'repo1'?
A: jf rt rpldel repo1

Q: Could you elucidate the approach for eradicating a replication job in Artifactory provided the repo name is 'repo3'?
A: jf rt rpldel repo3

Q: Could you outline the procedure for discarding a replication job in Artifactory with the repo name being 'repo5'?
A: jf rt rpldel repo5
`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "repository key",
			Description: "The repository from which the replication will be deleted.",
		},
	}
}
