package createevidence

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Create a custom evidence and save it to a repository."
}

func GetArguments() []components.Argument {
	return []components.Argument{{Name: "evidence repo key", Description: "Evidence repository name."}, {Name: "evidence repo path", Description: "Path in the evidence repository."}}
}
