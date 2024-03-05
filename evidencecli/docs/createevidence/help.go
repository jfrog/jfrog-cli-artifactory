package createevidence

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Create a custom evidencecli and save it to a repository."
}

func GetArguments() []components.Argument {
	return []components.Argument{{Name: "evidencecli repo key", Description: "Evidence repository name."}, {Name: "evidencecli repo path", Description: "Path in the evidencecli repository."}}
}
