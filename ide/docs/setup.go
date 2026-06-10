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
- A configured JFrog server (jf c add) OR an explicit --url + --repo-key.
- For VS Code-based IDEs you may need write access to product.json (sudo on some installs).

Common patterns:
  $ jf ide setup vscode --repo-key=vscode-remote
  $ jf ide setup cursor --repo-key=cursor-remote --server-id=my-prod
  $ jf ide setup jetbrains --repo-key=jetbrains-remote

Gotchas:
- Supported IDE names are case-sensitive; unknown names are rejected.
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
