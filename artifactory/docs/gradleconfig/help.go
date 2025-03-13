package gradleconfig

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt gradle-config [command options]"}

func GetDescription() string {
	return "Generate Gradle build configuration."
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
