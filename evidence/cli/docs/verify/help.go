package verify

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Verify all evidences on the given subject. Add a repo-path, keys, and format for output."
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
