package alpine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	biUtils "github.com/jfrog/build-info-go/build/utils"
	"github.com/jfrog/build-info-go/entities"
	artutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreutils "github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var buildInfoSubcmds = map[string]bool{
	"add":     true,
	"upgrade": true,
}

// jfFlagSet is the set of jf-specific flags that must be stripped before forwarding
// args to the native apk binary. Each flag may appear as "--flag value" or "--flag=value".
var jfFlagSet = map[string]bool{
	"--build-name":     true,
	"--build-number":   true,
	"--project":        true,
	"--module":         true,
	"--server-id":      true,
	"--repo":           true,
	"--alpine-version": true,
	"--user":           true,
	"--password":       true,
}

// ApkCommand wraps the native apk binary with credential injection and Build Info collection.
type ApkCommand struct {
	commandName        string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
	repoKey            string
	alpineVersion      string
	apkArgs            []string
	username           string
	password           string
}

// NewApkCommand constructs an ApkCommand for the given apk subcommand.
func NewApkCommand(commandName string) *ApkCommand {
	return &ApkCommand{commandName: commandName}
}

// SetArgs sets the arguments forwarded to the native apk binary.
func (apkCmd *ApkCommand) SetArgs(args []string) *ApkCommand {
	apkCmd.apkArgs = args
	return apkCmd
}

// SetServerDetails sets the Artifactory server config.
func (apkCmd *ApkCommand) SetServerDetails(serverDetails *config.ServerDetails) *ApkCommand {
	apkCmd.serverDetails = serverDetails
	return apkCmd
}

// SetBuildConfiguration sets the build configuration.
func (apkCmd *ApkCommand) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) *ApkCommand {
	apkCmd.buildConfiguration = bc
	return apkCmd
}

// SetRepo sets the Artifactory Alpine repository key.
func (apkCmd *ApkCommand) SetRepo(repoKey string) *ApkCommand {
	apkCmd.repoKey = repoKey
	return apkCmd
}

// SetAlpineVersion sets the Alpine release tag (e.g. "v3.20").
func (apkCmd *ApkCommand) SetAlpineVersion(version string) *ApkCommand {
	apkCmd.alpineVersion = version
	return apkCmd
}

func alpineModuleID(configured, repoKey, arch, alpineVersion string) string {
	if configured != "" {
		return configured
	}
	if repoKey == "" {
		repoKey = "apk"
	}
	if arch == "" {
		arch = "unknown"
	}
	if alpineVersion == "" {
		alpineVersion = "unknown"
	} else if !strings.HasPrefix(alpineVersion, "v") {
		alpineVersion = "v" + alpineVersion
	}
	return fmt.Sprintf("%s:%s:%s", repoKey, arch, alpineVersion)
}

// shouldIsolateRepo reports whether this command should target its --repo on the selected server
// via a temporary repositories file instead of /etc/apk/repositories. It isolates only when a
// --repo is set AND the selected server (serverDetails.ServerId) differs from the default server
// that /etc/apk/repositories is assumed to be configured with (via `jf setup apk`). This runs the
// one-off command against the requested server while leaving the default server's persistent
// configuration untouched.
func (apkCmd *ApkCommand) shouldIsolateRepo() bool {
	if apkCmd.repoKey == "" || apkCmd.serverDetails == nil || apkCmd.serverDetails.ServerId == "" {
		return false
	}
	defaultConf, err := config.GetDefaultServerConf()
	if err != nil || defaultConf == nil {
		// No default server to compare against — treat the explicitly selected server as isolated.
		return true
	}
	return apkCmd.serverDetails.ServerId != defaultConf.ServerId
}

// SetUsername sets the username CLI flag override.
func (apkCmd *ApkCommand) SetUsername(username string) *ApkCommand {
	apkCmd.username = username
	return apkCmd
}

// SetPassword sets the password CLI flag override.
func (apkCmd *ApkCommand) SetPassword(password string) *ApkCommand {
	apkCmd.password = password
	return apkCmd
}

// CommandName satisfies the Command interface.
func (apkCmd *ApkCommand) CommandName() string {
	return apkCmd.commandName
}

// ServerDetails satisfies the Command interface.
func (apkCmd *ApkCommand) ServerDetails() (*config.ServerDetails, error) {
	return apkCmd.serverDetails, nil
}

// Run executes the pre-exec, exec, and post-exec phases of the apk wrapper.
func (apkCmd *ApkCommand) Run() error {
	// --repo is an explicit repository selection. Validate it before invoking native apk so
	// every wrapped subcommand fails consistently and clearly for an unknown repository.
	// Skipped when no server is configured at all: apk still works against the system's
	// default repositories in that case, so there is nothing to validate against.
	if apkCmd.repoKey != "" && apkCmd.serverDetails != nil {
		if err := ensureRepoExists(apkCmd.repoKey, apkCmd.serverDetails); err != nil {
			return err
		}
	}

	apkPath, err := exec.LookPath("apk")
	if err != nil {
		return errorutils.CheckErrorf("'apk' binary not found. Is this an Alpine Linux environment?")
	}
	warnIfApkTooOld()

	collectBuildInfo, err := apkCmd.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	needsBuildInfo := buildInfoSubcmds[apkCmd.commandName] && collectBuildInfo

	nativeArgs := stripJFFlags(apkCmd.apkArgs)
	requestedPkgs := extractPackageNames(nativeArgs)

	noCache := containsFlag(nativeArgs, "--no-cache")
	userCacheDir := flagValue(nativeArgs, "--cache-dir")

	var preSnapshot []biUtils.AlpinePackage
	if needsBuildInfo {
		preSnapshot, err = biUtils.ListInstalledPackages()
		if err != nil {
			log.Warn("Cannot list installed packages — Build Info not captured:", err)
			needsBuildInfo = false
		}
	}

	env, err := apkCmd.buildEnvWithHTTPAuth()
	if err != nil {
		return err
	}

	var cacheDir string
	var ownCacheDir bool
	switch {
	case noCache:
		if needsBuildInfo {
			log.Warn("--no-cache prevents apk from keeping the downloaded archives, so checksums " +
				"cannot be computed locally and will only be filled for packages found in the " +
				"Artifactory repository. Drop --no-cache for complete Build Info.")
		}
		log.Debug("--no-cache detected: skipping temp cache dir; local checksums require a cached .apk.")
	case userCacheDir != "":
		cacheDir = userCacheDir
		log.Debug("Reusing caller-provided --cache-dir for checksum collection:", cacheDir)
	case needsBuildInfo:
		cacheDir, err = createTempDir()
		if err != nil {
			log.Warn("Could not create temp cache dir — checksums may be incomplete:", err)
		} else {
			ownCacheDir = true
		}
	}

	if apkCmd.shouldIsolateRepo() {
		repoFile, buildErr := apkCmd.writeIsolatedRepositoriesFile()
		if buildErr != nil {
			log.Warn("Could not build an isolated repositories file for --repo/--server-id; falling back to /etc/apk/repositories:", buildErr)
		} else {
			defer func() { _ = os.Remove(repoFile) }()
			nativeArgs = append([]string{"--repositories-file", repoFile}, nativeArgs...)
			log.Debug("Using isolated repositories file for this command:", repoFile)
		}
	}

	args := nativeArgs
	if ownCacheDir && cacheDir != "" {
		args = append([]string{"--cache-dir", cacheDir}, args...)
	}
	fullArgs := append([]string{apkCmd.commandName}, args...)
	exitCode, err := runWithPackageManager(apkPath, fullArgs, env)
	if ownCacheDir && cacheDir != "" && (err != nil || exitCode != 0) {
		_ = os.RemoveAll(cacheDir)
	}
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return coreutils.CliError{ExitCode: coreutils.ExitCode{Code: exitCode}}
	}
	defer func() {
		if ownCacheDir {
			_ = os.RemoveAll(cacheDir)
		}
	}()

	if collectBuildInfo && !buildInfoSubcmds[apkCmd.commandName] {
		log.Warn(unsupportedBuildInfoMessage(apkCmd.commandName))
		return nil
	}
	if !needsBuildInfo {
		return nil
	}
	downloadsDir := ""
	if ownCacheDir {
		downloadsDir = cacheDir
	}
	apkCmd.collectBuildInfo(preSnapshot, cacheDir, downloadsDir, requestedPkgs)
	return nil
}

func unsupportedBuildInfoMessage(commandName string) string {
	return fmt.Sprintf(
		"Build Info flags were provided, but Build Info collection is not available for 'apk %s'. "+
			"Only 'add', 'upgrade', and 'upload' support Build Info. The command was executed as a passthrough.",
		commandName,
	)
}

// buildEnvWithHTTPAuth returns the current process environment with HTTP_AUTH injected for the apk subprocess.
func (apkCmd *ApkCommand) buildEnvWithHTTPAuth() ([]string, error) {
	env := os.Environ()

	rtURL := ""
	if apkCmd.serverDetails != nil {
		rtURL = apkCmd.serverDetails.GetArtifactoryUrl()
	}

	// If there is no server config but --user/--password + --repo are provided, try to derive
	// the Artifactory URL from /etc/apk/repositories so we can still build a valid HTTP_AUTH
	// value without requiring jf c add to have been run.
	if rtURL == "" && (apkCmd.username != "" || apkCmd.password != "") {
		if apkCmd.repoKey != "" {
			if repoURL, err := readRepoURLFromRepositoriesFile(apkCmd.repoKey); err == nil && repoURL != "" {
				rtURL = repoURL
				log.Debug("HTTP_AUTH: derived Artifactory URL from /etc/apk/repositories:", rtURL)
			}
		}
		if rtURL == "" {
			log.Warn("--user/--password provided but no Artifactory URL could be determined. Use --server-id or run jf setup apk first.")
			return env, nil
		}
	}

	if rtURL == "" {
		log.Warn("No JFrog server configured — skipping HTTP_AUTH injection. Run: jf c add")
		return env, nil
	}

	// Filter out env vars matching the JFROG_CLI_ENV_EXCLUDE pattern to avoid secret leaks
	// in the subprocess environment.
	env = filterSecretEnvVars(env)

	// Explicit --user/--password flags override any pre-set HTTP_AUTH; a stored/default
	// server config does not, so a user-provided HTTP_AUTH is otherwise honoured as-is.
	userExplicitFlags := apkCmd.username != "" || apkCmd.password != ""

	existingHTTPAuth := os.Getenv("HTTP_AUTH")
	if existingHTTPAuth != "" {
		if userExplicitFlags {
			log.Warn("HTTP_AUTH is already set in your environment. Overriding it with the credentials from the provided flags/server config.")
		} else {
			// User did not pass explicit flags — honour their pre-set HTTP_AUTH.
			log.Debug("HTTP_AUTH already set in environment and no explicit flags provided — keeping existing value.")
			return env, nil
		}
	}

	var username, password string
	if apkCmd.serverDetails != nil {
		username, password = resolveHTTPAuthCredentials(apkCmd.serverDetails, apkCmd.username, apkCmd.password)
	} else {
		// No server config — use the explicitly provided flags directly.
		username, password = apkCmd.username, apkCmd.password
	}

	httpAuth, err := buildHTTPAuth(rtURL, username, password)
	if err != nil {
		return nil, err
	}

	log.Debug("HTTP_AUTH=basic:<host>:<user>:***")
	env = append(env, "HTTP_AUTH="+httpAuth)
	return env, nil
}

// readRepoURLFromRepositoriesFile scans /etc/apk/repositories and returns the first URL
// that contains repoKey as a path segment, so we can derive the Artifactory hostname when
// no server config is present but --repo + --user/--password were provided.
func readRepoURLFromRepositoriesFile(repoKey string) (string, error) {
	f, err := os.Open("/etc/apk/repositories")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "/"+repoKey+"/") || strings.HasSuffix(line, "/"+repoKey) {
			return line, nil
		}
	}
	return "", scanner.Err()
}

// writeIsolatedRepositoriesFile writes a temporary apk repositories file containing only the
// selected server's Alpine repo (with credentials embedded in the URL). It is passed to apk via
// --repositories-file so a single command targets that server/repo without reading or mutating
// the persistent /etc/apk/repositories. The caller is responsible for removing the returned path.
func (apkCmd *ApkCommand) writeIsolatedRepositoriesFile() (string, error) {
	if apkCmd.serverDetails == nil {
		return "", errorutils.CheckErrorf("no server configured for isolated repository access")
	}
	if apkCmd.repoKey == "" {
		return "", errorutils.CheckErrorf("--repo is required to build an isolated repositories file")
	}
	rtURL := strings.TrimRight(apkCmd.serverDetails.GetArtifactoryUrl(), "/")
	if rtURL == "" {
		return "", errorutils.CheckErrorf("the selected server has no Artifactory URL")
	}

	// Resolve the Alpine version (flag > host /etc/alpine-release) and normalize to the
	// canonical "v"-prefixed form so the path matches what Artifactory indexes.
	version := apkCmd.alpineVersion
	if version == "" {
		version = detectSystemAlpineVersion()
	}
	if version != "" && !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	var repoURL string
	if version != "" {
		repoURL = fmt.Sprintf("%s/%s/%s/main/", rtURL, apkCmd.repoKey, version)
	} else {
		repoURL = fmt.Sprintf("%s/%s/", rtURL, apkCmd.repoKey)
	}

	// Embed credentials in the URL so apk authenticates directly from the temp file.
	username, password := resolveHTTPAuthCredentials(apkCmd.serverDetails, apkCmd.username, apkCmd.password)
	if username != "" || password != "" {
		if parsed, perr := url.Parse(repoURL); perr == nil {
			parsed.User = url.UserPassword(username, password)
			repoURL = parsed.String()
		}
	}

	f, err := os.CreateTemp("", "jf-apk-repositories-*")
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	defer func() { _ = f.Close() }()
	if _, err = f.WriteString(repoURL + "\n"); err != nil {
		_ = os.Remove(f.Name())
		return "", errorutils.CheckError(err)
	}
	// The file embeds a secret — lock it down.
	if err = os.Chmod(f.Name(), 0600); err != nil {
		_ = os.Remove(f.Name())
		return "", errorutils.CheckError(err)
	}
	return f.Name(), nil
}

// buildHTTPAuth constructs the HTTP_AUTH=basic:<host>:<user>:<password> string for apk-tools.
func buildHTTPAuth(rtURL, username, password string) (string, error) {
	parsed, err := url.Parse(rtURL)
	if err != nil {
		return "", errorutils.CheckErrorf("invalid Artifactory URL %q: %w", rtURL, err)
	}
	host := parsed.Hostname()
	return fmt.Sprintf("basic:%s:%s:%s", host, username, password), nil
}

func runWithPackageManager(apkPath string, args []string, env []string) (int, error) {
	cmd := exec.Command(apkPath, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			emitSignatureHint(stderrBuf.String())
			code := exitErr.ExitCode()
			if code < 0 {
				// The process was terminated by a signal; ExitCode() returns -1, which is
				// not a valid process exit status. Report a conventional failure code.
				code = 1
			}
			return code, nil
		}
		return 1, err
	}
	return 0, nil
}

func emitSignatureHint(stderr string) {
	sigPatterns := []string{
		"UNTRUSTED signature",
		"WARNING: Ignoring APKINDEX",
		"signature error",
	}
	for _, pattern := range sigPatterns {
		if strings.Contains(stderr, pattern) {
			log.Warn("Signature verification failed. Fix: jf setup apk")
			return
		}
	}
}

// warnIfApkTooOld emits a warning when the installed apk-tools version is older than 2.12,
// the first release with HTTP_AUTH support for authenticated repository access.
func warnIfApkTooOld() {
	out, err := exec.Command("apk", "--version").Output()
	if err != nil {
		return
	}
	// Output format: "apk-tools 2.12.14, compiled for x86_64"
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return
	}
	parts := strings.SplitN(fields[1], ".", 3)
	if len(parts) < 2 {
		return
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return
	}
	if major < 2 || (major == 2 && minor < 12) {
		log.Warn(fmt.Sprintf("apk version %s.%d detected — HTTP_AUTH injection may not be supported. Upgrade to apk-tools >= 2.12.", parts[0], minor))
	}
}

func (apkCmd *ApkCommand) collectBuildInfo(preSnapshot []biUtils.AlpinePackage, cacheDir, downloadsDir string, requestedPkgs []string) {
	buildName, nameErr := apkCmd.buildConfiguration.GetBuildName()
	if nameErr != nil || buildName == "" {
		log.Debug("Build name not set — skipping Build Info capture for apk")
		return
	}
	buildNumber, numErr := apkCmd.buildConfiguration.GetBuildNumber()
	if numErr != nil || buildNumber == "" {
		log.Debug("Build number not set — skipping Build Info capture for apk")
		return
	}

	buildObj, err := buildUtils.PrepareBuildPrerequisites(apkCmd.buildConfiguration)
	if err != nil {
		log.Warn("Build Info publish failed:", err)
		return
	}

	alpineVersion := apkCmd.alpineVersion
	if alpineVersion == "" {
		alpineVersion = detectSystemAlpineVersion()
	}
	moduleID := alpineModuleID(apkCmd.buildConfiguration.GetModule(), apkCmd.repoKey, detectSystemArch(), alpineVersion)
	effectivePreSnapshot := excludeRequestedPackages(preSnapshot, requestedPkgs)

	alpineModule := buildObj.AddAlpineModule(moduleID, apkCmd.repoKey, alpineVersion)
	alpineModule.SetPreSnapshot(effectivePreSnapshot)
	alpineModule.SetCacheDir(cacheDir)
	alpineModule.SetDownloadsDir(downloadsDir)
	alpineModule.SetRequestedPackages(requestedPkgs)

	deps, err := alpineModule.CollectDependencies()
	if err != nil {
		log.Warn("Build Info collection failed:", err)
		return
	}

	if len(deps) == 0 && len(requestedPkgs) > 0 {
		log.Warn(fmt.Sprintf(
			"Build Info completeness check: 0 dependencies recorded for packages %v. "+
				"The pre/post snapshot diff may be empty (all packages were already installed).",
			requestedPkgs,
		))
	} else {
		log.Debug(fmt.Sprintf("Build Info completeness check: %d dep(s) recorded.", len(deps)))
	}

	if apkCmd.serverDetails != nil && apkCmd.repoKey != "" {
		deps = apkCmd.enrichChecksumsFromAQL(deps)
	}

	if saveErr := alpineModule.SaveBuildInfo(deps); saveErr != nil {
		log.Warn("Build Info save failed:", saveErr)
	}
}

func (apkCmd *ApkCommand) enrichChecksumsFromAQL(deps []entities.Dependency) []entities.Dependency {
	var missing []int
	for i, dep := range deps {
		if dep.Sha256 == "" && dep.Sha1 == "" {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		return deps
	}

	sm, err := artutils.CreateServiceManager(apkCmd.serverDetails, -1, 0, false)
	if err != nil {
		log.Debug("Could not create Artifactory service manager for AQL checksum enrichment:", err)
		return deps
	}

	return enrichDepsChecksumsFromAQL(deps, missing, sm, apkCmd.repoKey)
}

func enrichDepsChecksumsFromAQL(deps []entities.Dependency, missing []int, sm artifactory.ArtifactoryServicesManager, repoKey string) []entities.Dependency {
	fileToDep := make(map[string]int, len(missing))
	names := make([]string, 0, len(missing))
	for _, i := range missing {
		fileName := apkFileNameFromID(deps[i].Id)
		fileToDep[fileName] = i
		names = append(names, fmt.Sprintf(`{"name":%s}`, aqlJSONString(fileName)))
	}
	aqlQuery := fmt.Sprintf(
		`{"$and":[{"$or":[{"repo":%s},{"repo":%s}]},{"$or":[%s]}]}`,
		aqlJSONString(repoKey),
		aqlJSONString(repoKey+"-cache"),
		strings.Join(names, ","),
	)
	log.Debug(fmt.Sprintf("AQL checksum enrichment: querying %d missing dep(s) in repo %s", len(missing), repoKey))

	reader, err := sm.SearchFiles(services.SearchParams{
		CommonParams: &specutils.CommonParams{Aql: specutils.Aql{ItemsFind: aqlQuery}},
	})
	if err != nil {
		log.Debug("AQL checksum enrichment failed:", err)
		return deps
	}
	defer func() { _ = reader.Close() }()

	resolved := 0
	for item := new(specutils.ResultItem); reader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
		i, ok := fileToDep[item.Name]
		if !ok {
			continue
		}
		deps[i].Sha1 = item.Actual_Sha1
		deps[i].Sha256 = item.Sha256
		deps[i].Md5 = item.Actual_Md5
		resolved++
	}
	if resolved > 0 {
		log.Info(fmt.Sprintf("AQL enrichment: resolved checksums for %d/%d missing dep(s)", resolved, len(missing)))
	}
	return deps
}

func aqlJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

func apkFileNameFromID(id string) string {
	return strings.Replace(id, ":", "-", 1) + ".apk"
}

func excludeRequestedPackages(snapshot []biUtils.AlpinePackage, requestedPkgs []string) []biUtils.AlpinePackage {
	requested := make(map[string]bool, len(requestedPkgs))
	for _, name := range requestedPkgs {
		if name != "" {
			requested[name] = true
		}
	}
	if len(requested) == 0 {
		return snapshot
	}
	filtered := snapshot[:0:0]
	for _, pkg := range snapshot {
		if !requested[pkg.Name] {
			filtered = append(filtered, pkg)
		}
	}
	return filtered
}

// stripJFFlags removes jf-specific flags (and their values) from the args slice so that
// only native apk flags are forwarded to the apk binary.
//
// Supported forms:
//   - "--flag value"  (two separate tokens)
//   - "--flag=value"  (single token with =)
func stripJFFlags(args []string) []string {
	result := make([]string, 0, len(args))
	skip := false
	for _, arg := range args {
		if skip {
			skip = false
			continue
		}
		// "--flag=value" form — strip the whole token.
		if idx := strings.Index(arg, "="); idx != -1 {
			if jfFlagSet[arg[:idx]] {
				continue
			}
		}
		// "--flag" form — mark next token to be skipped.
		if jfFlagSet[arg] {
			skip = true
			continue
		}
		result = append(result, arg)
	}
	return result
}

// containsFlag reports whether args contains the exact flag string.
func containsFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// flagValue returns the value of a --flag=value or --flag value pair, or "" if not found.
func flagValue(args []string, flag string) string {
	prefix := flag + "="
	for i, a := range args {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix)
		}
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// extractPackageNames returns the non-flag tokens from the args list,
// which represent the package names the user explicitly requested.
// apkValueFlags are native apk flags that consume the following token as their value.
// Their value tokens must not be mistaken for requested package names.
var apkValueFlags = map[string]bool{
	"--cache-dir": true, "--cache-max-age": true, "--keys-dir": true,
	"--repositories-file": true, "--arch": true, "--wait": true,
	"--repository": true, "-X": true, "--root": true, "-p": true,
	"--virtual": true, "-t": true,
}

func extractPackageNames(args []string) []string {
	var pkgs []string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// `--flag=value` carries its own value; `--flag value` consumes the next token.
			if !strings.Contains(arg, "=") && apkValueFlags[arg] {
				skipNext = true
			}
			continue
		}
		if arg != "" {
			pkgs = append(pkgs, arg)
		}
	}
	return pkgs
}

// filterSecretEnvVars removes environment variables whose names match common secret patterns
// (password, secret, token, key) from the env slice so they are not passed to the apk subprocess
// and are not captured in build-info environment records.
func filterSecretEnvVars(env []string) []string {
	excludePattern := os.Getenv(coreutils.EnvExclude)
	if excludePattern == "" {
		excludePattern = "*password*;*psw*;*secret*;*key*;*token*;*auth*"
	}
	excludePattern = strings.ReplaceAll(excludePattern, ",", ";")
	patterns := strings.Split(strings.ToLower(excludePattern), ";")

	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name := strings.ToLower(strings.SplitN(entry, "=", 2)[0])
		excluded := false
		for _, pat := range patterns {
			pat = strings.TrimSpace(pat)
			if pat == "" {
				continue
			}
			if matchGlob(pat, name) {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// matchGlob performs a simple glob match where '*' matches any substring.
func matchGlob(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx == -1 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	last := parts[len(parts)-1]
	return last == "" || strings.HasSuffix(s, last)
}

// createTempDir creates a temporary directory and returns its path.
func createTempDir() (string, error) {
	return os.MkdirTemp("", "apk-cache-*")
}
