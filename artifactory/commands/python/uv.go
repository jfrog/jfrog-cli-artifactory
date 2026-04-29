package python

import (
	"io"
	"os/exec"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/pythonutils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python/dependencies"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// UvCommand is a placeholder for the future `jf rt python uv` path (non-native,
// config-file-based). It is not currently wired to any CLI command; the active
// uv integration uses NativeUvCommand in native_uv.go instead.
type UvCommand struct {
	PythonCommand
}

func NewUvCommand() *UvCommand {
	return &UvCommand{PythonCommand: *NewPythonCommand(pythonutils.UV)}
}

func (pc *UvCommand) UpdateDepsChecksumInfoFunc(dependenciesMap map[string]entities.Dependency, srcPath string) error {
	servicesManager, err := utils.CreateServiceManager(pc.serverDetails, -1, 0, false)
	if err != nil {
		return err
	}
	return dependencies.UpdateDepsChecksumInfo(dependenciesMap, srcPath, servicesManager, pc.repository)
}

func (pc *UvCommand) SetRepo(repo string) *UvCommand {
	pc.PythonCommand.SetRepo(repo)
	return pc
}

func (pc *UvCommand) SetArgs(arguments []string) *UvCommand {
	pc.PythonCommand.SetArgs(arguments)
	return pc
}

func (pc *UvCommand) SetCommandName(commandName string) *UvCommand {
	pc.PythonCommand.SetCommandName(commandName)
	return pc
}

func (pc *UvCommand) CommandName() string {
	return "rt_python_uv"
}

func (pc *UvCommand) SetServerDetails(serverDetails *config.ServerDetails) *UvCommand {
	pc.PythonCommand.SetServerDetails(serverDetails)
	return pc
}

func (pc *UvCommand) ServerDetails() (*config.ServerDetails, error) {
	return pc.serverDetails, nil
}

func (pc *UvCommand) GetCmd() *exec.Cmd {
	cmd := []string{string(pythonutils.UV), pc.commandName}
	cmd = append(cmd, pc.args...)
	return exec.Command(cmd[0], cmd[1:]...)
}

func (pc *UvCommand) GetEnv() map[string]string {
	return map[string]string{}
}

func (pc *UvCommand) GetStdWriter() io.WriteCloser {
	return nil
}

func (pc *UvCommand) GetErrWriter() io.WriteCloser {
	return nil
}
