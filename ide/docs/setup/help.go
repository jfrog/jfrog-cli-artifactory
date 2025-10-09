package setup

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{
	"ide setup <IDE_NAME>",
	"ide s <IDE_NAME>",
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
  jf ide setup vscode

  # Setup JetBrains   
  jf ide setup jetbrains`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "IDE_NAME",
			Description: "The name of the IDE to setup. Supported IDEs are 'vscode' and 'jetbrains'.",
		},
	}
}
