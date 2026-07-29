package python

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/pythonutils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python/dependencies"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type PipCommand struct {
	PythonCommand
}

func NewPipCommand() *PipCommand {
	return &PipCommand{PythonCommand: *NewPythonCommand(pythonutils.Pip)}
}

func (pc *PipCommand) Run() (err error) {
	return pc.PythonCommand.Run()
}

func (pc *PipCommand) UpdateDepsChecksumInfoFunc(dependenciesMap map[string]entities.Dependency, srcPath string) error {
	servicesManager, err := utils.CreateServiceManager(pc.serverDetails, -1, 0, false)
	if err != nil {
		return err
	}
	return dependencies.UpdateDepsChecksumInfo(dependenciesMap, srcPath, servicesManager, pc.repository)
}

func (pc *PipCommand) SetRepo(repo string) *PipCommand {
	pc.PythonCommand.SetRepo(repo)
	return pc
}

func (pc *PipCommand) SetArgs(arguments []string) *PipCommand {
	pc.PythonCommand.SetArgs(arguments)
	return pc
}

func (pc *PipCommand) SetCommandName(commandName string) *PipCommand {
	pc.PythonCommand.SetCommandName(commandName)
	return pc
}

func CreatePipConfigManually(customPipConfigPath, repoWithCredsUrl string) error {
	cleanPath := filepath.Clean(customPipConfigPath)
	if err := os.MkdirAll(filepath.Dir(cleanPath), os.ModePerm); err != nil {
		return err
	}
	// Write the configuration to pip.conf with owner-only permissions — the
	// index-url may embed credentials (user:token@host).
	configContent := fmt.Sprintf("[global]\nindex-url = %s\n", repoWithCredsUrl)
	return os.WriteFile(cleanPath, []byte(configContent), 0600)
}

// ResolvePipConfigPath returns the pip config file path that `jf setup pip`
// writes. Honors PIP_CONFIG_FILE; otherwise uses the platform default that
// `pip config set` targets (Windows: %APPDATA%/pip/pip.ini, else ~/.config/pip/pip.conf).
func ResolvePipConfigPath() (string, error) {
	if custom := os.Getenv("PIP_CONFIG_FILE"); custom != "" {
		return filepath.Clean(custom), nil
	}
	if coreutils.IsWindows() {
		appData := filepath.Clean(os.Getenv("APPDATA"))
		if appData == "" || appData == "." {
			return "", errorutils.CheckErrorf("APPDATA environment variable not set")
		}
		return filepath.Join(appData, "pip", "pip.ini"), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", errorutils.CheckErrorf("failed to determine home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "pip", "pip.conf"), nil
}

// HardenPipConfigPermissions forces the resolved pip config file to 0600 so a
// cleartext token in index-url is not left world-readable after `pip config set`.
func HardenPipConfigPermissions() error {
	confPath, err := ResolvePipConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(confPath); err != nil {
		if os.IsNotExist(err) {
			return errorutils.CheckErrorf("pip config file missing after setup: %s", confPath)
		}
		return errorutils.CheckError(err)
	}
	return errorutils.CheckError(os.Chmod(confPath, 0600))
}

func (pc *PipCommand) CommandName() string {
	return "rt_python_pip"
}

func (pc *PipCommand) SetServerDetails(serverDetails *config.ServerDetails) *PipCommand {
	pc.PythonCommand.SetServerDetails(serverDetails)
	return pc
}

func (pc *PipCommand) ServerDetails() (*config.ServerDetails, error) {
	return pc.serverDetails, nil
}

func (pc *PipCommand) GetCmd() *exec.Cmd {
	var cmd []string
	cmd = append(cmd, string(pc.pythonTool))
	cmd = append(cmd, pc.commandName)
	cmd = append(cmd, pc.args...)
	return exec.Command(cmd[0], cmd[1:]...)
}

func (pc *PipCommand) GetEnv() map[string]string {
	return map[string]string{}
}

func (pc *PipCommand) GetStdWriter() io.WriteCloser {
	return nil
}

func (pc *PipCommand) GetErrWriter() io.WriteCloser {
	return nil
}
