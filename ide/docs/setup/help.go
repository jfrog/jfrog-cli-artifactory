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
