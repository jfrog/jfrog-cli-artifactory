package gocommand

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{"rt go <go arguments> [command options]"}

func GetDescription() string {
	return "Run Go commands with JFrog CLI."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "go commands",
			Description: "Arguments and options for the Go command.",
		},
	}
}
