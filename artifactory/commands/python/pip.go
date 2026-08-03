package python

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/pythonutils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python/dependencies"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/permissions"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
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
	return permissions.WriteFileOwnerOnly(cleanPath, []byte(configContent))
}

// ResolvePipConfigPath mirrors pip's own per-user config resolution
// (https://pip.pypa.io/en/stable/topics/configuration/):
//
//	PIP_CONFIG_FILE, when set
//	Windows: %APPDATA%\pip\pip.ini
//	macOS:   ~/Library/Application Support/pip/pip.conf when that directory
//	         exists, else ~/.config/pip/pip.conf (pip does not consult XDG here)
//	Unix:    $XDG_CONFIG_HOME/pip/pip.conf, defaulting to ~/.config
//
// os.UserConfigDir already implements the first three lines of that table
// verbatim, so only pip's macOS "if the directory exists" twist is added here.
//
// pip's legacy ~/.pip/pip.conf is deliberately not derived: its user-config
// list is [legacy, new] and `pip config set` edits the last entry, so it always
// writes the new location. Verified on pip 26.1.2 - with a legacy file present
// it still writes ~/.config/pip/pip.conf. Preferring legacy here would tighten a
// file that holds no token and miss the one that does.
//
// `pip config set` defaults to the user-level file, so this resolution matches
// the file it just wrote without having to parse pip's human-readable output.
func ResolvePipConfigPath() (string, error) {
	if custom := os.Getenv("PIP_CONFIG_FILE"); custom != "" {
		return filepath.Clean(custom), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	pipDir := filepath.Join(configDir, "pip")
	// os.UserConfigDir returns ~/Library/Application Support on macOS, but pip
	// only writes there when that directory already exists and otherwise falls
	// back to a literal ~/.config/pip - XDG_CONFIG_HOME is not consulted.
	if runtime.GOOS == "darwin" {
		if info, statErr := os.Stat(pipDir); statErr != nil || !info.IsDir() {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return "", errorutils.CheckError(homeErr)
			}
			pipDir = filepath.Join(homeDir, ".config", "pip")
		}
	}
	configName := "pip.conf"
	if coreutils.IsWindows() {
		configName = "pip.ini"
	}
	return filepath.Join(pipDir, configName), nil
}

// HardenPipConfigPermissions best-effort restricts the pip config file
// `pip config set` wrote to owner-only, so a cleartext token in index-url is not
// left world-readable. It is best-effort by design: pip is already configured, so
// a path we cannot resolve or tighten is warned about rather than failing the
// setup command (see permissions.RestrictExisting).
//
// Unix only - see permissions.chmodOwnerOnly: on Windows the mode cannot be
// tightened, so pip.ini keeps the ACLs of its parent directory.
func HardenPipConfigPermissions() {
	confPath, err := ResolvePipConfigPath()
	if err != nil {
		log.Warn("Could not resolve the pip configuration file to restrict its permissions: " + err.Error() +
			". If it holds credentials, restrict it to owner-only access manually.")
		return
	}
	permissions.RestrictExisting(confPath)
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
