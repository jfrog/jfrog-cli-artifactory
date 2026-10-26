package setup

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"gopkg.in/yaml.v3"
)

const npmUserConfigEnv = "NPM_CONFIG_USERCONFIG"

// npmrc is the subset of an .npmrc file status needs: the registry and every auth entry.
type npmrc map[string]string

// parseNpmrc parses npm's ini-like config format. Keys such as
// "//host/path/:_authToken" contain colons, so only '=' separates key from value.
func parseNpmrc(content []byte) npmrc {
	values := npmrc{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[expandNpmEnv(strings.TrimSpace(key))] = expandNpmEnv(npmrcValue(value))
	}
	return values
}

// npmrcValue applies the ini rules npm reads values with: a value in matching quotes is
// taken literally; otherwise it ends at the first unescaped ';' or '#', and "\;", "\#" and
// "\\" stand for the character itself.
func npmrcValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		return raw[1 : len(raw)-1]
	}
	var value strings.Builder
	escaped := false
	for _, char := range raw {
		switch {
		case escaped:
			if !strings.ContainsRune(`\;#`, char) {
				value.WriteRune('\\')
			}
			value.WriteRune(char)
			escaped = false
		case char == ';' || char == '#':
			return strings.TrimSpace(value.String())
		case char == '\\':
			escaped = true
		default:
			value.WriteRune(char)
		}
	}
	if escaped {
		value.WriteRune('\\')
	}
	return strings.TrimSpace(value.String())
}

var npmEnvReference = regexp.MustCompile(`\$\{([^${}?]+)(\?)?\}`)

// expandNpmEnv applies npm's substitution, so a token kept in the environment is seen the
// same way npm sees it: ${VAR} becomes its value and stays literal when VAR is unset,
// and ${VAR?} becomes empty when VAR is unset.
func expandNpmEnv(value string) string {
	return npmEnvReference.ReplaceAllStringFunc(value, func(reference string) string {
		match := npmEnvReference.FindStringSubmatch(reference)
		if envValue, set := os.LookupEnv(match[1]); set {
			return envValue
		}
		if match[2] == "?" {
			return ""
		}
		return reference
	})
}

// credentialsFor returns the credentials npm would send to registry: those under the
// longest nerf-darted prefix ("//host/path/") of the registry URL that holds any auth.
// npm 9 and later reject top-level auth entries, so they are not consulted.
func (values npmrc) credentialsFor(registry string) (storedCredentials, bool) {
	nerfed := "//" + strings.TrimPrefix(strings.TrimPrefix(registry, "https://"), "http://")
	if !strings.HasSuffix(nerfed, "/") {
		nerfed += "/"
	}
	var best storedCredentials
	bestPrefix := ""
	for key := range values {
		prefix, _, ok := cutLast(key, ":")
		if !ok || !strings.HasPrefix(prefix, "//") || !strings.HasPrefix(nerfed, strings.TrimSuffix(prefix, "/")+"/") || len(prefix) <= len(bestPrefix) {
			continue
		}
		if creds := values.credentialsAt(prefix); !creds.isEmpty() {
			best, bestPrefix = creds, prefix
		}
	}
	return best, !best.isEmpty()
}

// credentialsAt reads the auth entries under one nerf-darted prefix. A password counts
// only together with its username, as npm requires.
func (values npmrc) credentialsAt(prefix string) storedCredentials {
	creds := storedCredentials{
		token:     values[prefix+":"+commandsutils.NpmConfigAuthTokenKey],
		basicAuth: values[prefix+":"+commandsutils.NpmConfigAuthKey],
	}
	user, encoded := values[prefix+":username"], values[prefix+":_password"]
	if user != "" && encoded != "" {
		if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
			creds.user, creds.password = user, string(decoded)
		}
	}
	return creds
}

func cutLast(value, separator string) (before, after string, found bool) {
	i := strings.LastIndex(value, separator)
	if i == -1 {
		return value, "", false
	}
	return value[:i], value[i+len(separator):], true
}

func npmUserConfigPath() (string, error) {
	if custom := getEnvAnyCase(npmUserConfigEnv); custom != "" {
		return filepath.Abs(custom)
	}
	return userFile(".npmrc")
}

// getEnvAnyCase reads an npm-style variable, which npm accepts in upper or lower case.
func getEnvAnyCase(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return os.Getenv(strings.ToLower(name))
}

func inspectNpm(serverDetails *config.ServerDetails) (inspection, error) {
	result := newInspection(binaryFound("npm"))
	path, err := npmUserConfigPath()
	if err != nil {
		return inspection{}, err
	}
	result.status.Location = path
	content, _, err := readOptionalFile(path)
	if err != nil {
		return inspection{}, err
	}
	values := parseNpmrc(content)
	applyNpmRegistry(&result, project.Npm, values, values, serverDetails)
	homeNpmrc, _ := userFile(".npmrc")
	result.status.OverriddenBy = npmOverrides(path, homeNpmrc)
	return result, nil
}

// pnpmConfigFiles are the files in pnpm's config directory that may hold the registry and
// its credentials, in the order status consults them.
var pnpmConfigFiles = []string{"auth.ini", "rc", "config.yaml"}

func inspectPnpm(serverDetails *config.ServerDetails) (inspection, error) {
	found := binaryFound("pnpm")
	result := newInspection(found)
	// pnpm chooses its own config directory per version and platform, and there is no way
	// to locate it without asking pnpm.
	if !found {
		return result, nil
	}
	configDir, err := pnpmConfigDir()
	if err != nil {
		return inspection{}, err
	}
	if configDir == "" {
		return result, nil
	}
	result.status.Location = configDir
	registrySource, authEntries := npmrc{}, npmrc{}
	for _, name := range pnpmConfigFiles {
		path := filepath.Join(configDir, name)
		content, exists, err := readOptionalFile(path)
		if err != nil {
			return inspection{}, err
		}
		if !exists {
			continue
		}
		var values npmrc
		if name == "config.yaml" {
			if values, err = parsePnpmYaml(content); err != nil {
				return inspection{}, err
			}
		} else {
			values = parseNpmrc(content)
		}
		for key, value := range values {
			if _, set := authEntries[key]; !set {
				authEntries[key] = value
			}
		}
		if _, set := registrySource[commandsutils.NpmConfigRegistryKey]; !set && values[commandsutils.NpmConfigRegistryKey] != "" {
			registrySource = values
			result.status.Location = path
		}
	}
	applyNpmRegistry(&result, project.Pnpm, registrySource, authEntries, serverDetails)
	homeNpmrc, _ := userFile(".npmrc")
	result.status.OverriddenBy = npmOverrides(homeNpmrc)
	return result, nil
}

func parsePnpmYaml(content []byte) (npmrc, error) {
	raw := map[string]interface{}{}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	values := npmrc{}
	for key, value := range raw {
		if text, ok := value.(string); ok {
			values[key] = expandNpmEnv(text)
		}
	}
	return values, nil
}

func applyNpmRegistry(result *inspection, packageManager project.ProjectType, registrySource, authEntries npmrc, serverDetails *config.ServerDetails) {
	registry := registrySource[commandsutils.NpmConfigRegistryKey]
	result.status.State, result.status.Host, result.status.RepoKey = classify(packageManager, registry, serverDetails, func(rest string) string {
		return repoKeyAfter(rest, "api", "npm")
	})
	if result.status.State != StateConfigured {
		return
	}
	result.status.Credentials = CredentialsAbsent
	if creds, ok := authEntries.credentialsFor(registry); ok {
		result.status.Credentials = CredentialsPresent
		result.credentials = creds
	}
}

// npmOverrides lists what beats the user-level registry for npm and pnpm in the current
// directory: the environment, then the project .npmrc when it sets a registry.
func npmOverrides(userConfigs ...string) []ConfigOverride {
	var overrides []ConfigOverride
	if getEnvAnyCase("NPM_CONFIG_REGISTRY") != "" {
		overrides = append(overrides, ConfigOverride{Source: "NPM_CONFIG_REGISTRY environment variable"})
	}
	path := npmProjectConfigPath()
	if path == "" {
		return overrides
	}
	for _, userConfig := range userConfigs {
		if userConfig != "" && filepath.Clean(userConfig) == path {
			return overrides
		}
	}
	if content, exists, err := readOptionalFile(path); err == nil && exists && parseNpmrc(content)[commandsutils.NpmConfigRegistryKey] != "" {
		overrides = append(overrides, ConfigOverride{Source: "project .npmrc", Path: path})
	}
	return overrides
}

// npmProjectConfigPath is the only project .npmrc npm reads: the one in its local prefix,
// the nearest directory holding package.json or node_modules, or else the current directory.
func npmProjectConfigPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		for _, marker := range []string{"package.json", "node_modules"} {
			if _, statErr := os.Stat(filepath.Join(dir, marker)); statErr == nil {
				return filepath.Join(dir, ".npmrc")
			}
		}
		if filepath.Dir(dir) == dir {
			return filepath.Join(cwd, ".npmrc")
		}
	}
}
