package setup

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/ide/ideconsts"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{
	"ide setup <IDE_NAME> [SERVICE_URL]",
	"ide s <IDE_NAME> [SERVICE_URL]",
}

func GetDescription() string {
	return `Setup IDE integration with JFrog Artifactory.

Supported Action:
  setup    Configure your IDE to use JFrog Artifactory

Supported IDEs:
  vscode     Visual Studio Code
  cursor     Cursor IDE
  windsurf   Windsurf IDE
  kiro       Kiro IDE
  jetbrains  JetBrains IDEs (IntelliJ IDEA, PyCharm, WebStorm, etc.)

Examples:
  # Setup VSCode 
  jf ide setup vscode --repo-key=vscode-remote

  # Setup Cursor
  jf ide setup cursor --repo-key=cursor-remote

  # Setup Windsurf
  jf ide setup windsurf --repo-key=windsurf-remote

  # Setup Kiro
  jf ide setup kiro --repo-key=kiro-remote

  # Setup JetBrains
  jf ide setup jetbrains --repo-key=jetbrains-remote`
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
  $ jf ide setup vscode --product-json-path=/Applications/Visual\ Studio\ Code.app/Contents/Resources/app/product.json
  $ jf ide setup jetbrains --repo-key=jetbrains-remote

Gotchas:
- Supported IDE names are case-sensitive; unknown names are rejected.
- --update-mode only applies to VS Code-based IDEs: "default", "manual", or "none".
- product.json may require elevated privileges to edit on macOS/Linux system installs.
- If SERVICE_URL is passed as the second positional, --repo-key and server config become optional.

Related: jf c add, jf rt repo-create`
}

func GetArguments() []components.Argument {
	// Create a quoted list of IDE names for better readability
	ideNames := make([]string, len(ideconsts.SupportedIDEsList))
	for i, name := range ideconsts.SupportedIDEsList {
		ideNames[i] = fmt.Sprintf("'%s'", name)
	}
	supportedIDEsDesc := strings.Join(ideNames, ", ")

	return []components.Argument{
		{
			Name:        "IDE_NAME",
			Description: fmt.Sprintf("The name of the IDE to setup. Supported IDEs are %s.", supportedIDEsDesc),
		},
		{
			Name:        "SERVICE_URL",
			Description: "(Optional) Direct repository service URL. When provided, --repo-key and server config are not required. Example: https://host/api/aieditorextensions/repo/_apis/public/gallery",
			Optional:    true,
		},
	}
}
