package setup

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

func pypiRepoKey(rest string) string {
	return repoKeyAfter(rest, "api", "pypi")
}

func inspectPip(packageManager project.ProjectType, serverDetails *config.ServerDetails) (inspection, error) {
	binaries := []string{"pip", "pip3"}
	if packageManager == project.Pipenv {
		binaries = []string{"pipenv"}
	}
	result := newInspection(binaryFound(binaries...))
	indexURL, configPath, err := python.GetConfiguredPipIndexURL()
	if err != nil {
		return inspection{}, err
	}
	result.status.Location = configPath
	result.status.State, result.status.Host, result.status.RepoKey = classify(packageManager, indexURL, serverDetails, pypiRepoKey)
	applyURLCredentials(&result, indexURL, CredentialsAbsent)
	result.status.OverriddenBy = pipOverrides(configPath)
	return result, nil
}

// applyURLCredentials reports the credentials embedded in a configured URL, or
// whenMissing when the URL carries none.
func applyURLCredentials(result *inspection, configuredURL string, whenMissing CredentialsState) {
	if result.status.State != StateConfigured {
		return
	}
	result.status.Credentials = whenMissing
	if creds, ok := credentialsFromURL(configuredURL); ok {
		result.status.Credentials = CredentialsPresent
		result.credentials = creds
	}
}

// pipOverrides lists what beats global.index-url in userConfig for `pip install`:
// PIP_INDEX_URL, an [install] index-url, and the active virtual environment's config file,
// which pip reads after the per-user one unless PIP_CONFIG_FILE (read last) is set.
func pipOverrides(userConfig string) []ConfigOverride {
	var overrides []ConfigOverride
	if os.Getenv("PIP_INDEX_URL") != "" {
		overrides = append(overrides, ConfigOverride{Source: "PIP_INDEX_URL environment variable"})
	}
	if indexURL, err := python.ReadPipSectionIndexURL(userConfig, "install"); err == nil && indexURL != "" {
		overrides = append(overrides, ConfigOverride{Source: "[install] index-url", Path: userConfig})
	}
	virtualEnv := os.Getenv("VIRTUAL_ENV")
	if virtualEnv == "" || os.Getenv("PIP_CONFIG_FILE") != "" {
		return overrides
	}
	name := "pip.conf"
	if coreutils.IsWindows() {
		name = "pip.ini"
	}
	venvConfig := filepath.Join(virtualEnv, name)
	for _, section := range []string{"global", "install"} {
		if indexURL, err := python.ReadPipSectionIndexURL(venvConfig, section); err == nil && indexURL != "" {
			overrides = append(overrides, ConfigOverride{Source: "virtual environment pip config", Path: venvConfig})
			break
		}
	}
	return overrides
}

func inspectUV(serverDetails *config.ServerDetails) (inspection, error) {
	result := newInspection(binaryFound("uv"))
	configPath, err := python.GetUserUVConfigPath()
	if err != nil {
		return inspection{}, err
	}
	result.status.Location = configPath
	indexURL, err := python.GetConfiguredUVIndexURL()
	if err != nil {
		return inspection{}, err
	}
	result.status.State, result.status.Host, result.status.RepoKey = classify(project.UV, indexURL, serverDetails, pypiRepoKey)
	// `jf setup uv` stores credentials with `uv auth login`, in a store status cannot read.
	applyURLCredentials(&result, indexURL, CredentialsUnknown)
	result.status.OverriddenBy = uvOverrides(configPath)
	return result, nil
}

// uvOverrides lists what beats the user-level index: the index environment variables, and
// the nearest project uv.toml or pyproject.toml [tool.uv] that sets an index. uv skips
// project discovery when UV_CONFIG_FILE names the file or UV_NO_CONFIG is set.
func uvOverrides(userConfig string) []ConfigOverride {
	var overrides []ConfigOverride
	for _, env := range []string{"UV_DEFAULT_INDEX", "UV_INDEX_URL", "UV_INDEX", "UV_EXTRA_INDEX_URL"} {
		if os.Getenv(env) != "" {
			overrides = append(overrides, ConfigOverride{Source: env + " environment variable"})
		}
	}
	if os.Getenv(python.UVConfigFileEnv) != "" || os.Getenv("UV_NO_CONFIG") != "" {
		return overrides
	}
	if path, ok := uvProjectIndexConfig(userConfig); ok {
		overrides = append(overrides, ConfigOverride{Source: "project " + filepath.Base(path), Path: path})
	}
	return overrides
}

// uvIndexKeys are the settings that choose where uv resolves from.
var uvIndexKeys = []string{"index", "index-url", "extra-index-url", "default-index"}

// uvProjectIndexConfig walks up from the current directory the way uv discovers project
// settings: in each directory uv.toml wins over pyproject.toml's [tool.uv], and the first
// directory with either decides. It reports the file only when it sets an index.
func uvProjectIndexConfig(userConfig string) (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if candidate := filepath.Join(dir, "uv.toml"); candidate != filepath.Clean(userConfig) {
			var settings map[string]any
			if _, decodeErr := toml.DecodeFile(candidate, &settings); decodeErr == nil {
				return candidate, hasAnyKey(settings, uvIndexKeys)
			}
		}
		var pyproject struct {
			Tool struct {
				UV map[string]any `toml:"uv"`
			} `toml:"tool"`
		}
		candidate := filepath.Join(dir, "pyproject.toml")
		if _, decodeErr := toml.DecodeFile(candidate, &pyproject); decodeErr == nil && pyproject.Tool.UV != nil {
			return candidate, hasAnyKey(pyproject.Tool.UV, uvIndexKeys)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func hasAnyKey(values map[string]any, keys []string) bool {
	for _, key := range keys {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}
