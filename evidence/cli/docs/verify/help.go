package verify

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return `Verify all evidence for the given subject. Specify the path to the subject and keys.
	You can provide keys using the --keys flag, the JFROG_CLI_SIGNING_KEY environment variable, or retrieve them from Artifactory using --use-artifactory-keys.
	The command returns exit code 0 if verification is passed and exit code 1 if it is failed.`
}

func GetArguments() []components.Argument {
	return []components.Argument{}
}
