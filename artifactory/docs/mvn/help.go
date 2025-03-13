package mvn

import "github.com/jfrog/jfrog-cli/docs/common"
import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt mvn <goals and options> [command options]"}

var EnvVar = []string{common.JfrogCliReleasesRepo, common.JfrogCliDependenciesDir}

func GetDescription() string {
	return "Run Maven build."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "goals and options",
			Description: "Goals and options to run with mvn command. For example, -f path/to/pom.xml",
		},
	}
}
