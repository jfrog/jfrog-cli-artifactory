package repotemplate

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rpt <template path>"}

func GetDescription() string {
	return "Create a JSON template for repository creation or update."
}

func GetAIDescription() string {
	return `Interactively generate a JSON template file that describes a repository, suitable for jf rt repo-create or jf rt repo-update. The interactive prompts walk through repo type, package format, and per-format options.

When to use:
- Bootstrapping a new repo definition that will be checked into version control.
- Producing a template once, then using it with --vars across many environments.

Prerequisites:
- No server connection required.
- A writable filesystem path for the output file.

Common patterns:
  $ jf rt repo-template ./templates/libs-local.json
  # Then edit the produced JSON and apply:
  $ jf rt repo-create ./templates/libs-local.json --vars "repoKey=libs-local"

Gotchas:
- Interactive only; cannot be scripted unattended.
- Output is a literal template; you usually want to add ${var} placeholders by hand for reusability.

Related: jf rt repo-create, jf rt repo-update, jf rt replication-template

QA:
Q: Could you guide me through the process of creating a configuration template in JFrog Artifactory considering that the template path is 'template1.json'?
A: jf rt rpt template1.json

Q: Could you elucidate the approach for establishing a configuration template in JFrog Artifactory provided the template path is 'template3.json'?
A: jf rt rpt template3.json

Q: Could you outline the procedure for forming a configuration template in JFrog Artifactory with the template path being 'template5.json'?
A: jf rt rpt template5.json
`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "template path",
			Description: "Specifies the local file system path for the template file.",
		},
	}
}
