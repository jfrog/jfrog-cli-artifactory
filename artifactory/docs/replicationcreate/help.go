package replicationcreate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rplc <template path>"}

func GetAIDescription() string {
	return `Create a replication (push or pull) on an Artifactory repository from a template file. Used to mirror artifacts between Artifactory instances or geo-distributed sites.

When to use:
- Setting up push replication from a primary to a secondary Artifactory.
- Configuring pull replication on a remote repo for content caching.

Prerequisites:
- A replication template JSON file; generate one with jf rt replication-template first.
- Admin credentials on the configured server (replication endpoints require admin).
- The source/target repo referenced in the template must already exist.

Common patterns:
  $ jf rt replication-create replication-template.json
  $ jf rt replication-create replication-template.json --vars "sourceRepo=libs-local;targetUrl=https://other-rt"

Gotchas:
- Template variables ${var} in JSON are substituted via --vars "k=v;k2=v2".
- Push replication needs network reachability from this Artifactory to the target.
- Only one replication per repo+URL combination; calling again with the same target errors.

Related: jf rt replication-template, jf rt replication-delete, jf rt repo-create`
}

func GetDescription() string {
	return "Create a new replication in Artifactory."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "template path",
			Description: "Specifies the local file system path for the template file to be used to create a replication. The template can be created using the “jfrog rt rplt” command.",
		},
	}
}
