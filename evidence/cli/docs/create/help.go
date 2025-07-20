package create

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return " Create a custom evidence and save it to a repository. Add a predicate, predicate-type, repo-path, key, and key-name."
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
