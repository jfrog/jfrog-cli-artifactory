package goconfig

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt go-config [command options]"}

func GetDescription() string {
	return "Generate Go build configuration."
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
