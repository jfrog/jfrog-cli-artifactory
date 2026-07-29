package python

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/pythonutils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python/dependencies"
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
	if err := os.WriteFile(cleanPath, []byte(configContent), 0600); err != nil {
		return errorutils.CheckError(err)
	}
	// WriteFile applies the mode only when it creates the file, so a config left
	// at 0644 by an earlier run would otherwise stay world-readable.
	return errorutils.CheckError(os.Chmod(cleanPath, 0600))
}

// pipWritingToPrefix is the prefix `pip config set` prints when it persists a
// value, e.g. "Writing to /Users/me/.config/pip/pip.conf".
const pipWritingToPrefix = "Writing to "

// PipConfigPathFromOutput extracts the config file path pip reported writing to,
// or "" when the output carries no such line. This is authoritative — unlike
// ResolvePipConfigPath it cannot disagree with the pip that actually ran.
func PipConfigPathFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		reported, found := strings.CutPrefix(strings.TrimSpace(line), pipWritingToPrefix)
		if !found {
			continue
		}
		if reported = strings.TrimSpace(reported); reported != "" {
			return filepath.Clean(reported)
		}
	}
	return ""
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
// Prefer PipConfigPathFromOutput; this is the fallback for when pip's own
// report is unavailable.
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

// HardenPipConfigPermissions forces the pip config file to 0600 so a cleartext
// token in index-url is not left world-readable after `pip config set`.
//
// reportedPath is the path pip printed; pass "" to fall back to
// ResolvePipConfigPath. A *derived* path that turns out not to exist is only
// warned about: pip's resolution can legitimately differ from ours, and failing
// there would break `jf setup pip` on a machine it had just configured
// correctly. A path pip itself reported must exist, so that stays fail-closed.
//
// On Windows os.Chmod only toggles the read-only attribute; the file is instead
// protected by the per-user ACLs %APPDATA% already carries.
func HardenPipConfigPermissions(reportedPath string) error {
	confPath := reportedPath
	derived := confPath == ""
	if derived {
		var err error
		if confPath, err = ResolvePipConfigPath(); err != nil {
			return err
		}
	}
	if _, err := os.Stat(confPath); err != nil {
		if os.IsNotExist(err) {
			if derived {
				log.Warn("Could not locate the pip configuration file to restrict its permissions (looked in " + confPath +
					"). It may hold credentials in cleartext - consider restricting it to owner-only access manually.")
				return nil
			}
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
