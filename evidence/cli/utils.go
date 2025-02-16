package cli

import (
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
)

type execCommandFunc func(command commands.Command) error

func exec(command commands.Command) error {
	return commands.Exec(command)
}

var subjectTypes = []string{
	subjectRepoPath,
	releaseBundle,
	buildName,
	packageName,
	typeFlag,
}
