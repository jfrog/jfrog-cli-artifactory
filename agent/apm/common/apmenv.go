package apmcommon

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const agentPackagesAPIPrefix = "/api/agentpackages/"

// AgentPackagesBaseURL returns the Artifactory agentpackages base URL for a repo.
func AgentPackagesBaseURL(sd *config.ServerDetails, repoName string) string {
	base := strings.TrimSuffix(sd.ArtifactoryUrl, "/")
	return base + agentPackagesAPIPrefix + repoName + "/"
}

// BuildRegistryEntry returns (registryURL, token) for APM config.
// AccessToken set → Bearer auth via token field.
// User+Password only → Basic auth via URL-embedded credentials.
func BuildRegistryEntry(sd *config.ServerDetails, repoName string) (registryURL, token string) {
	base := AgentPackagesBaseURL(sd, repoName)
	if sd.AccessToken != "" {
		return base, sd.AccessToken
	}
	if sd.User != "" && sd.Password != "" {
		u, err := url.Parse(base)
		if err == nil {
			u.User = url.UserPassword(sd.User, sd.Password)
			return u.String(), ""
		}
	}
	return base, ""
}

// apmConfigJSON models ~/.apm/config.json. Real-world files carry other top-level keys
// too (e.g. "default_client", "install_target") that belong entirely to the apm CLI and
// aren't understood here — Extra preserves them byte-for-byte across the read-merge-write
// cycle so this code never silently destroys settings it doesn't know about.
type apmConfigJSON struct {
	Experimental experimentalConfig        `json:"-"`
	Registries   map[string]registryConfig `json:"-"`
	Extra        map[string]json.RawMessage
}

type experimentalConfig struct {
	Registries bool `json:"registries,omitempty"`
}

type registryConfig struct {
	URL     string `json:"url"`
	Token   string `json:"token,omitempty"`
	Default bool   `json:"default,omitempty"`
}

func (c *apmConfigJSON) UnmarshalJSON(data []byte) error {
	raw := make(map[string]json.RawMessage)
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
	}
	if v, ok := raw["experimental"]; ok {
		if err := json.Unmarshal(v, &c.Experimental); err != nil {
			return err
		}
		delete(raw, "experimental")
	}
	if v, ok := raw["registries"]; ok {
		if err := json.Unmarshal(v, &c.Registries); err != nil {
			return err
		}
		delete(raw, "registries")
	}
	c.Extra = raw
	return nil
}

func (c apmConfigJSON) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(c.Extra)+2)
	maps.Copy(out, c.Extra)
	expJSON, err := json.Marshal(c.Experimental)
	if err != nil {
		return nil, err
	}
	out["experimental"] = expJSON
	if len(c.Registries) > 0 {
		regJSON, err := json.Marshal(c.Registries)
		if err != nil {
			return nil, err
		}
		out["registries"] = regJSON
	}
	return json.Marshal(out)
}

// discoveredRegistry pairs a registry name with its URL, resolved from either
// ~/.apm/config.json or the project's apm.yml.
type discoveredRegistry struct {
	Name string
	URL  string
}

// discoverMatchingRegistries returns every registry name+URL, from existing config.json
// entries and the project's apm.yml, whose host matches sd.ArtifactoryUrl. A name already
// found in config.json is not overwritten by an apm.yml entry of the same name.
func discoverMatchingRegistries(existing *apmConfigJSON, manifestPath string, sd *config.ServerDetails) []discoveredRegistry {
	seen := make(map[string]bool, len(existing.Registries))
	found := make([]discoveredRegistry, 0, len(existing.Registries))

	for name, entry := range existing.Registries {
		if apmHostMatches(entry.URL, sd.ArtifactoryUrl) {
			found = append(found, discoveredRegistry{Name: name, URL: entry.URL})
			seen[name] = true
		}
	}

	if manifestPath != "" {
		if manifest, loadErr := LoadManifest(manifestPath); loadErr == nil {
			for name, reg := range manifest.Registries {
				if seen[name] || !apmHostMatches(reg.URL, sd.ArtifactoryUrl) {
					continue
				}
				found = append(found, discoveredRegistry{Name: name, URL: reg.URL})
				seen[name] = true
			}
		} else {
			log.Debug("apm.yml parsing failed while discovering registries:", loadErr.Error())
		}
	}

	return found
}

// sanitizeApmEnvName converts a registry name into apm's env-var-safe form: uppercased,
// with "-" and "." mapped to "_" — confirmed against apm's own docs, which give
// "corp-main"/"corp.main"/"Corp-Main" as an explicit example of names that collide.
func sanitizeApmEnvName(name string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToUpper(name))
}

func apmTokenEnvVar(name string) string { return "APM_REGISTRY_TOKEN_" + sanitizeApmEnvName(name) }
func apmUserEnvVar(name string) string  { return "APM_REGISTRY_USER_" + sanitizeApmEnvName(name) }
func apmPassEnvVar(name string) string  { return "APM_REGISTRY_PASS_" + sanitizeApmEnvName(name) }

// checkSanitizationCollisions rejects a set of registry names if two distinct names would
// sanitize to the same env var — apm would only ever see credentials for whichever one
// wins, silently misrouting auth for the other.
func checkSanitizationCollisions(names []string) error {
	bySanitized := make(map[string][]string, len(names))
	for _, n := range names {
		s := sanitizeApmEnvName(n)
		bySanitized[s] = append(bySanitized[s], n)
	}
	for s, ns := range bySanitized {
		if len(ns) > 1 {
			return fmt.Errorf(
				"registry names %v all sanitize to the same env var APM_REGISTRY_TOKEN_%s; rename one to avoid credential misrouting",
				ns, s)
		}
	}
	return nil
}

// injectRegistryCredentialEnv appends APM_REGISTRY_TOKEN_<NAME> (or USER_/PASS_) to env for
// the given registry name, computed from sd. Non-destructive: if the caller already exported
// a credential for this exact name, it's left alone and nothing is appended.
func injectRegistryCredentialEnv(env []string, name string, sd *config.ServerDetails) []string {
	tokenKey, userKey, passKey := apmTokenEnvVar(name), apmUserEnvVar(name), apmPassEnvVar(name)
	if os.Getenv(tokenKey) != "" || (os.Getenv(userKey) != "" && os.Getenv(passKey) != "") {
		log.Debug(fmt.Sprintf("apm auth [%s]: credential env var already set — respecting existing value", name))
		return env
	}
	if sd.AccessToken != "" {
		return append(env, tokenKey+"="+sd.AccessToken)
	}
	if sd.User != "" && sd.Password != "" {
		return append(env, userKey+"="+sd.User, passKey+"="+sd.Password)
	}
	return env
}

// ensureExperimentalFlagEnabled sets experimental.registries=true in the real
// ~/.apm/config.json if it isn't already set. This is the one non-secret, monotonic
// exception to "only jf setup agent-apm writes to the real home": apm has no env-var
// equivalent for this flag (confirmed against apm's own docs), and unlike a registry
// URL it can't collide across projects — it's a single global switch, not project-scoped
// state, so enabling it as a side effect of ordinary usage carries none of the
// cross-project collision risk a persisted registry entry would.
func ensureExperimentalFlagEnabled(realHome string, existing *apmConfigJSON) error {
	if existing.Experimental.Registries {
		return nil
	}
	existing.Experimental.Registries = true
	return writeApmConfig(realHome, existing)
}

// BuildApmEnv resolves how apm should authenticate for this invocation. Credentials always
// travel via APM_REGISTRY_TOKEN_<NAME>/APM_REGISTRY_USER_<NAME>+PASS_<NAME> env vars — never
// written to a file. Three cases:
//  1. The registry is already declared (an existing ~/.apm/config.json entry or apm.yml's own
//     registries: block) — no file write at all; the real environment is used with credential
//     env vars appended.
//  2. --repo names a registry apm doesn't already know about — its URL has to be declared
//     somewhere, since an env var can't carry a URL, so a temp HOME is used containing only
//     URLs (this one, plus every other already-discovered registry so the HOME swap doesn't
//     hide them) and no secrets at all.
//  3. Neither an existing declaration nor --repo — nothing to authenticate against, so this
//     returns an error rather than silently running apm unauthenticated.
//
// Returns os.Environ() with empty tmpHome when sd is nil (no server configured).
func BuildApmEnv(sd *config.ServerDetails, repoName, manifestPath string) (env []string, tmpHome string, err error) {
	if sd == nil {
		return os.Environ(), "", nil
	}

	realHome, existing, err := loadExistingApmConfig()
	if err != nil {
		return nil, "", err
	}

	discovered := discoverMatchingRegistries(existing, manifestPath, sd)

	needsNewDeclaration := false
	if repoName != "" {
		alreadyDeclared := false
		for _, d := range discovered {
			if d.Name == repoName {
				alreadyDeclared = true
				break
			}
		}
		if !alreadyDeclared {
			needsNewDeclaration = true
			discovered = append(discovered, discoveredRegistry{Name: repoName, URL: AgentPackagesBaseURL(sd, repoName)})
		}
	}

	if len(discovered) == 0 {
		return nil, "", fmt.Errorf(
			"no APM registry found for %s: declare one in apm.yml's registries: block, "+
				"add it to ~/.apm/config.json (via 'jf setup agent-apm'), or pass --repo <name>",
			sd.ArtifactoryUrl)
	}

	names := make([]string, 0, len(discovered))
	for _, d := range discovered {
		names = append(names, d.Name)
	}
	if err = checkSanitizationCollisions(names); err != nil {
		return nil, "", err
	}

	if err = ensureExperimentalFlagEnabled(realHome, existing); err != nil {
		return nil, "", err
	}

	env = os.Environ()
	for _, d := range discovered {
		env = injectRegistryCredentialEnv(env, d.Name, sd)
	}

	if !needsNewDeclaration {
		return env, "", nil
	}

	// URL-only, no tokens: --repo named something not already declared anywhere, so its URL
	// has to live in a config.json somewhere. Preserve every other registry (matching-host
	// ones URL-only, non-matching ones verbatim including any token they already had) so this
	// invocation doesn't hide anything the real config already had from apm.
	tempCfg := &apmConfigJSON{
		Experimental: experimentalConfig{Registries: true},
		Registries:   make(map[string]registryConfig, len(discovered)),
		Extra:        existing.Extra,
	}
	for _, d := range discovered {
		tempCfg.Registries[d.Name] = registryConfig{URL: d.URL, Default: d.Name == repoName}
	}
	for name, entry := range existing.Registries {
		if _, ok := tempCfg.Registries[name]; !ok {
			tempCfg.Registries[name] = entry
		}
	}

	tmpHome, err = os.MkdirTemp("", "jf-apm-home-")
	if err != nil {
		return nil, "", fmt.Errorf("create temp home: %w", err)
	}
	if err = writeApmConfig(tmpHome, tempCfg); err != nil {
		_ = os.RemoveAll(tmpHome)
		return nil, "", err
	}

	return replaceEnvHome(env, tmpHome), tmpHome, nil
}

// loadExistingApmConfig reads the user's real ~/.apm/config.json, returning the home path and parsed config.
func loadExistingApmConfig() (realHome string, existing *apmConfigJSON, err error) {
	realHome, err = os.UserHomeDir()
	if err != nil {
		return "", nil, fmt.Errorf("get user home dir: %w", err)
	}
	existing, readErr := readApmConfig(filepath.Join(realHome, ".apm", "config.json"))
	if readErr != nil {
		log.Debug("Could not read existing APM config, starting fresh:", readErr.Error())
		existing = &apmConfigJSON{}
	}
	return realHome, existing, nil
}

func readApmConfig(path string) (*apmConfigJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &apmConfigJSON{}, nil
		}
		return nil, err
	}
	var cfg apmConfigJSON
	if err = json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func writeApmConfig(tmpHome string, cfg *apmConfigJSON) error {
	apmDir := filepath.Join(tmpHome, ".apm")
	if err := os.MkdirAll(apmDir, 0700); err != nil {
		return fmt.Errorf("create .apm dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal APM config: %w", err)
	}
	return os.WriteFile(filepath.Join(apmDir, "config.json"), data, 0600)
}

func replaceEnvHome(env []string, newHome string) []string {
	result := make([]string, 0, len(env))
	replaced := false
	for _, e := range env {
		if strings.HasPrefix(e, "HOME=") {
			result = append(result, "HOME="+newHome)
			replaced = true
		} else {
			result = append(result, e)
		}
	}
	if !replaced {
		result = append(result, "HOME="+newHome)
	}
	return result
}

// RunApmCommand runs "apm <subcmd> <args...>" with the provided environment.
// If env is nil, the current process environment is used.
func RunApmCommand(env []string, subcmd string, args []string) error {
	allArgs := append([]string{subcmd}, args...)
	log.Debug(fmt.Sprintf("Running: apm %s", strings.Join(allArgs, " ")))
	cmd := exec.Command("apm", allArgs...)
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apm %s failed: %w", subcmd, err)
	}
	return nil
}

// ConfigureApmRegistryPersistent configures the user's real ~/.apm/config.json using apm's own
// `apm experimental enable registries` and `apm config set` commands — never by writing the
// file directly. This is the one allowed persistent write — called only by `jf setup agent-apm`.
// repoName is always resolved by the shared `jf setup <tool>` interactive repo picker before
// this is called, so it's never empty here. Every other registry already in the file, and any
// other top-level key (e.g. default_client), is left alone automatically — apm's own config-set
// only ever touches the one key it's told to, and switching a registry's default clears any
// previous default on its own (confirmed live: setting a second registry's default un-defaults
// the first, with no separate unset step needed).
func ConfigureApmRegistryPersistent(sd *config.ServerDetails, repoName string) error {
	if sd == nil {
		return fmt.Errorf("server details are required for APM registry configuration")
	}

	if err := RunApmCommand(nil, "experimental", []string{"enable", "registries"}); err != nil {
		return fmt.Errorf("enable experimental registries: %w", err)
	}

	registryURL, token := BuildRegistryEntry(sd, repoName)
	if err := RunApmCommand(nil, "config", []string{"set", fmt.Sprintf("registry.%s.url", repoName), registryURL}); err != nil {
		return fmt.Errorf("set registry url: %w", err)
	}
	if token != "" {
		if err := RunApmCommand(nil, "config", []string{"set", fmt.Sprintf("registry.%s.token", repoName), token}); err != nil {
			return fmt.Errorf("set registry token: %w", err)
		}
	}
	return RunApmCommand(nil, "config", []string{"set", fmt.Sprintf("registry.%s.default", repoName), "true"})
}

// ResolveRepoNameFromRegistry returns the Artifactory repo name for sd, derived from whichever
// already-declared registry (config.json or apm.yml) matches sd.ArtifactoryUrl. jf setup agent-apm
// always names a registry after the repo it points to, so the registry name doubles as the repo
// name. Returns "" if no registry matches or more than one does (ambiguous) - callers treat this
// the same as an unknown repo, not an error, since it only affects build-info enrichment
// (OriginalDeploymentRepo / checksum lookup), never publish itself.
func ResolveRepoNameFromRegistry(sd *config.ServerDetails, manifestPath string) string {
	if sd == nil {
		return ""
	}
	_, existing, err := loadExistingApmConfig()
	if err != nil {
		return ""
	}
	discovered := discoverMatchingRegistries(existing, manifestPath, sd)
	if len(discovered) != 1 {
		return ""
	}
	return discovered[0].Name
}

// RunApmSubcommandWithAuth is the shared body for all apm command Run() methods:
// validates prerequisites, builds a temp HOME with merged credentials, runs the subcommand,
// and cleans up on return.
func RunApmSubcommandWithAuth(subcmd string, args []string, sd *config.ServerDetails, repoName string) error {
	if err := ValidateApmPrerequisites(); err != nil {
		return err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	manifestPath := filepath.Join(workingDir, ApmManifestName)
	env, tmpHome, err := BuildApmEnv(sd, repoName, manifestPath)
	if err != nil {
		return err
	}
	if tmpHome != "" {
		defer func() {
			if removeErr := os.RemoveAll(tmpHome); removeErr != nil {
				log.Debug("Failed to clean up temp home:", removeErr.Error())
			}
		}()
	}
	return RunApmCommand(env, subcmd, args)
}

// IsHelpRequest returns true if the args include --help, -h, or "help".
func IsHelpRequest(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" || arg == "help" {
			return true
		}
	}
	return false
}
