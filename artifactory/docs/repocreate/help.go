package repocreate

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

var Usage = []string{"rt rc <template path>"}

func GetDescription() string {
	return "Create a new repository in Artifactory."
}

func GetAIDescription() string {
	return `Create one or more Artifactory repositories from a JSON template. The template defines repo key, type (local/remote/virtual/federated), package format, and per-format settings.

When to use:
- Provisioning new repos as part of platform-as-code workflows.
- Creating remote proxies (e.g. for npmjs, maven central) from a checked-in template.

Prerequisites:
- Admin credentials on the configured server (repository creation requires admin).
- A repo template JSON file; produce one interactively with jf rt repo-template, then commit it.

Common patterns:
  $ jf rt repo-create repo-template.json
  $ jf rt repo-create repo-template.json --vars "repoKey=libs-snapshot-local"
  $ jf rt repo-create repo-template.json --server-id my-prod

Gotchas:
- Repo key in the template must NOT already exist; this command will error rather than update.
- For updates, use jf rt repo-update.
- Template variables ${var} are substituted via --vars "k=v;k2=v2".
- Federated repos require Artifactory Enterprise+; remote-host URLs must be reachable.

Related: jf rt repo-update, jf rt repo-delete, jf rt repo-template`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "template path",
			Description: "Specifies the local file system path for the template file to be used for the repository creation. " +
				"The template can be created using the \"" + coreutils.GetCliExecutableName() + " rt rpt\" command.",
		},
	}
}
