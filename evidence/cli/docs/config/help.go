package config

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Generate Evidence Provider Configuration."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "sonar", Description: "Generate configuration for SonarQube."},
	}
}
