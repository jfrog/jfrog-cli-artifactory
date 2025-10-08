package commandWrappers

import (
	containerutils "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func DeprecationCmdWarningWrapper(cmdName, oldSubcommand string, c *components.Context,
	cmd func(c *components.Context) error) error {
	commonCliUtils.LogNonNativeCommandDeprecation(cmdName, oldSubcommand)
	return cmd(c)
}

func LogNativeCommandDeprecation(cmdName, projectType string) {
	log.Warn(
		`You are using a deprecated syntax of the command.
	The new command syntax is quite similar to the syntax used by the native ` + projectType + ` client.
	All you need to do is to add '` + coreutils.GetCliExecutableName() + `' as a prefix to the command.
	For example:
	$ ` + coreutils.GetCliExecutableName() + ` ` + cmdName + ` ...
	The --build-name and --build-number options are still supported.`)
}

func ShowDockerDeprecationMessageIfNeeded(containerManagerType containerutils.ContainerManagerType, isGetRepoSupported func() (bool, error)) error {
	if containerManagerType == containerutils.DockerClient {
		// Show a deprecation message for this command, if Artifactory supports fetching the physical docker repository name.
		supported, err := isGetRepoSupported()
		if err != nil {
			return err
		}
		if supported {
			LogNativeCommandDeprecation("docker", "Docker")
		}
	}
	return nil
}
