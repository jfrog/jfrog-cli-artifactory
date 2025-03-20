package importbundle

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbi [command options] <path to archive>"}

func GetDescription() string {
	return "Import a local release bundle archive to Artifactory"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{Name: "path to archive", Description: "Path to the release bundle archive on the filesystem"},
	}
}
