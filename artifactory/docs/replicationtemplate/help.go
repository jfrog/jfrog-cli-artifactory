package replicationtemplate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rplt <template path>"}

func GetDescription() string {
	return "Create a JSON template for creating a replication repository."
}

func GetAIDescription() string {
	return `Interactively generate a JSON template for replication configuration, usable by jf rt replication-create. Walks through replication type (push/pull), source/target, schedule, etc.

When to use:
- Bootstrapping a new replication definition to commit to version control.
- Producing a parameterizable template that can be applied across environments with --vars.

Prerequisites:
- No server connection required.
- Writable filesystem path for the output file.

Common patterns:
  $ jf rt replication-template ./templates/libs-replication.json

Gotchas:
- Interactive only; cannot be scripted unattended.
- Output is a literal template; add ${var} placeholders manually for reuse.

Related: jf rt replication-create, jf rt replication-delete, jf rt repo-template

QA:
Q: Could you guide me through the process of creating a replication template in Artifactory considering that the template path is 'template1.json'?
A: jf rt rplt template1.json

Q: Could you elucidate the approach for establishing a replication template in Artifactory provided the template path is 'template3.json'?
A: jf rt rplt template3.json

Q: Could you outline the procedure for forming a replication template in Artifactory with the template path being 'template5.json'?
A: jf rt rplt template5.json
`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "template path",
			Description: "Specifies the local file system path for the template file to be used for the replication creation.",
		},
	}
}
