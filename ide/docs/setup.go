package docs

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/ide/ideconsts"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetDescription() string {
	return "Setup IDE integration with JFrog Artifactory."
}

func GetAIDescription() string {
	return `Configure a locally installed IDE (VS Code variants or JetBrains) to pull AI editor extensions or plugins from a JFrog Artifactory remote repo. Auto-detects the IDE installation when possible.

When to use:
- Onboarding a developer machine to a corporate Artifactory mirror of marketplace extensions.
- Pointing VS Code / Cursor / Windsurf / Kiro / JetBrains products at an internal extensions repo.

Prerequisites:
- The IDE must be installed locally; auto-detection looks at standard install paths.
- Authentication comes from one of:
    * a server saved with 'jf config add' (default server, or one selected via --server-id alone), OR
    * --url paired with --access-token, OR
    * --url paired with --user + --password.
- --repo-key identifies the Artifactory repository.
- For VS Code-based IDEs you may need write access to product.json (sudo on some installs).

Common patterns:
  $ jf ide setup vscode --repo-key=vscode-remote
  $ jf ide setup cursor --repo-key=cursor-remote --server-id=my-prod
  $ jf ide setup vscode --url=https://acme.jfrog.io/artifactory --repo-key=vscode-remote --access-token=<token>
  $ jf ide setup jetbrains --repo-key=jetbrains-remote

Gotchas:
- Supported IDE names are case-sensitive; unknown names are rejected.
- --url on its own is not enough; pair it with --access-token or --user + --password.
- --url + --server-id is NOT a valid combination — --server-id's saved credentials are
  ignored when --url is present. To use a saved server, drop --url and pass --server-id alone.
- --update-mode only applies to VS Code-based IDEs: "default", "manual", or "none".
- product.json may require elevated privileges to edit on macOS/Linux system installs.
- If a service URL is passed as the second positional arg, --repo-key and server config become optional.

Related: jf c add, jf rt repo-create`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "ide-name",
			Description: fmt.Sprintf("IDE to setup. Supported: %s", ideconsts.GetSupportedIDEsString()),
		},
		{
			Name:        "url",
			Description: "[Optional] Direct repository/service URL. When provided, --repo-key and server config are not required.",
			Optional:    true,
		},
	}
}
