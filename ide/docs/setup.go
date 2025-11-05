package docs

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Setup IDE integration with JFrog Artifactory."
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
