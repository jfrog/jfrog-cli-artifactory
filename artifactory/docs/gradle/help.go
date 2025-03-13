package gradle

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli/docs/common"
)

var Usage = []string{"rt gradle <tasks and options> [command options]"}

var EnvVar = []string{common.JfrogCliReleasesRepo, common.JfrogCliDependenciesDir}

func GetDescription() string {
	return "Execute a Gradle build with Artifactory integration."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "tasks and options",
			Description: "Tasks and options to run with the Gradle command. For example: '-b path/to/build.gradle'.",
		},
	}
}
