package repoupdate

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

var Usage = []string{"rt ru <template path>"}

func GetDescription() string {
	return "Update an existing repository configuration in Artifactory."
}

func GetAIDescription() string {
	return `Update an existing Artifactory repository from a JSON template. Use to change settings (description, includes, replication settings, layout) without recreating the repo.

When to use:
- Adjusting remote URL of a remote repo.
- Changing repo description or property sets across environments.
- Applying config drift fixes from infrastructure-as-code.

Prerequisites:
- Admin credentials on the configured server.
- The repository specified in the template must already exist.

Common patterns:
  $ jf rt repo-update repo-template.json
  $ jf rt repo-update repo-template.json --vars "repoKey=libs-snapshot-local"

Gotchas:
- Updating "rclass" or "packageType" is typically rejected by Artifactory; use repo-delete + repo-create instead.
- Template substitutes ${var} via --vars "k=v;k2=v2".
- Update is whole-document; omitted fields may be reset to defaults.

Related: jf rt repo-create, jf rt repo-delete, jf rt repo-template

QA:
Q: Could you guide me through the process of updating a repository in JFrog Artifactory considering that the repo name is 'repo1'?
A: jf rt ru repo1

Q: Could you elucidate the approach for altering a repository in JFrog Artifactory provided the repo name is 'repo3'?
A: jf rt ru repo3

Q: Could you outline the procedure for adjusting a repository in JFrog Artifactory with the repo name being 'repo5'?
A: jf rt ru repo5
`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "template path",
			Description: "Specifies the local file system path for the template file to be used for the repository update. " +
				"The template can be created using the `" + coreutils.GetCliExecutableName() + " rt rpt` command.",
		},
	}
}
