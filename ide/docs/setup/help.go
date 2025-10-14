package setup

import (
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
  jetbrains  JetBrains IDEs (IntelliJ IDEA, PyCharm, WebStorm, etc.)

Examples:
  # Setup VSCode 
  jf ide setup vscode --repo-key=https://mycompany.org

  # Setup JetBrains   
  jf ide setup jetbrains --repo-key=https://mycompany.org`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "IDE_NAME",
			Description: "The name of the IDE to setup. Supported IDEs are 'vscode' and 'jetbrains'.",
		},
		{
			Name:        "SERVICE_URL",
			Description: "(Optional) Full marketplace service URL. If provided, --repo-key is not required.",
			Optional:    true,
		},
	}
}
