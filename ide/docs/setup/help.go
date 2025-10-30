package setup

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{
	"ide setup <ide-name> [url]",
	"ide s <ide-name> [url]",
}

func GetDescription() string {
	return `Setup IDE integration with JFrog Artifactory.

Supported IDEs:
  vscode     Visual Studio Code
  cursor     Cursor IDE
  windsurf   Windsurf IDE
  jetbrains  JetBrains IDEs (IntelliJ IDEA, PyCharm, WebStorm, etc.)

Examples:
  # Setup VSCode 
  jf ide setup vscode --repo-key=vscode-remote

  # Setup Cursor
  jf ide setup cursor --repo-key=cursor-remote

  # Setup with direct URL
  jf ide setup vscode "https://artifactory.example.com/artifactory/api/aieditorextensions/vscode-repo/_apis/public/gallery"`
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "ide-name",
			Description: "IDE to setup. Supported: vscode, cursor, windsurf, jetbrains",
		},
		{
			Name:        "url",
			Description: "[Optional] Direct repository/service URL. When provided, --repo-key and server config are not required.",
			Optional:    true,
		},
	}
}
