package get

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Fetch evidence based on a specified subject, which can be either an artifact or a release bundle.\n" +
    "When retrieving evidence from a release bundle, you will obtain information about the builds contained within it,\n" +
    "as well as the artifacts associated with those builds.\n" +
    "Supports JSON and JSONL formats."
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
