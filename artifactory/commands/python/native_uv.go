package python

import (
	"crypto/md5"  // #nosec G501 -- sha1/md5 are used for Artifactory build-info checksums, not for security
	"crypto/sha1" // #nosec G505
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// NativeUVCommand runs `uv` directly (no config file required) and collects build info.
type NativeUVCommand struct {
	commandName        string
	args               []string
	serverID           string
	deployerRepo       string
	buildConfiguration *buildUtils.BuildConfiguration
}

// NewNativeUVCommand creates a new NativeUVCommand instance.
func NewNativeUVCommand() *NativeUVCommand {
	return &NativeUVCommand{}
}

func (c *NativeUVCommand) SetCommandName(name string) *NativeUVCommand {
	c.commandName = name
	return c
}

func (c *NativeUVCommand) SetArgs(args []string) *NativeUVCommand {
	c.args = args
	return c
}

func (c *NativeUVCommand) SetServerID(serverID string) *NativeUVCommand {
	c.serverID = serverID
	return c
}

func (c *NativeUVCommand) SetDeployerRepo(deployerRepo string) *NativeUVCommand {
	c.deployerRepo = deployerRepo
	return c
}

func (c *NativeUVCommand) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) *NativeUVCommand {
	c.buildConfiguration = bc
	return c
}

func (c *NativeUVCommand) CommandName() string {
	return "rt_uv_native"
}

func (c *NativeUVCommand) ServerDetails() (*coreConfig.ServerDetails, error) {
	return uvResolveServerDetails(c.serverID)
}

// Run executes the UV command with auth injection and optional build info collection.
func (c *NativeUVCommand) Run() error {
	// Help flags (-h / --help) and the bare "help" sub-command must bypass all auth
	// injection and Artifactory calls. Injecting credential env vars before calling uv
	// causes uv to print them in its help output, exposing secrets.
	if isHelpRequest(c.commandName, c.args) {
		return runUvBinary(append([]string{c.commandName}, c.args...))
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Parse pyproject.toml once and reuse across the entire Run invocation.
	pyproject := parseUvPyproject(workingDir)

	// Resolve publish URL: explicit arg > pyproject.toml
	deployerRepo := c.deployerRepo
	// use a local args copy so we don't mutate the receiver
	args := append([]string(nil), c.args...)
	if c.commandName == "publish" && deployerRepo == "" {
		if tomlURL := pyproject.Tool.Uv.PublishURL; tomlURL != "" {
			deployerRepo = tomlURL
			args = append(args, "--publish-url", tomlURL)
			log.Info("Using publish URL from pyproject.toml [tool.uv]: " + tomlURL)
		}
	}

	// For --script, also read [[tool.uv.index]] from the script's inline PEP 723 metadata.
	// The script runs in an isolated temp env separate from the project, so its own
	// index config must be used for credential injection.
	scriptIndexes := uvScriptInlineIndexes(c.commandName, c.args)

	serverDetails, credErr := uvResolveServerDetails(c.serverID)
	if credErr != nil {
		log.Warn("UV auth: could not load jf server config — " + credErr.Error())
	} else if serverDetails != nil {
		c.injectCredentials(workingDir, deployerRepo, serverDetails, scriptIndexes)
	}

	log.Info(fmt.Sprintf("Running UV %s.", c.commandName))
	if err := runUvBinary(append([]string{c.commandName}, args...)); err != nil {
		return fmt.Errorf("uv %s failed: %w", c.commandName, err)
	}

	if c.buildConfiguration != nil {
		buildName, err := c.buildConfiguration.GetBuildName()
		if err == nil && buildName != "" {
			// PEP 723 inline scripts get their own build-info from the adjacent .lock file.
			if scriptPath := uvScriptPath(c.commandName, c.args); scriptPath != "" {
				if biErr := uvGetScriptBuildInfo(scriptPath, c.buildConfiguration, serverDetails); biErr != nil {
					log.Warn("Failed to collect UV script build info: " + biErr.Error())
				}
			} else {
			// For commands that modify the venv, capture exactly what was installed.
			var installed map[string]string
			if uvModifiesVenv(c.commandName) {
				installed = uvInstalledPackages()
			}
			if biErr := uvGetBuildInfo(workingDir, c.buildConfiguration, deployerRepo, c.commandName, c.args, installed, serverDetails); biErr != nil {
				log.Warn("Failed to collect UV build info: " + biErr.Error())
			}
			} // end else (not a --script invocation)
		}
	}
	return nil
}

// injectCredentials sets UV_INDEX_* and UV_PUBLISH_* env vars from jf config.
// When the user explicitly specified --server-id, credentials are injected regardless of
// URL hostname — the explicit choice overrides the host-mismatch safety check.
// Without --server-id (using the default server), credentials are only injected when
// the index URL hostname matches the jf server to prevent cross-instance credential leakage.
// scriptIndexes are additional index entries parsed from a PEP 723 inline script's metadata;
// when non-nil they are used instead of the project's pyproject.toml entries.
func (c *NativeUVCommand) injectCredentials(workingDir, deployerRepo string, serverDetails *coreConfig.ServerDetails, scriptIndexes []uvIndexEntry) {
	user := serverDetails.User
	pass := serverDetails.Password
	if serverDetails.AccessToken != "" {
		if user == "" {
			user = auth.ExtractUsernameFromAccessToken(serverDetails.AccessToken)
		}
		pass = serverDetails.AccessToken
	}
	if user == "" || pass == "" {
		return
	}

	injectedAny := false

	// UV_INDEX_URL and UV_DEFAULT_INDEX are UV's global default index env vars.
	// Log them for visibility; per-named-index injection still proceeds because
	// these vars only affect the default/fallback index, not named [[tool.uv.index]] entries.
	if os.Getenv("UV_INDEX_URL") != "" || os.Getenv("UV_DEFAULT_INDEX") != "" {
		log.Info("UV auth: global index env var set (UV_INDEX_URL or UV_DEFAULT_INDEX); per-index credential injection still proceeds below")
	}

	// Use script inline indexes when --script is active; fall back to pyproject.toml entries.
	indexes := scriptIndexes
	if len(indexes) == 0 {
		indexes = uvReadIndexesFromToml(workingDir)
	}
	for _, idx := range indexes {
		envName := uvIndexEnvName(idx.Name)
		userKey := uvIndexUsernameKey(envName)
		if uvIndexHasNativeCredentials(idx.URL, userKey) {
			log.Info(fmt.Sprintf("UV auth [index %q]: native credentials already present (env var, embedded URL, or netrc)", idx.Name))
		} else {
			hostMatches := uvHostMatchesServer(idx.URL, serverDetails.ArtifactoryUrl)
			explicitServerID := c.serverID != ""
			if hostMatches || explicitServerID {
				_ = os.Setenv(userKey, user)
				_ = os.Setenv(uvIndexPasswordKey(envName), pass)
				injectedAny = true
				if explicitServerID && !hostMatches {
					log.Info(fmt.Sprintf("UV auth [index %q]: injecting credentials from --server-id (host override)", idx.Name))
				} else {
					log.Info(fmt.Sprintf("UV auth [index %q]: using jf server config (fallback)", idx.Name))
				}
			} else {
				log.Warn(fmt.Sprintf(
					"UV auth [index %q]: index host (%s) differs from jf server config host (%s) — "+
						"set UV_INDEX_%s_USERNAME/PASSWORD, embed credentials in the URL, or add ~/.netrc entry for %s",
					idx.Name, uvHostOf(idx.URL), uvHostOf(serverDetails.ArtifactoryUrl),
					envName, uvHostOf(idx.URL)))
			}
		}
	}

	if injectedAny {
		_ = os.Setenv("UV_KEYRING_PROVIDER", "disabled")
	}

	if c.commandName == "publish" {
		uvApplyPublishAuth(deployerRepo, workingDir, serverDetails, user, pass, c.serverID != "")
	}
}

// runUvBinary executes the uv binary with stdio pass-through.
// uvModifiesVenv returns true for uv sub-commands that install packages into the venv,
// meaning uv pip list will reflect exactly what those flags resolved.
func uvModifiesVenv(cmdName string) bool {
	switch cmdName {
	case "sync", "install", "add", "remove":
		return true
	}
	return false
}

// uvInstalledPackages runs `uv pip list --format=json` and returns the installed set as
// normalisedName → version. Returns nil on any error (caller falls back to lock-file logic).
func uvInstalledPackages() map[string]string {
	out, err := exec.Command("uv", "pip", "list", "--format=json").Output()
	if err != nil {
		log.Debug(fmt.Sprintf("uv pip list failed, falling back to lock-file dep resolution: %v", err))
		return nil
	}
	var list []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		log.Debug(fmt.Sprintf("failed to parse uv pip list output: %v", err))
		return nil
	}
	installed := make(map[string]string, len(list))
	for _, p := range list {
		installed[strings.ToLower(strings.ReplaceAll(p.Name, "_", "-"))] = p.Version
	}
	return installed
}

// uvScriptPath returns the script file path when --script <path> is present in
// a "run" invocation, or "" for all other commands.
func uvScriptPath(cmdName string, args []string) string {
	if cmdName != "run" {
		return ""
	}
	for i, a := range args {
		if a == "--script" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--script=") {
			return strings.TrimPrefix(a, "--script=")
		}
	}
	return ""
}

// uvGetScriptBuildInfo collects build-info for a PEP 723 inline script by:
//  1. Ensuring a <script>.lock file exists (runs `uv lock --script` if needed).
//  2. Parsing that lock file with UVFlexPack using the script name as module ID.
//
// This gives accurate dependency tracking for the script's isolated environment
// rather than reporting the surrounding project's dependencies.
func uvGetScriptBuildInfo(scriptPath string, buildConfiguration *buildUtils.BuildConfiguration, _ *coreConfig.ServerDetails) error {
	lockPath := scriptPath + ".lock"
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		log.Info(fmt.Sprintf("UV script: creating lock file at %s", lockPath))
		if err := runUvBinary([]string{"lock", "--script", scriptPath}); err != nil {
			return fmt.Errorf("uv lock --script failed: %w", err)
		}
	}
	buildName, err := buildConfiguration.GetBuildName()
	if err != nil {
		return fmt.Errorf("GetBuildName failed: %w", err)
	}
	buildNumber, err := buildConfiguration.GetBuildNumber()
	if err != nil {
		return fmt.Errorf("GetBuildNumber failed: %w", err)
	}
	scriptName := strings.TrimSuffix(filepath.Base(scriptPath), filepath.Ext(scriptPath))
	collector, err := flexpack.NewUVFlexPack(flexpack.UVConfig{
		WorkingDirectory:       filepath.Dir(scriptPath),
		LockFilePath:           lockPath,
		ProjectName:            scriptName,
		IncludeDevDependencies: true, // scripts have no dev/main distinction
	})
	if err != nil {
		return fmt.Errorf("failed to create UV FlexPack for script: %w", err)
	}
	bi, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect script build info: %w", err)
	}
	directURLDeps := collector.GetDirectURLDeps()
	if len(directURLDeps) > 0 && len(bi.Modules) > 0 {
		uvEnrichDirectURLChecksums(bi.Modules[0].Dependencies, directURLDeps)
	}
	service := buildUtils.CreateBuildInfoService()
	bld, err := service.GetOrCreateBuildWithProject(bi.Name, bi.Number, buildConfiguration.GetProject())
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}
	if err := bld.SaveBuildInfo(bi); err != nil {
		return err
	}
	log.Info(fmt.Sprintf("UV script build info collected. Use 'jf rt bp %s %s' to publish.", buildName, buildNumber))
	return nil
}

// uvScriptInlineIndexes returns [[tool.uv.index]] entries parsed from the PEP 723
// inline metadata block (# /// script ... ///) of the script file referenced by
// the --script flag in args. Returns nil when not a --script invocation or on any error.
func uvScriptInlineIndexes(cmdName string, args []string) []uvIndexEntry {
	if cmdName != "run" {
		return nil
	}
	// Find --script <path> in args
	scriptPath := ""
	for i, a := range args {
		if a == "--script" && i+1 < len(args) {
			scriptPath = args[i+1]
			break
		}
		if strings.HasPrefix(a, "--script=") {
			scriptPath = strings.TrimPrefix(a, "--script=")
			break
		}
	}
	if scriptPath == "" {
		return nil
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil
	}
	content := string(data)
	// Locate # /// script ... # /// block
	const startMarker = "# /// script"
	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return nil
	}
	afterStart := content[startIdx+len(startMarker):]
	endIdx := strings.Index(afterStart, "# ///")
	if endIdx == -1 {
		return nil
	}
	// Strip leading "# " from each line to get raw TOML
	var tomlLines []string
	for _, line := range strings.Split(afterStart[:endIdx], "\n") {
		stripped := strings.TrimPrefix(line, "# ")
		if stripped == "#" {
			stripped = ""
		}
		tomlLines = append(tomlLines, stripped)
	}
	var meta uvPyprojectToml
	if err := toml.Unmarshal([]byte(strings.Join(tomlLines, "\n")), &meta); err != nil {
		return nil
	}
	return meta.Tool.Uv.Index
}

// uvShouldIncludeDevDeps mirrors uv's own default: dev dependencies are installed
// unless --no-dev is explicitly passed. Only commands that resolve the environment
// (sync/install/lock/add/remove) have a dev/group concept; others default to false.
func uvShouldIncludeDevDeps(cmdName string, args []string) bool {
	switch cmdName {
	case "sync", "install", "lock", "add", "remove":
		// uv's default: dev deps are included unless the caller opts out
	default:
		return false
	}
	for _, a := range args {
		if a == "--no-dev" {
			return false
		}
	}
	// --only-dev, --group, --only-group, --all-groups all still resolve dev deps
	return true
}

// isHelpRequest returns true when the user is asking for uv help (-h / --help anywhere
// in the args, or bare "help" sub-command). In these cases we must skip all auth injection
// so that credential env vars are never set — uv prints their values in help output.
func isHelpRequest(cmdName string, args []string) bool {
	if cmdName == "help" || cmdName == "" {
		return true
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func runUvBinary(args []string) error {
	cmd := exec.Command("uv", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ── TOML types ───────────────────────────────────────────────────────────────

type uvIndexEntry struct {
	Name string `toml:"name"`
	URL  string `toml:"url"`
}

type uvToolUv struct {
	PublishURL string         `toml:"publish-url"`
	Index      []uvIndexEntry `toml:"index"`
}

type uvPyprojectToml struct {
	Tool struct {
		Uv uvToolUv `toml:"uv"`
	} `toml:"tool"`
}

// parseUvPyproject reads and parses pyproject.toml from workingDir.
// Returns a zero-value struct on any error (missing file is normal).
func parseUvPyproject(workingDir string) uvPyprojectToml {
	data, err := os.ReadFile(filepath.Join(workingDir, "pyproject.toml"))
	if err != nil {
		return uvPyprojectToml{}
	}
	var p uvPyprojectToml
	if err := toml.Unmarshal(data, &p); err != nil {
		return uvPyprojectToml{}
	}
	return p
}


func uvReadIndexesFromToml(workingDir string) []uvIndexEntry {
	p := parseUvPyproject(workingDir)
	var entries []uvIndexEntry
	for _, idx := range p.Tool.Uv.Index {
		if idx.Name != "" {
			entries = append(entries, idx)
		}
	}
	return entries
}

// ── Credential helpers ────────────────────────────────────────────────────────

// uvIndexHasNativeCredentials returns true when any native UV mechanism
// (env var, embedded URL, netrc) already provides credentials for the index,
// so jf config injection should be skipped.
func uvIndexHasNativeCredentials(indexURL, envVarUsername string) bool {
	if os.Getenv(envVarUsername) != "" {
		return true
	}
	if uvURLHasEmbeddedCredentials(indexURL) {
		return true
	}
	return uvNetrcHasCredentials(indexURL)
}

// uvHostOf returns the hostname from a URL, or empty string on parse error.
func uvHostOf(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// uvHostMatchesServer returns true when publishURL's hostname equals the Artifactory
// server's hostname, preventing credential injection onto a different instance.
func uvHostMatchesServer(publishURL, serverURL string) bool {
	return uvHostOf(publishURL) != "" && uvHostOf(publishURL) == uvHostOf(serverURL)
}

// uvMatchingIndexCredentials looks for a [[tool.uv.index]] whose URL shares the
// same hostname as publishURL and returns credentials from the first matching source.
func uvMatchingIndexCredentials(publishURL, workingDir string) (username, password string) {
	publishHost := uvHostOf(publishURL)
	if publishHost == "" {
		return "", ""
	}
	for _, idx := range uvReadIndexesFromToml(workingDir) {
		if uvHostOf(idx.URL) != publishHost {
			continue
		}
		envName := uvIndexEnvName(idx.Name)
		u := os.Getenv(uvIndexUsernameKey(envName))
		p := os.Getenv(uvIndexPasswordKey(envName))
		if u != "" {
			return u, p
		}
		if parsed, err := url.Parse(idx.URL); err == nil && parsed.User != nil {
			u = parsed.User.Username()
			p, _ = parsed.User.Password()
			if u != "" {
				return u, p
			}
		}
	}
	return "", ""
}

// uvURLHasEmbeddedCredentials returns true when the URL contains a userinfo component.
func uvURLHasEmbeddedCredentials(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return parsed.User != nil && parsed.User.Username() != ""
}

// uvNetrcPath returns the effective netrc file path (respecting the NETRC env var).
func uvNetrcPath() string {
	if custom := os.Getenv("NETRC"); custom != "" {
		return custom
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".netrc")
}

// uvNetrcHasCredentials returns true when the netrc file contains a `machine <host>`
// entry for the hostname of rawURL.
func uvNetrcHasCredentials(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := parsed.Hostname()

	data, err := os.ReadFile(uvNetrcPath())
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "machine" && fields[1] == host {
			return true
		}
	}
	return false
}

// uvIndexEnvName converts a UV index name to the env var suffix UV expects.
// e.g. "agrasth-uv-local" → "AGRASTH_UV_LOCAL"
func uvIndexEnvName(name string) string {
	upper := strings.ToUpper(name)
	return strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(upper)
}

// uvIndexUsernameKey returns the UV_INDEX_<NAME>_USERNAME env var name for a given index env suffix.
func uvIndexUsernameKey(envName string) string { return "UV_INDEX_" + envName + "_USERNAME" }

// uvIndexPasswordKey returns the UV_INDEX_<NAME>_PASSWORD env var name for a given index env suffix.
func uvIndexPasswordKey(envName string) string { return "UV_INDEX_" + envName + "_PASSWORD" }

// uvResolveServerDetails returns server details for the given server ID.
// If serverID is empty, the default configured server is used.
func uvResolveServerDetails(serverID string) (*coreConfig.ServerDetails, error) {
	if serverID == "" {
		return coreConfig.GetDefaultServerConf()
	}
	return coreConfig.GetSpecificConfig(serverID, true, true)
}

// uvApplyPublishAuth selects and applies publish credentials following UV's priority chain.
// explicitServerID is true when the user passed --server-id explicitly; in that case the
// host-mismatch check is bypassed so credentials are always injected for the publish URL.
func uvApplyPublishAuth(publishURL, workingDir string, serverDetails *coreConfig.ServerDetails, user, pass string, explicitServerID bool) {
	switch {
	case os.Getenv("UV_PUBLISH_TOKEN") != "":
		log.Debug("UV auth [publish]: using UV_PUBLISH_TOKEN")
	case os.Getenv("UV_PUBLISH_USERNAME") != "" || os.Getenv("UV_PUBLISH_PASSWORD") != "":
		log.Debug("UV auth [publish]: using UV_PUBLISH_USERNAME/PASSWORD")
	case uvURLHasEmbeddedCredentials(publishURL):
		log.Info("UV auth [publish]: using credentials embedded in publish URL (native, step 3)")
	case uvNetrcHasCredentials(publishURL):
		log.Info(fmt.Sprintf("UV auth [publish]: using %s (native, step 4)", uvNetrcPath()))
	default:
		uvInjectPublishCredentials(publishURL, workingDir, serverDetails, user, pass, explicitServerID)
	}
}

// uvInjectPublishCredentials injects publish credentials from same-host index credentials
// or the jf server config. When explicitServerID is true, the host-mismatch check is skipped.
func uvInjectPublishCredentials(publishURL, workingDir string, serverDetails *coreConfig.ServerDetails, user, pass string, explicitServerID bool) {
	if idxUser, idxPass := uvMatchingIndexCredentials(publishURL, workingDir); idxUser != "" {
		_ = os.Setenv("UV_PUBLISH_USERNAME", idxUser)
		_ = os.Setenv("UV_PUBLISH_PASSWORD", idxPass)
		_ = os.Setenv("UV_KEYRING_PROVIDER", "disabled")
		log.Info("UV auth [publish]: reusing same-host index credentials for publish")
		return
	}
	if uvHostMatchesServer(publishURL, serverDetails.ArtifactoryUrl) || explicitServerID {
		_ = os.Setenv("UV_PUBLISH_USERNAME", user)
		_ = os.Setenv("UV_PUBLISH_PASSWORD", pass)
		_ = os.Setenv("UV_KEYRING_PROVIDER", "disabled")
		if explicitServerID && !uvHostMatchesServer(publishURL, serverDetails.ArtifactoryUrl) {
			log.Info("UV auth [publish]: injecting credentials from --server-id (host override)")
		} else {
			log.Info("UV auth [publish]: using jf server config (fallback)")
		}
		return
	}
	log.Warn(fmt.Sprintf(
		"UV auth [publish]: publish URL host (%s) does not match jf server config host (%s) — "+
			"set UV_PUBLISH_USERNAME/UV_PUBLISH_PASSWORD or UV_PUBLISH_TOKEN to authenticate",
		uvHostOf(publishURL), uvHostOf(serverDetails.ArtifactoryUrl)))
}

// ── Build info collection ────────────────────────────────────────────────────

// uvGetBuildInfo collects build info for UV projects using the FlexPack native implementation.
func uvGetBuildInfo(workingDir string, buildConfiguration *buildUtils.BuildConfiguration, deployerRepo, cmdName string, args []string, installed map[string]string, serverDetails *coreConfig.ServerDetails) error {
	log.Debug(fmt.Sprintf("Collecting UV build info for command '%s' in: %s", cmdName, workingDir))

	buildName, err := buildConfiguration.GetBuildName()
	if err != nil {
		return fmt.Errorf("GetBuildName failed: %w", err)
	}
	buildNumber, err := buildConfiguration.GetBuildNumber()
	if err != nil {
		return fmt.Errorf("GetBuildNumber failed: %w", err)
	}

	uvConfig := flexpack.UVConfig{
		WorkingDirectory:       workingDir,
		InstalledPackages:      installed, // nil for lock/build/publish → fallback to flag-based logic
		IncludeDevDependencies: uvShouldIncludeDevDeps(cmdName, args),
	}
	collector, err := flexpack.NewUVFlexPack(uvConfig)
	if err != nil {
		return fmt.Errorf("failed to create UV FlexPack collector: %w", err)
	}

	bi, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect UV build info: %w", err)
	}

	if customModule := buildConfiguration.GetModule(); customModule != "" && len(bi.Modules) > 0 {
		bi.Modules[0].Id = customModule
	}

	// Collect direct-URL deps (source = { url = "..." } in uv.lock) — these are not in
	// Artifactory so AQL enrichment must skip them; sha256 from the lock file is sufficient.
	directURLDeps := collector.GetDirectURLDeps()
	if len(directURLDeps) > 0 && len(bi.Modules) > 0 {
		uvEnrichDirectURLChecksums(bi.Modules[0].Dependencies, directURLDeps)
	}

	switch cmdName {
	case "sync", "install", "lock", "add", "remove", "run":
		if len(bi.Modules) > 0 && len(bi.Modules[0].Dependencies) > 0 {
			// Resolve enrichment repo: prefer [[tool.uv.index]] in pyproject.toml,
			// fall back to UV_DEFAULT_INDEX / UV_INDEX_URL env vars (used when no
			// pyproject.toml index is configured, e.g. CI workflows that inject creds
			// via environment).
			repoKey := uvResolverRepoFromToml(workingDir)
			indexURL := uvIndexURLFromToml(workingDir)
			if repoKey == "" {
				for _, envVar := range []string{"UV_DEFAULT_INDEX", "UV_INDEX_URL"} {
					if val := os.Getenv(envVar); val != "" {
						repoKey = uvExtractRepoKeyFromURL(val)
						indexURL = val
						if repoKey != "" {
							log.Debug(fmt.Sprintf("UV build-info: using %s for dependency enrichment repo: %s", envVar, repoKey))
							break
						}
					}
				}
			}
			if repoKey != "" {
				sd := serverDetails
				if sd == nil {
					sd, _ = coreConfig.GetDefaultServerConf()
				}
				if sd != nil {
					if indexURL != "" && !uvHostMatchesServer(indexURL, sd.ArtifactoryUrl) {
						log.Warn(fmt.Sprintf(
							"UV build-info: jf server config host (%s) differs from index URL host (%s) — "+
								"dependency checksum enrichment (sha1/md5) will be skipped. "+
								"Use --server-id to specify the Artifactory instance that hosts your uv packages.",
							uvHostOf(sd.ArtifactoryUrl), uvHostOf(indexURL)))
					}
					uvEnrichDepsFromArtifactory(bi.Modules[0].Dependencies, repoKey, directURLDeps, sd)
				}
			}
		}
	}

	switch cmdName {
	case "publish":
		repoKey := uvExtractRepoKeyFromURL(deployerRepo)
		sd := serverDetails
		if sd == nil {
			var sdErr error
			sd, sdErr = coreConfig.GetDefaultServerConf()
			if sdErr != nil {
				log.Warn("Could not load server config for artifact lookup: " + sdErr.Error())
				sd = nil
			}
		}
		if sd == nil {
			if artifacts, scanErr := uvCollectDistArtifacts(workingDir); scanErr == nil && len(bi.Modules) > 0 {
				bi.Modules[0].Artifacts = artifacts
			}
			break
		}
		if repoKey != "" {
			if deployerRepo != "" && !uvHostMatchesServer(deployerRepo, sd.ArtifactoryUrl) {
				log.Warn(fmt.Sprintf(
					"UV build-info: jf server config host (%s) differs from publish URL host (%s) — "+
						"artifact lookup and build property setting will be skipped. "+
						"Use --server-id to specify the Artifactory instance that hosts your uv packages.",
					uvHostOf(sd.ArtifactoryUrl), uvHostOf(deployerRepo)))
			} else {
				if artErr := uvAddArtifactsToBuildInfo(bi, sd, repoKey, workingDir); artErr != nil {
					log.Warn("Could not look up artifact repo paths, using local checksums: " + artErr.Error())
					if artifacts, scanErr := uvCollectDistArtifacts(workingDir); scanErr == nil && len(bi.Modules) > 0 {
						bi.Modules[0].Artifacts = artifacts
					}
				}
				if propErr := uvSetBuildProperties(sd, repoKey, buildName, buildNumber, buildConfiguration.GetProject(), bi); propErr != nil {
					log.Warn("Failed to set build properties on artifacts: " + propErr.Error())
				}
			}
		} else {
			if artifacts, scanErr := uvCollectDistArtifacts(workingDir); scanErr == nil && len(artifacts) > 0 {
				if len(bi.Modules) > 0 {
					bi.Modules[0].Artifacts = artifacts
					log.Info(fmt.Sprintf("Collected %d artifact(s) from dist/ (no deployer repo set)", len(artifacts)))
				}
			}
		}
	}

	if err = uvSaveBuildInfo(bi, buildConfiguration); err != nil {
		return fmt.Errorf("failed to save UV build info: %w", err)
	}

	log.Info(fmt.Sprintf("UV build info collected. Use 'jf rt bp %s %s' to publish.", buildName, buildNumber))
	return nil
}

// uvResolverRepoFromToml extracts the Artifactory repo key from the first
// [[tool.uv.index]] URL in pyproject.toml.
func uvResolverRepoFromToml(workingDir string) string {
	for _, idx := range uvReadIndexesFromToml(workingDir) {
		if idx.URL != "" {
			return uvExtractRepoKeyFromURL(idx.URL)
		}
	}
	return ""
}

// uvIndexURLFromToml returns the raw URL of the first [[tool.uv.index]] entry.
func uvIndexURLFromToml(workingDir string) string {
	for _, idx := range uvReadIndexesFromToml(workingDir) {
		if idx.URL != "" {
			return idx.URL
		}
	}
	return ""
}

// uvEnrichDepsFromArtifactory fetches sha1/md5 for all dependencies in a single batched AQL call.
//
// uvEnrichDirectURLChecksums fetches sha1/md5 for direct-URL deps by streaming each file.
// Git deps are skipped (no downloadable archive). Unreachable URLs retain sha256-only.
func uvEnrichDirectURLChecksums(deps []buildinfo.Dependency, directURLDeps map[string]string) {
	for i, dep := range deps {
		sourceURL, ok := directURLDeps[dep.Id]
		if !ok || dep.Sha1 != "" {
			continue
		}
		// Skip git URLs — no deterministic archive to download
		if strings.HasPrefix(sourceURL, "git+") || strings.Contains(sourceURL, ".git") ||
			strings.HasPrefix(sourceURL, "git://") {
			log.Info(fmt.Sprintf("UV build-info: dep %s is a git dep (%s) — sha256 available, sha1/md5 not computable without rebuilding", dep.Id, sourceURL))
			continue
		}
		if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
			log.Info(fmt.Sprintf("UV build-info: dep %s is from a direct URL (%s) — sha256 available, sha1/md5 not enrichable from Artifactory", dep.Id, sourceURL))
			continue
		}
		// Stream the file and compute sha1 + md5 in a single pass — no disk write needed.
		resp, err := http.Get(sourceURL) // #nosec G107 -- URL is from uv.lock, not user input
		if err != nil {
			log.Info(fmt.Sprintf("UV build-info: dep %s direct URL not reachable (%v) — sha256 only", dep.Id, err))
			continue
		}
		sha1w := sha1.New() // #nosec G401 -- sha1 used for Artifactory build-info checksums, not security
		md5w := md5.New()   // #nosec G401
		_, copyErr := io.Copy(io.MultiWriter(sha1w, md5w), resp.Body)
		_ = resp.Body.Close()
		if copyErr != nil {
			log.Info(fmt.Sprintf("UV build-info: dep %s could not stream URL — sha256 only", dep.Id))
			continue
		}
		deps[i].Sha1 = fmt.Sprintf("%x", sha1w.Sum(nil))
		deps[i].Md5 = fmt.Sprintf("%x", md5w.Sum(nil))
		log.Info(fmt.Sprintf("UV build-info: dep %s enriched sha1/md5 from direct URL", dep.Id))
	}
}

// uvEnrichDepsFromArtifactory fetches sha1/md5 for all registry-based dependencies in a single
// batched AQL call. directURLDeps maps dep ID → source URL for deps that were installed from
// a direct URL — these are skipped since they are not in Artifactory.
func uvEnrichDepsFromArtifactory(deps []buildinfo.Dependency, repoKey string, directURLDeps map[string]string, serverDetails *coreConfig.ServerDetails) {
	if len(deps) == 0 {
		return
	}
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		log.Warn("Could not create services manager for dependency enrichment: " + err.Error())
		return
	}
	searchRepo, err := utils.GetRepoNameForDependenciesSearch(repoKey, servicesManager)
	if err != nil {
		log.Warn("Could not resolve repo for dependency search, using as-is: " + err.Error())
		searchRepo = repoKey
	}

	// Build (depIndex, filePrefix) pairs. dep.Id is "name:version".
	//
	// PyPI wheel filename format: <name>-<version>-<python>-<abi>-<platform>.whl
	// The name portion normalises hyphens/dots to underscores (PEP 427), but the
	// separator between name and version is ALWAYS a hyphen. So for:
	//   charset-normalizer:3.4.7  →  prefix "charset-normalizer-3.4.7"   (hyphenated name)
	//                               prefix "charset_normalizer-3.4.7"   (underscored name, correct!)
	// Replacing ALL hyphens (the old approach) produced "charset_normalizer_3.4.7"
	// which never matches any real filename.
	type depEntry struct {
		idx    int
		prefix string // "name-version" used to match result filenames
	}
	var entries []depEntry
	for i, dep := range deps {
		if dep.Id == "" {
			continue
		}
		if _, isDirectURL := directURLDeps[dep.Id]; isDirectURL {
			continue // not in Artifactory — sha256 from uv.lock is the only available checksum
		}
		// Split "name:version" — version separator is always ":"
		colonIdx := strings.Index(dep.Id, ":")
		if colonIdx < 0 {
			continue
		}
		name, version := dep.Id[:colonIdx], dep.Id[colonIdx+1:]
		// Prefix with original name (e.g. "charset-normalizer-3.4.7")
		prefixHyphen := name + "-" + version
		// Prefix with underscored name only (e.g. "charset_normalizer-3.4.7")
		// The version separator before the version tag is always "-", never "_".
		prefixUnderscore := strings.ReplaceAll(name, "-", "_") + "-" + version
		entries = append(entries, depEntry{i, prefixHyphen})
		if prefixUnderscore != prefixHyphen {
			entries = append(entries, depEntry{i, prefixUnderscore})
		}
	}
	if len(entries) == 0 {
		return
	}

	// One AQL query: $or of wildcard name patterns, one per (name, version) variant.
	var orClauses []string
	seen := make(map[string]bool)
	for _, e := range entries {
		if seen[e.prefix] {
			continue
		}
		seen[e.prefix] = true
		orClauses = append(orClauses, fmt.Sprintf(
			`{"$and":[{"path":{"$match":"*"},"name":{"$match":%q}}]}`,
			e.prefix+"*",
		))
	}
	aqlQuery := fmt.Sprintf(
		`items.find({"repo":%q,"$or":[%s]}).include("name","actual_sha1","actual_md5","sha256")`,
		searchRepo, strings.Join(orClauses, ","),
	)

	stream, err := servicesManager.Aql(aqlQuery)
	if err != nil {
		log.Debug(fmt.Sprintf("Batch AQL enrichment failed for repo %s: %v", searchRepo, err))
		return
	}
	raw, _ := io.ReadAll(stream)
	_ = stream.Close()

	var aqlResult struct {
		Results []struct {
			Name       string `json:"name"`
			ActualSha1 string `json:"actual_sha1"`
			ActualMd5  string `json:"actual_md5"`
			Sha256     string `json:"sha256"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &aqlResult); err != nil {
		log.Debug(fmt.Sprintf("Failed to parse AQL enrichment response: %v", err))
		return
	}

	// Match each result back to its dep by filename prefix. A result file like
	// "certifi-2026.4.22-py3-none-any.whl" starts with prefix "certifi-2026.4.22"
	// followed by "-" (wheel) or "." (sdist).
	enriched := 0
	for _, r := range aqlResult.Results {
		if r.ActualSha1 == "" {
			continue
		}
		for _, e := range entries {
			if deps[e.idx].Sha1 != "" {
				continue // already enriched
			}
			if strings.HasPrefix(r.Name, e.prefix+"-") || strings.HasPrefix(r.Name, e.prefix+".") {
				deps[e.idx].Sha1 = r.ActualSha1
				deps[e.idx].Md5 = r.ActualMd5
				if r.Sha256 != "" && deps[e.idx].Sha256 == "" {
					deps[e.idx].Sha256 = r.Sha256
				}
				enriched++
				break
			}
		}
	}

	if enriched > 0 {
		log.Info(fmt.Sprintf("Enriched %d/%d UV dependencies with Artifactory checksums (repo: %s)", enriched, len(deps), searchRepo))
	} else {
		log.Debug(fmt.Sprintf("No UV dependencies enriched from repo %s — packages may not be cached yet", searchRepo))
	}
}

// uvExtractRepoKeyFromURL returns just the repo key from a full Artifactory URL or a bare key.
func uvExtractRepoKeyFromURL(repoOrURL string) string {
	if repoOrURL == "" {
		return ""
	}
	if !strings.HasPrefix(repoOrURL, "http://") && !strings.HasPrefix(repoOrURL, "https://") {
		return repoOrURL
	}
	parsed, err := url.Parse(repoOrURL)
	if err != nil {
		return repoOrURL
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, seg := range segments {
		if seg == "api" && i+2 < len(segments) {
			return segments[i+2]
		}
	}
	for i := len(segments) - 1; i >= 0; i-- {
		if segments[i] != "" && segments[i] != "simple" {
			return segments[i]
		}
	}
	return repoOrURL
}

// uvCollectDistArtifacts collects wheel/sdist artifacts from the dist/ directory.
func uvCollectDistArtifacts(workingDir string) ([]buildinfo.Artifact, error) {
	distDir := filepath.Join(workingDir, "dist")
	if _, err := os.Stat(distDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("dist directory not found: %s", distDir)
	}
	entries, err := os.ReadDir(distDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read dist directory: %w", err)
	}

	var artifacts []buildinfo.Artifact
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if !strings.HasSuffix(filename, ".whl") && !strings.HasSuffix(filename, ".tar.gz") {
			continue
		}
		artifact := buildinfo.Artifact{
			Name: filename,
			Path: ".",
			Type: getArtifactType(filename),
		}
		if checksums, csErr := uvFileChecksums(filepath.Join(distDir, filename)); csErr == nil {
			artifact.Checksum = checksums
		} else {
			log.Warn(fmt.Sprintf("Failed to calculate checksums for %s: %v", filename, csErr))
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

// uvFileChecksums calculates SHA1, SHA256, and MD5 checksums for a file.
func uvFileChecksums(filePath string) (buildinfo.Checksum, error) {
	fileDetails, err := crypto.GetFileDetails(filePath, true)
	if err != nil {
		return buildinfo.Checksum{}, fmt.Errorf("failed to calculate checksums: %w", err)
	}
	return buildinfo.Checksum{
		Sha1:   fileDetails.Checksum.Sha1,
		Sha256: fileDetails.Checksum.Sha256,
		Md5:    fileDetails.Checksum.Md5,
	}, nil
}

// uvAddArtifactsToBuildInfo looks up uploaded artifacts in Artifactory and adds them
// to the build info module.
func uvAddArtifactsToBuildInfo(bi *buildinfo.BuildInfo, serverDetails *coreConfig.ServerDetails, targetRepo, workingDir string) error {
	if len(bi.Modules) == 0 {
		return fmt.Errorf("no modules found in build info")
	}
	localArtifacts, err := uvCollectDistArtifacts(workingDir)
	if err != nil {
		return fmt.Errorf("failed to get local artifacts: %w", err)
	}
	if len(localArtifacts) == 0 {
		return nil
	}

	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	var artifacts []buildinfo.Artifact
	for _, localArtifact := range localArtifacts {
		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Aql: specutils.Aql{
					ItemsFind: uvAqlQueryForSearch(targetRepo, localArtifact.Name),
				},
			},
		}
		searchReader, err := servicesManager.SearchFiles(searchParams)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to search for artifact %s: %v", localArtifact.Name, err))
			continue
		}
		for result := new(specutils.ResultItem); searchReader.NextRecord(result) == nil; result = new(specutils.ResultItem) {
			artifacts = append(artifacts, buildinfo.Artifact{
				Name:     result.Name,
				Path:     result.Path,
				Type:     getArtifactType(result.Name),
				Checksum: localArtifact.Checksum,
			})
			break
		}
		if err := searchReader.Close(); err != nil {
			log.Warn("Failed to close search reader:", err)
		}
	}

	// Modules[0] is safe: guarded by the len(bi.Modules)==0 check at function entry.
	if len(bi.Modules) == 0 {
		return nil
	}
	bi.Modules[0].Artifacts = artifacts
	log.Info(fmt.Sprintf("Added %d artifacts to build info", len(artifacts)))
	return nil
}

// uvSetBuildProperties sets build.name / build.number properties on uploaded Python dist artifacts.
func uvSetBuildProperties(serverDetails *coreConfig.ServerDetails, targetRepo, buildName, buildNumber, project string, bi *buildinfo.BuildInfo) error {
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	if err := buildUtils.SaveBuildGeneralDetails(buildName, buildNumber, project); err != nil {
		return fmt.Errorf("SaveBuildGeneralDetails failed: %w", err)
	}
	buildProps, err := buildUtils.CreateBuildProperties(buildName, buildNumber, project)
	if err != nil {
		return fmt.Errorf("CreateBuildProperties failed: %w", err)
	}

	if len(bi.Modules) == 0 || len(bi.Modules[0].Artifacts) == 0 {
		return nil
	}
	for _, artifact := range bi.Modules[0].Artifacts {
		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Aql: specutils.Aql{
					ItemsFind: uvAqlQueryForSearch(targetRepo, artifact.Name),
				},
			},
		}
		searchReader, err := servicesManager.SearchFiles(searchParams)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to find artifact %s: %v", artifact.Name, err))
			continue
		}
		_, err = servicesManager.SetProps(services.PropsParams{Reader: searchReader, Props: buildProps})
		if closeErr := searchReader.Close(); closeErr != nil {
			log.Warn("Failed to close search reader:", closeErr)
		}
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to set properties on artifact %s: %v", artifact.Name, err))
		}
	}
	log.Info(fmt.Sprintf("Successfully set build properties on %d artifacts", len(bi.Modules[0].Artifacts)))
	return nil
}

// uvSaveBuildInfo saves the build info locally for later publishing with 'jf rt bp'.
func uvSaveBuildInfo(bi *buildinfo.BuildInfo, buildConfiguration *buildUtils.BuildConfiguration) error {
	service := buildUtils.CreateBuildInfoService()
	bld, err := service.GetOrCreateBuildWithProject(bi.Name, bi.Number, buildConfiguration.GetProject())
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}
	return bld.SaveBuildInfo(bi)
}

// uvAqlQueryForSearch returns an AQL items.find query for a file in a repo.
func uvAqlQueryForSearch(repo, file string) string {
	return fmt.Sprintf(
		`{"repo": %q, "$or": [{"$and": [{"path": {"$match": "*"}, "name": {"$match": %q}}]}]}`,
		repo, file,
	)
}
