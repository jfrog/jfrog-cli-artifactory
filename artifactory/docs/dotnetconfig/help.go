package dotnetconfig

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt dotnet-config [command options]"}

func GetDescription() string {
	return "Generate dotnet configuration."
}

func GetArguments() []components.Argument {
	return nil
}
