package ping

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt ping [command options]"}

func GetDescription() string {
	return "Send applicative ping to Artifactory."
}

func GetArguments() []components.Argument {
	return nil
}
