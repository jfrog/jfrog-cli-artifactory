package alpine

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	biUtils "github.com/jfrog/build-info-go/build/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var buildInfoSubcmds = map[string]bool{
	"add":     true,
	"upgrade": true,
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
	apkPath, err := exec.LookPath("apk")
	if err != nil {
		return errorutils.CheckErrorf("'apk' binary not found. Is this an Alpine Linux environment?")
	}
	warnIfApkTooOld()

	needsAuth := true

	collectBuildInfo, err := apkCmd.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	needsBuildInfo := buildInfoSubcmds[apkCmd.commandName] && collectBuildInfo

	var preSnapshot []biUtils.AlpinePackage
	if needsBuildInfo {
		preSnapshot, err = biUtils.ListInstalledPackages()
		if err != nil {
			log.Warn("Cannot list installed packages — Build Info not captured:", err)
			needsBuildInfo = false
		}
	}

	env, err := apkCmd.buildEnvWithHTTPAuth(needsAuth)
	if err != nil {
		return err
	}

	var cacheDir string
	if needsBuildInfo {
		cacheDir, err = io.CreateTempDir()
		if err != nil {
			log.Warn("Could not create temp cache dir — checksums may be incomplete:", err)
		}
	}

	args := apkCmd.apkArgs
	if cacheDir != "" {
		args = append([]string{"--cache-dir", cacheDir}, args...)
	}
	fullArgs := append([]string{apkCmd.commandName}, args...)
	exitCode, err := runNativeApk(apkPath, fullArgs, env)
	if cacheDir != "" && (err != nil || exitCode != 0) {
		_ = os.RemoveAll(cacheDir)
	}
	if err != nil {
		return err
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	defer func() { _ = os.RemoveAll(cacheDir) }()

	if !needsBuildInfo {
		return nil
	}
	apkCmd.collectBuildInfo(preSnapshot, cacheDir)
	return nil
}

// buildEnvWithHTTPAuth returns the current process environment with HTTP_AUTH injected for the apk subprocess.
func (apkCmd *ApkCommand) buildEnvWithHTTPAuth(injectAuth bool) ([]string, error) {
	env := os.Environ()
	if !injectAuth {
		return env, nil
	}

	if apkCmd.serverDetails == nil {
		if apkCmd.username != "" || apkCmd.password != "" {
			log.Warn("--user/--password provided but no server URL is known. Use --server-id to select a configured server so HTTP_AUTH can be injected.")
		} else {
			log.Warn("No JFrog server configured — skipping HTTP_AUTH injection. Run: jf c add")
		}
		return env, nil
	}

	username, password := resolveHTTPAuthCredentials(apkCmd.serverDetails, apkCmd.username, apkCmd.password)
	httpAuth, err := buildHTTPAuth(apkCmd.serverDetails.GetArtifactoryUrl(), username, password)
	if err != nil {
		return nil, err
	}

	log.Debug("HTTP_AUTH=basic:<host>:<user>:***")
	env = append(env, "HTTP_AUTH="+httpAuth)
	return env, nil
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

// runNativeApk spawns the native apk binary, streaming stdout/stderr in real time.
func runNativeApk(apkPath string, args []string, env []string) (int, error) {
	cmd := exec.Command(apkPath, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			emitSignatureHint(exitErr)
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

// emitSignatureHint prints a remediation hint when apk exits with an RSA signature error.
func emitSignatureHint(exitErr *exec.ExitError) {
	stderr := string(exitErr.Stderr)
	sigPatterns := []string{
		"UNTRUSTED signature",
		"WARNING: Ignoring APKINDEX",
		"signature error",
	}
	for _, pattern := range sigPatterns {
		if strings.Contains(stderr, pattern) {
			log.Warn("Signature verification failed. Fix: jf apk config --server-id <id> --repo <repo> --alpine-version <vX.Y>")
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

// collectBuildInfo diffs the pre-snapshot against the current package list and saves the result as Build Info.
func (apkCmd *ApkCommand) collectBuildInfo(preSnapshot []biUtils.AlpinePackage, cacheDir string) {
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

	moduleID := apkCmd.buildConfiguration.GetModule()
	if moduleID == "" {
		moduleID = buildName + ":" + buildNumber + ":alpine"
	}
	// Remove explicitly requested packages from the pre-snapshot so they always
	// appear in the diff — even if they were already installed before this run.
	// This matches the contract of every other package manager: `jf apk add zlib`
	// must record zlib as a dependency regardless of prior installation state.
	effectivePreSnapshot := excludeRequestedPackages(preSnapshot, apkCmd.apkArgs)

	alpineModule := buildObj.AddAlpineModule(moduleID, apkCmd.repoKey, apkCmd.alpineVersion)
	alpineModule.SetPreSnapshot(effectivePreSnapshot)
	alpineModule.SetCacheDir(cacheDir)

	if err := alpineModule.CollectBuildInfo(); err != nil {
		log.Warn("Build Info collection failed:", err)
	}
}

// excludeRequestedPackages removes packages whose name matches any explicitly
// requested package in args (non-flag tokens) from the snapshot.
// This ensures the post-run diff always captures explicitly requested packages.
func excludeRequestedPackages(snapshot []biUtils.AlpinePackage, args []string) []biUtils.AlpinePackage {
	requested := make(map[string]bool)
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") && arg != "" {
			requested[arg] = true
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
