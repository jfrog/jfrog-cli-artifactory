package python

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/pythonutils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python/dependencies"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
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

// ── Setup helpers ────────────────────────────────────────────────────────────

// uvIndexName is the fixed name for the JFrog index entry in uv.toml.
// Re-running setup against a different server overwrites the previous entry
// (same single-instance design as pip global.index-url and twine's [pypi] section).
const (
	uvIndexName = "jfrog-pypi"
)

// RunUVAuthLogin stores credentials in UV's native credential store.
//
//	uv auth login <service-url> --username <user> --password <password>
//
// Uses --username + --password (not --token) because --token stores username as
// "__token__" (a PyPI convention Artifactory doesn't recognize), and --username
// with --token is rejected as mutually exclusive by uv.
//
// TODO: switch to --password-stdin once supported by uv to avoid brief ps exposure.
func RunUVAuthLogin(serviceURL, username, password string) error {
	log.Debug("Running uv auth login for", serviceURL)
	cmd := exec.Command("uv", "auth", "login", serviceURL, "--username", username, "--password", password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorutils.CheckErrorf("uv auth login failed: %s\n%s", err, string(output))
	}
	return nil
}

// RunUVAuthLogout removes credentials from UV's native credential store.
// Called by fly-desktop's UV handler during teardown.
//
//	uv auth logout <service-url> --username <user>
func RunUVAuthLogout(serviceURL, username string) error {
	log.Debug("Running uv auth logout for", serviceURL)
	args := []string{"auth", "logout", serviceURL}
	if username != "" {
		args = append(args, "--username", username)
	}
	cmd := exec.Command("uv", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorutils.CheckErrorf("uv auth logout failed: %s\n%s", err, string(output))
	}
	return nil
}

// ConfigureUVIndex writes a [[index]] entry and a global publish-url to the
// user-level uv.toml. The publish-url is derived from the index URL by stripping
// the /simple suffix, so bare `uv publish` uploads to the configured registry.
// If the file already exists, it adds or updates the entry with the given name
// while preserving all other settings in the file.
func ConfigureUVIndex(indexURL string) error {
	configPath, err := getUserUVConfigPath()
	if err != nil {
		return err
	}

	fullCfg, indexes, err := loadUVConfig(configPath)
	if err != nil {
		return err
	}

	found := false
	for i, idx := range indexes {
		if idx.Name == uvIndexName {
			indexes[i].URL = indexURL
			indexes[i].Default = true
			found = true
		} else {
			indexes[i].Default = false
		}
	}
	if !found {
		indexes = append(indexes, uvIndexEntry{
			Name:    uvIndexName,
			URL:     indexURL,
			Default: true,
		})
	}

	fullCfg["publish-url"] = strings.TrimSuffix(indexURL, "/simple")

	return writeUVConfig(configPath, fullCfg, indexes)
}

// RemoveUVIndex removes the JFrog index entry and the global publish-url from
// the user-level uv.toml. If the config file doesn't exist, this is a no-op.
// Called by fly-desktop's UV handler during teardown.
func RemoveUVIndex() error {
	configPath, err := getUserUVConfigPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}

	fullCfg, indexes, err := loadUVConfig(configPath)
	if err != nil {
		return err
	}

	filtered := make([]uvIndexEntry, 0, len(indexes))
	for _, idx := range indexes {
		if idx.Name != uvIndexName {
			filtered = append(filtered, idx)
		}
	}

	delete(fullCfg, "publish-url")

	return writeUVConfig(configPath, fullCfg, filtered)
}

// GetConfiguredUVIndexURL reads the user-level uv.toml and returns the URL
// for the JFrog index entry, or empty string if not found.
// Called by fly-desktop's UV handler during status checks.
func GetConfiguredUVIndexURL() (string, error) {
	configPath, err := getUserUVConfigPath()
	if err != nil {
		return "", err
	}

	_, indexes, err := loadUVConfig(configPath)
	if err != nil {
		return "", err
	}

	for _, idx := range indexes {
		if idx.Name == uvIndexName {
			return idx.URL, nil
		}
	}
	return "", nil
}

func getUserUVConfigPath() (string, error) {
	if configFile := os.Getenv("UV_CONFIG_FILE"); configFile != "" {
		return configFile, nil
	}

	var configDir string
	if runtime.GOOS == "windows" {
		configDir = os.Getenv("APPDATA")
		if configDir == "" {
			return "", errorutils.CheckErrorf("APPDATA environment variable not set")
		}
		configDir = filepath.Join(configDir, "uv")
	} else {
		configDir = os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", errorutils.CheckErrorf("failed to determine home directory: %w", err)
			}
			configDir = filepath.Join(home, ".config", "uv")
		} else {
			configDir = filepath.Join(configDir, "uv")
		}
	}
	return filepath.Join(configDir, "uv.toml"), nil
}

// loadUVConfig reads the uv.toml at path.
// Returns the full config as a generic map (to preserve unknown keys on write)
// and the parsed [[index]] entries separately.
func loadUVConfig(path string) (map[string]any, []uvIndexEntry, error) {
	fullCfg := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fullCfg, nil, nil
		}
		return nil, nil, errorutils.CheckErrorf("failed to read uv config at %s: %w", path, err)
	}
	if _, err := toml.Decode(string(data), &fullCfg); err != nil {
		return nil, nil, errorutils.CheckErrorf("failed to parse uv config at %s: %w", path, err)
	}

	var indexes []uvIndexEntry
	if rawIndexes, ok := fullCfg["index"]; ok {
		indexSlice, ok := rawIndexes.([]map[string]any)
		if !ok {
			return nil, nil, errorutils.CheckErrorf("unexpected type for 'index' in uv config at %s: expected [[index]] array of tables", path)
		}
		for _, entry := range indexSlice {
			idx := uvIndexEntry{}
			if name, ok := entry["name"].(string); ok {
				idx.Name = name
			}
			if url, ok := entry["url"].(string); ok {
				idx.URL = url
			}
			if def, ok := entry["default"].(bool); ok {
				idx.Default = def
			}
			indexes = append(indexes, idx)
		}
	}

	return fullCfg, indexes, nil
}

// writeUVConfig writes the full config map back to disk, replacing the "index"
// key with the provided entries while preserving all other settings.
func writeUVConfig(path string, fullCfg map[string]any, indexes []uvIndexEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return errorutils.CheckErrorf("failed to create uv config directory: %w", err)
	}

	indexMaps := make([]map[string]any, 0, len(indexes))
	for _, idx := range indexes {
		m := map[string]any{
			"name": idx.Name,
			"url":  idx.URL,
		}
		if idx.Default {
			m["default"] = true
		}
		indexMaps = append(indexMaps, m)
	}

	if len(indexMaps) > 0 {
		fullCfg["index"] = indexMaps
	} else {
		delete(fullCfg, "index")
	}

	var b strings.Builder
	encoder := toml.NewEncoder(&b)
	if err := encoder.Encode(fullCfg); err != nil {
		return errorutils.CheckErrorf("failed to encode uv config: %w", err)
	}

	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return errorutils.CheckErrorf("failed to write uv config at %s: %w", path, err)
	}

	log.Debug("Wrote uv configuration to", path)
	return nil
}
