package apmcommon

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	rtUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/access/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	agentPackagesAPIPrefix = "/api/agentpackages/"

	// ApmBinaryName is the apm executable RunApmCommand always shells out to.
	ApmBinaryName = "apm"

	// HelpFlag is the help flag this package constructs when forwarding to apm.
	HelpFlag = "--help"

	// apmConfigDirName and apmConfigFileName make up ~/.apm/config.json.
	apmConfigDirName  = ".apm"
	apmConfigFileName = "config.json"

	// apmAccessTokenExpirySeconds bounds the lifetime of tokens this package mints for APM
	// registry auth. Matches cliutils/flagkit.ArtifactoryTokenExpiry, the same default `jf
	// access token-create` already uses - the only other place in this repo that mints an
	// access token itself. A non-expiring (expires_in=0) token left no way to bound how long a
	// credential written into ~/.apm/config.json stays valid.
	apmAccessTokenExpirySeconds = 3600
)

// AgentPackagesBaseURL returns the Artifactory agentpackages base URL for a repo.
func AgentPackagesBaseURL(serverDetails *config.ServerDetails, repoName string) string {
	base := strings.TrimSuffix(serverDetails.ArtifactoryUrl, "/")
	return base + agentPackagesAPIPrefix + repoName + "/"
}

// BuildRegistryEntry returns (registryURL, token) for APM config.
// Strategy: Check for AccessToken first, else generate token from User+Password.
// Never embed plaintext credentials in URL - always use token field.
// AccessToken set → use it.
// User+Password set → generate token via Artifactory API; a generation failure is returned as
// an error rather than silently falling back to an unauthenticated URL-only entry.
// Neither → return URL only (caller must handle auth separately) - a legitimate
// anonymous/public-registry case, not a failure.
func BuildRegistryEntry(serverDetails *config.ServerDetails, repoName string) (registryURL, token string, err error) {
	base := AgentPackagesBaseURL(serverDetails, repoName)

	// Priority 1: Use existing access token
	if serverDetails.AccessToken != "" {
		return base, serverDetails.AccessToken, nil
	}

	// Priority 2: Generate token from username/password (secure - no plaintext in config)
	if serverDetails.User != "" && serverDetails.Password != "" {
		generatedToken, genErr := generateAccessToken(serverDetails)
		if genErr != nil {
			return "", "", fmt.Errorf("apm: failed to generate access token for registry %q: %w", repoName, genErr)
		}
		return base, generatedToken, nil
	}

	// No credentials configured at all - return URL only. Not an error: a legitimate
	// anonymous/public-registry case.
	return base, "", nil
}

// generateAccessToken creates an access token from username/password. It tries
// jfrog-client-go's access TokenService first (the same ServiceManager-based path used
// elsewhere in this repo, e.g. jfrog-cli-core's AccessTokenCreateCommand), falling back to a
// direct call against Artifactory's older, deprecated token endpoint only if that fails - some
// Artifactory instances still don't expose (or allow) the modern Access service. Returns an
// error, combining both attempts' failures, only if both paths fail.
func generateAccessToken(serverDetails *config.ServerDetails) (string, error) {
	if serverDetails.User == "" || serverDetails.Password == "" {
		return "", fmt.Errorf("username and password are required to generate an access token")
	}

	token, accessAPIErr := generateAccessTokenViaAccessAPI(serverDetails)
	if accessAPIErr == nil {
		return token, nil
	}
	log.Debug(fmt.Sprintf("apm: modern access-token API failed (%s); falling back to the deprecated Artifactory token endpoint", accessAPIErr.Error()))

	token, legacyErr := generateAccessTokenLegacy(serverDetails)
	if legacyErr == nil {
		return token, nil
	}
	return "", fmt.Errorf("access API: %w; legacy endpoint: %w", accessAPIErr, legacyErr)
}

// generateAccessTokenViaAccessAPI is the primary token-generation path, described above.
func generateAccessTokenViaAccessAPI(serverDetails *config.ServerDetails) (string, error) {
	accessManager, err := rtUtils.CreateAccessServiceManager(serverDetails, false)
	if err != nil {
		return "", fmt.Errorf("failed to create access service manager for token generation: %w", err)
	}

	expiresIn := uint(apmAccessTokenExpirySeconds)
	tokenParams := services.CreateTokenParams{Username: serverDetails.User}
	tokenParams.Scope = "applied-permissions/user"
	tokenParams.ExpiresIn = &expiresIn

	tokenResponse, err := accessManager.CreateAccessToken(tokenParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate access token via the access API: %w", err)
	}
	if tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("access API token generation returned no access_token")
	}

	log.Debug("Access token generated for APM registry via the access API")
	return tokenResponse.AccessToken, nil
}

// generateAccessTokenLegacy calls Artifactory's deprecated token generation API (POST
// /artifactory/api/security/token, form-urlencoded - the JSON, plural "/tokens" endpoint
// returns 405) to create an access token from username/password. Fallback only - see
// generateAccessToken. Returns an error if generation fails.
func generateAccessTokenLegacy(serverDetails *config.ServerDetails) (string, error) {
	tokenURL := strings.TrimSuffix(serverDetails.ArtifactoryUrl, "/") + "/api/security/token"

	form := url.Values{}
	form.Set("username", serverDetails.User)
	form.Set("scope", "applied-permissions/user")
	form.Set("expires_in", strconv.Itoa(apmAccessTokenExpirySeconds))

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to build legacy access token request: %w", err)
	}
	req.SetBasicAuth(serverDetails.User, serverDetails.Password)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to generate legacy access token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() // read-side close on an already fully-read response

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read legacy access token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("legacy access token generation returned status %d: %s", resp.StatusCode, string(body))
	}

	// Response field is "access_token", not "token".
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse legacy token response: %w", err)
	}

	token, ok := response["access_token"].(string)
	if !ok || token == "" {
		return "", fmt.Errorf("no access_token in legacy API response")
	}

	log.Debug("Access token generated for APM registry via the legacy endpoint")
	return token, nil
}

// apmConfigJSON models ~/.apm/config.json. Extra preserves top-level keys this code doesn't
// understand (e.g. "default_client") byte-for-byte across the read-merge-write cycle.
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
	if experimentalRaw, ok := raw["experimental"]; ok {
		if err := json.Unmarshal(experimentalRaw, &c.Experimental); err != nil {
			return err
		}
		delete(raw, "experimental")
	}
	if registriesRaw, ok := raw["registries"]; ok {
		if err := json.Unmarshal(registriesRaw, &c.Registries); err != nil {
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
// entries and the project's apm.yml, whose host matches serverDetails.ArtifactoryUrl. A name
// already found in config.json is not overwritten by an apm.yml entry of the same name.
func discoverMatchingRegistries(existing *apmConfigJSON, manifestPath string, serverDetails *config.ServerDetails) []discoveredRegistry {
	seen := make(map[string]bool, len(existing.Registries))
	found := make([]discoveredRegistry, 0, len(existing.Registries))

	for name, entry := range existing.Registries {
		if apmHostMatches(entry.URL, serverDetails.ArtifactoryUrl) {
			found = append(found, discoveredRegistry{Name: name, URL: entry.URL})
			seen[name] = true
		}
	}

	if manifestPath != "" {
		if manifest, loadErr := LoadManifest(manifestPath); loadErr == nil {
			for name, reg := range manifest.Registries.Entries {
				if seen[name] || !apmHostMatches(reg.URL, serverDetails.ArtifactoryUrl) {
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
// with "-" and "." mapped to "_" (apm's own docs note these collide, e.g. "corp-main"/"corp.main").
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
	for _, name := range names {
		sanitized := sanitizeApmEnvName(name)
		bySanitized[sanitized] = append(bySanitized[sanitized], name)
	}
	for sanitized, collidingNames := range bySanitized {
		if len(collidingNames) > 1 {
			return fmt.Errorf(
				"registry names %v all sanitize to the same env var APM_REGISTRY_TOKEN_%s; rename one to avoid credential misrouting",
				collidingNames, sanitized)
		}
	}
	return nil
}

// injectRegistryCredentialEnv appends APM_REGISTRY_TOKEN_<NAME> (or USER_/PASS_) to env for
// the given registry name, computed from serverDetails. Non-destructive: if the caller already
// exported a credential for this exact name, it's left alone and nothing is appended.
func injectRegistryCredentialEnv(env []string, name string, serverDetails *config.ServerDetails) []string {
	tokenKey, userKey, passKey := apmTokenEnvVar(name), apmUserEnvVar(name), apmPassEnvVar(name)
	if os.Getenv(tokenKey) != "" || (os.Getenv(userKey) != "" && os.Getenv(passKey) != "") {
		log.Debug(fmt.Sprintf("apm auth [%s]: credential env var already set — respecting existing value", name))
		return env
	}
	if serverDetails.AccessToken != "" {
		return append(env, tokenKey+"="+serverDetails.AccessToken)
	}
	if serverDetails.User != "" && serverDetails.Password != "" {
		return append(env, userKey+"="+serverDetails.User, passKey+"="+serverDetails.Password)
	}
	return env
}

// ensureExperimentalFlagEnabled sets experimental.registries=true in ~/.apm/config.json if
// unset. Safe as a side effect of ordinary usage: it's a global, non-secret switch with no
// per-project collision risk, unlike a registry entry.
func ensureExperimentalFlagEnabled(realHome string, existing *apmConfigJSON) error {
	if existing.Experimental.Registries {
		return nil
	}
	existing.Experimental.Registries = true
	return writeApmConfig(realHome, existing)
}

// BuildApmEnv resolves how apm should authenticate for this invocation. Credentials always
// travel via APM_REGISTRY_TOKEN_<NAME>/APM_REGISTRY_USER_<NAME>+PASS_<NAME> env vars — never
// written to a file. The registry must already be declared (an existing ~/.apm/config.json
// entry, written by 'jf setup apm', or apm.yml's own registries: block); if none matches
// serverDetails's host, this returns an error rather than silently running apm unauthenticated.
func BuildApmEnv(serverDetails *config.ServerDetails, manifestPath string) ([]string, error) {
	if serverDetails == nil {
		return nil, fmt.Errorf("server details are required to build the APM environment")
	}

	realHome, existing, err := loadExistingApmConfig()
	if err != nil {
		return nil, err
	}

	discovered := discoverMatchingRegistries(existing, manifestPath, serverDetails)
	if len(discovered) == 0 {
		return nil, fmt.Errorf(
			"no APM registry found for %s: declare one in apm.yml's registries: block, "+
				"or add it to ~/.apm/config.json (via 'jf setup apm')",
			serverDetails.ArtifactoryUrl)
	}

	names := make([]string, 0, len(discovered))
	for _, registry := range discovered {
		names = append(names, registry.Name)
	}
	if err = checkSanitizationCollisions(names); err != nil {
		return nil, err
	}

	if err = ensureExperimentalFlagEnabled(realHome, existing); err != nil {
		return nil, err
	}

	env := os.Environ()
	for _, registry := range discovered {
		env = injectRegistryCredentialEnv(env, registry.Name, serverDetails)
	}
	return env, nil
}

// loadExistingApmConfig reads the user's real ~/.apm/config.json, returning the home path and parsed config.
func loadExistingApmConfig() (realHome string, existing *apmConfigJSON, err error) {
	realHome, err = os.UserHomeDir()
	if err != nil {
		return "", nil, fmt.Errorf("get user home dir: %w", err)
	}
	existing, readErr := readApmConfig(filepath.Join(realHome, apmConfigDirName, apmConfigFileName))
	if readErr != nil {
		log.Debug("Could not read existing APM config, starting fresh:", readErr.Error())
		existing = &apmConfigJSON{}
	}
	return realHome, existing, nil
}

func readApmConfig(path string) (*apmConfigJSON, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is always os.UserHomeDir()+"/.apm/config.json" (see loadExistingApmConfig), never user-supplied
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

// writeApmConfig writes cfg via temp-file-plus-rename, so a crash mid-write or two racing
// invocations can never truncate or corrupt the user's real, persistent APM config.
func writeApmConfig(tmpHome string, cfg *apmConfigJSON) error {
	apmDir := filepath.Join(tmpHome, apmConfigDirName)
	if err := os.MkdirAll(apmDir, 0700); err != nil {
		return fmt.Errorf("create .apm dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal APM config: %w", err)
	}

	tmpFile, err := os.CreateTemp(apmDir, "config-*.json.tmp") // 0600 by default
	if err != nil {
		return fmt.Errorf("create temp APM config: %w", err)
	}
	tmpPath := tmpFile.Name()
	// No-op once the rename below succeeds; otherwise clears the leftover temp file (best-effort).
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err = tmpFile.Write(data); err != nil {
		_ = tmpFile.Close() // close-on-error: the write error above already explains the failure
		return fmt.Errorf("write temp APM config: %w", err)
	}
	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp APM config: %w", err)
	}
	if err = os.Rename(tmpPath, filepath.Join(apmDir, apmConfigFileName)); err != nil {
		return fmt.Errorf("rename temp APM config into place: %w", err)
	}
	return nil
}

// SanitizeLogValue strips newline/carriage-return characters from a value before it's
// concatenated into a log message, so CLI-controlled input (e.g. a subcommand name) can't forge
// fake log lines (CWE-117) by embedding its own line breaks.
func SanitizeLogValue(value string) string {
	return strings.NewReplacer("\n", "", "\r", "").Replace(value)
}

// RunApmCommand runs "apm <subcmd> <args...>" with the provided environment (current process
// environment if nil). Captures output to detect validation failures that APM may report but
// exit with code 0 on. Logs only the subcommand name, never args, since args can carry secrets
// (registry tokens, basic-auth URLs).
func RunApmCommand(env []string, subcmd string, args []string) error {
	log.Debug(fmt.Sprintf("Running: apm %s", SanitizeLogValue(subcmd)))
	allArgs := append([]string{subcmd}, args...)
	cmd := exec.Command(ApmBinaryName, allArgs...) // #nosec G204 -- args are this same invocation's own CLI arguments, forwarded verbatim by design (this is the passthrough wrapper); no shell is invoked and no privilege boundary is crossed
	if env != nil {
		cmd.Env = env
	}

	// Capture only a bounded tail of stdout/stderr - just enough to spot the validation
	// markers below - so a chatty subcommand can't grow these buffers unbounded. The full,
	// unbounded output still reaches the user via os.Stdout/os.Stderr.
	var outBuf, errBuf tailBuffer
	outBuf.maxSize, errBuf.maxSize = maxCapturedOutputBytes, maxCapturedOutputBytes
	cmd.Stdout = io.MultiWriter(os.Stdout, &outBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("apm %s failed: %w", subcmd, err)
	}

	// apm sometimes exits with code 0 even when dependency validation failed, so scan the
	// output too - but only for subcommands that actually perform validation. Passthrough
	// subcommands (e.g. "list", "--help") can legitimately contain "[x]" in unrelated text.
	if isValidationCheckedSubcommand(subcmd) {
		output := outBuf.String() + errBuf.String()
		if strings.Contains(output, "[x]") || strings.Contains(output, "All packages failed validation") {
			return fmt.Errorf("apm %s failed: validation errors detected in output", subcmd)
		}
	}
	return nil
}

// maxCapturedOutputBytes bounds how much of each stream's tail RunApmCommand retains for
// validation-marker detection.
const maxCapturedOutputBytes = 64 * 1024

// tailBuffer is an io.Writer that retains only the most recent maxSize bytes written to it.
type tailBuffer struct {
	maxSize int
	buf     []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.maxSize {
		t.buf = t.buf[len(t.buf)-t.maxSize:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	return string(t.buf)
}

// isValidationCheckedSubcommand reports whether subcmd is one of the apm subcommands that
// perform dependency validation and can exit 0 while still having failed it.
func isValidationCheckedSubcommand(subcmd string) bool {
	switch subcmd {
	case "install", "publish":
		return true
	default:
		return false
	}
}

// ConfigureApmRegistryPersistent configures ~/.apm/config.json via apm's own `apm experimental
// enable registries` and `apm config set` commands, never by writing the file directly. Called
// only by `jf setup apm`; repoName is always non-empty, resolved by the interactive repo
// picker beforehand.
func ConfigureApmRegistryPersistent(serverDetails *config.ServerDetails, repoName string) error {
	if serverDetails == nil {
		return fmt.Errorf("server details are required for APM registry configuration")
	}

	if err := RunApmCommand(nil, "experimental", []string{"enable", "registries"}); err != nil {
		return fmt.Errorf("enable experimental registries: %w", err)
	}

	registryURL, token, err := BuildRegistryEntry(serverDetails, repoName)
	if err != nil {
		return fmt.Errorf("build registry entry: %w", err)
	}
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

// registryNameFromArgs extracts the value of an explicit --registry flag from apm subcommand
// args, e.g. ["--package", "acme/pkg", "--registry", "corp-main"] -> "corp-main", or
// ["--registry=corp-main"] -> "corp-main". Returns "" if --registry isn't present.
func registryNameFromArgs(args []string) string {
	for i, arg := range args {
		if arg == "--registry" && i+1 < len(args) {
			return args[i+1]
		}
		if cut, ok := strings.CutPrefix(arg, "--registry="); ok {
			return cut
		}
	}
	return ""
}

// repoKeyFromRegistryURL extracts the Artifactory repo key from an APM registry URL of the form
// ".../api/agentpackages/<repoKey>/" (trailing slash and any further sub-path are ignored, e.g.
// for a virtual-package path suffix). Returns "" if the URL doesn't contain that prefix.
func repoKeyFromRegistryURL(registryURL string) string {
	_, after, found := strings.Cut(registryURL, agentPackagesAPIPrefix)
	if !found {
		return ""
	}
	rest := strings.Trim(after, "/")
	if rest == "" {
		return ""
	}
	if before, _, ok := strings.Cut(rest, "/"); ok {
		rest = before
	}
	return rest
}

// registryURLByName looks up a registry's URL by name, checking the project's apm.yml first
// (it can override or add to what's in ~/.apm/config.json) and falling back to config.json.
func registryURLByName(manifest *ApmManifest, existing *apmConfigJSON, name string) (string, bool) {
	if manifest != nil {
		if entry, ok := manifest.Registries.Entries[name]; ok {
			return entry.URL, true
		}
	}
	if entry, ok := existing.Registries[name]; ok {
		return entry.URL, true
	}
	return "", false
}

// repoNameByRegistryName resolves a registry name to its Artifactory repo key. Returns "" if the
// name is empty or isn't declared anywhere.
func repoNameByRegistryName(manifest *ApmManifest, existing *apmConfigJSON, name string) string {
	if name == "" {
		return ""
	}
	url, ok := registryURLByName(manifest, existing, name)
	if !ok {
		return ""
	}
	return repoKeyFromRegistryURL(url)
}

// defaultRegistryName returns whichever registry is marked default: apm.yml's registries.default
// takes priority, then whichever ~/.apm/config.json entry has "default": true. Returns "" if
// neither declares one.
func defaultRegistryName(manifest *ApmManifest, existing *apmConfigJSON) string {
	if manifest != nil && manifest.Registries.Default != "" {
		return manifest.Registries.Default
	}
	for name, entry := range existing.Registries {
		if entry.Default {
			return name
		}
	}
	return ""
}

// ResolveRepoNameFromRegistry returns the Artifactory repo name that an apm publish/install
// actually targeted, for build-info enrichment. Priority:
//  1. An explicit --registry <name> in args - looked up by name in apm.yml, then
//     ~/.apm/config.json, and its repo key derived from the registry URL.
//  2. No --registry passed - whichever registry is marked default (see defaultRegistryName).
//  3. Neither resolves - falls back to the old host-matching heuristic (only definitive when
//     exactly one configured registry matches serverDetails' host).
//
// Returns "" if nothing resolves (treated as unknown, not an error - it only affects build-info
// enrichment, never publish itself).
func ResolveRepoNameFromRegistry(serverDetails *config.ServerDetails, manifestPath string, args []string) string {
	if serverDetails == nil {
		return ""
	}
	_, existing, err := loadExistingApmConfig()
	if err != nil {
		return ""
	}
	var manifest *ApmManifest
	if manifestPath != "" {
		if loadedManifest, loadErr := LoadManifest(manifestPath); loadErr == nil {
			manifest = loadedManifest
		} else {
			log.Debug("apm.yml parsing failed while resolving registry repo name:", loadErr.Error())
		}
	}

	if explicit := registryNameFromArgs(args); explicit != "" {
		if repo := repoNameByRegistryName(manifest, existing, explicit); repo != "" {
			return repo
		}
		log.Debug(fmt.Sprintf("apm publish: --registry %s not found in apm.yml or ~/.apm/config.json; falling back to host-matching", explicit))
	} else if repo := repoNameByRegistryName(manifest, existing, defaultRegistryName(manifest, existing)); repo != "" {
		return repo
	}

	discovered := discoverMatchingRegistries(existing, manifestPath, serverDetails)
	if len(discovered) != 1 {
		return ""
	}
	return discovered[0].Name
}

// RunApmSubcommandWithAuth is the shared body for all apm command Run() methods:
// validates prerequisites, builds the auth environment, and runs the subcommand.
func RunApmSubcommandWithAuth(subcmd string, args []string, serverDetails *config.ServerDetails) error {
	if err := ValidateApmPrerequisites(); err != nil {
		return err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	manifestPath := filepath.Join(workingDir, ApmManifestName)
	env, err := BuildApmEnv(serverDetails, manifestPath)
	if err != nil {
		return err
	}
	return RunApmCommand(env, subcmd, args)
}

// IsHelpRequest returns true if the args include --help, -h, or "help".
func IsHelpRequest(args []string) bool {
	for _, arg := range args {
		if arg == HelpFlag || arg == "-h" || arg == "help" {
			return true
		}
	}
	return false
}

// IsDryRunArg returns true if the args include --dry-run. install and publish both support it,
// and neither changes anything on disk when it's set.
func IsDryRunArg(args []string) bool {
	return slices.Contains(args, "--dry-run")
}

// PassthroughCommand runs an arbitrary apm subcommand with auth environment injected - no
// build-info collection, unlike install/publish. It satisfies jfrog-cli-core's Command interface
// (CommandName/ServerDetails/Run) on its own, so `jf agent apm <subcmd>` for any subcommand not
// covered by install/publish needs no command-specific type of its own.
type PassthroughCommand struct {
	Subcmd string
	Args   []string
	Server *config.ServerDetails
}

func (c *PassthroughCommand) CommandName() string {
	return CommandNamePrefix + c.Subcmd
}

func (c *PassthroughCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.Server, nil
}

func (c *PassthroughCommand) Run() error {
	log.Info("Running apm " + SanitizeLogValue(c.Subcmd) + "...")
	return RunApmSubcommandWithAuth(c.Subcmd, c.Args, c.Server)
}
