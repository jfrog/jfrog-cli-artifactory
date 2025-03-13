package gitlfsclean

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

var Usage = []string{"rt glc [command options] [path to .git]"}

func GetDescription() string {
	return "Clean files from a Git LFS repository. This command deletes all files from a Git LFS repository that are no longer available in the corresponding Git repository."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path to .git",
			Description: "Path to a directory containing the .git directory. If not specified, the .git directory is assumed to be in the current directory.",
		},
	}
}
