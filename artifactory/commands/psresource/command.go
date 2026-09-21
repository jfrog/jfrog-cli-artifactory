// Package psresource implements the FlexPack-native command for PowerShell PSResourceGet
// (Install-PSResource, Save-PSResource, Update-PSResource, Publish-PSResource, and the rest of the
// PSResourceGet cmdlet surface passed through unchanged). It mirrors the shape of
// artifactory/commands/choco's command, adapted for native PowerShell cmdlets rather than a
// single-purpose CLI: the subcommand is the cmdlet's own native name (e.g. "Install-PSResource"),
// and args are that cmdlet's own native parameters, forwarded through mostly as-is.
package psresource

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jfrog/build-info-go/entities"
	buildinfoflex "github.com/jfrog/build-info-go/flexpack"
	psresourceflex "github.com/jfrog/build-info-go/flexpack/psresource"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/setup"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	rtutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildutils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/http/httpclient"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/io/httputils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	// SubCommand names. These are the native PSResourceGet cmdlet names, used verbatim
	// (case-sensitive, as PowerShell itself writes them) - unlike choco's lowercase "install"/"push",
	// this project's naming decision for PSResource is to key off the native cmdlet name directly.
	SubCommandInstall                    = "Install-PSResource"
	SubCommandSave                       = "Save-PSResource"
	SubCommandUpdate                     = "Update-PSResource"
	SubCommandPublish                    = "Publish-PSResource"
	psresourceHeadRetries                = 3
	psresourceHeadRetryIntervalMilliSecs = 1000
)

// errPSResourcePackageNotFound reports that the package this build-info is about was not found in
// Artifactory at the path derived from its own name/version/repository. Persisting build-info built
// from that same wrong path would only bake the bad reference into a later promotion, so this must
// stop the command instead of being logged and ignored.
var errPSResourcePackageNotFound = errors.New("the PSResource package was not found in Artifactory at the expected path, so no build-info was saved")

// validatePSResourcePlatform and resolvePSResourceShell indirect through the setup package's
// platform detection (setup.ValidatePSResourcePlatform / setup.PSResourceShell). They are vars,
// wrapping those functions, purely so tests in this package can stub the platform/shell decision
// without needing access to setup's own unexported test doubles - the underlying detection logic
// still lives in exactly one place (the setup package), never reimplemented here.
var validatePSResourcePlatform = setup.ValidatePSResourcePlatform
var resolvePSResourceShell = setup.PSResourceShell

// psresourceNativeRunner shells out to the resolved PowerShell executable to run the actual
// PSResourceGet cmdlet, streaming stdio to the user like any other passthrough wrapper. It is a
// var, like chocoNativeRunner, so tests can replace it with a stub instead of invoking pwsh.
//
// envAdditions are appended to a *copy* of the current process environment for this child process
// only - see Run() for why credentials travel this way instead of as a literal command-line
// argument or a call to os.Setenv on the parent process.
var psresourceNativeRunner = func(shell, script string, envAdditions []string, workingDirectory string) error {
	cmd := exec.Command(shell, "-NoProfile", "-Command", script) // #nosec G204 -- shell is resolved by setup.PSResourceShell (pwsh or powershell.exe only); script is this same command's own constructed PowerShell invocation, built from this command's own arguments
	cmd.Dir = workingDirectory
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	var stderr strings.Builder
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	if len(envAdditions) > 0 {
		cmd.Env = append(append([]string(nil), os.Environ()...), envAdditions...)
	}
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

// psresourceQueryRunner shells out to the resolved PowerShell executable and captures its stdout,
// for the small number of read-only queries this package needs to make of PSResourceGet itself
// (resolving a manifest's ModuleVersion, resolving an installed package's version). It is a
// separate var from psresourceNativeRunner because these calls must capture output rather than
// stream it to the user, and never accept stdin.
var psresourceQueryRunner = func(shell, script string) (string, error) {
	cmd := exec.Command(shell, "-NoProfile", "-Command", script) // #nosec G204 -- shell is resolved by setup.PSResourceShell (pwsh or powershell.exe only); script is this package's own constructed query
	output, err := cmd.Output()
	return string(output), err
}

// PSResourceFlexPackCommand runs one native PSResourceGet cmdlet and, when build-name/build-number
// are supplied, collects build-info for it via the FlexPack-native build-info-go collector.
type PSResourceFlexPackCommand struct {
	// subCommand is the native PSResourceGet cmdlet name, e.g. "Install-PSResource".
	subCommand string
	// args are that cmdlet's own native parameters, forwarded through mostly as-is.
	args               []string
	serverDetails      *config.ServerDetails
	repoResolve        string
	repoDeploy         string
	buildConfiguration *buildutils.BuildConfiguration
	workingDirectory   string
}

func NewPSResourceFlexPackCommand() *PSResourceFlexPackCommand {
	return &PSResourceFlexPackCommand{}
}

func (command *PSResourceFlexPackCommand) SetSubCommand(value string) *PSResourceFlexPackCommand {
	command.subCommand = value
	return command
}

func (command *PSResourceFlexPackCommand) SetArgs(value []string) *PSResourceFlexPackCommand {
	command.args = append([]string(nil), value...)
	return command
}

func (command *PSResourceFlexPackCommand) SetServerDetails(value *config.ServerDetails) *PSResourceFlexPackCommand {
	command.serverDetails = value
	return command
}

func (command *PSResourceFlexPackCommand) SetRepoResolve(value string) *PSResourceFlexPackCommand {
	command.repoResolve = value
	return command
}

func (command *PSResourceFlexPackCommand) SetRepoDeploy(value string) *PSResourceFlexPackCommand {
	command.repoDeploy = value
	return command
}

func (command *PSResourceFlexPackCommand) SetBuildConfiguration(value *buildutils.BuildConfiguration) *PSResourceFlexPackCommand {
	command.buildConfiguration = value
	return command
}

func (command *PSResourceFlexPackCommand) SetWorkingDirectory(value string) *PSResourceFlexPackCommand {
	command.workingDirectory = value
	return command
}

func (command *PSResourceFlexPackCommand) CommandName() string {
	return "rt_psresource_flexpack"
}

func (command *PSResourceFlexPackCommand) ServerDetails() (*config.ServerDetails, error) {
	return command.serverDetails, nil
}

func (command *PSResourceFlexPackCommand) Run() error {
	if err := validatePSResourcePlatform(); err != nil {
		return err
	}
	// --build-name and --build-number are only meaningful as a pair; reject a half-specified pair
	// before the native command runs, exactly like choco does, so a flag mistake costs nothing.
	if command.buildConfiguration != nil {
		if err := command.buildConfiguration.ValidateBuildParams(); err != nil {
			return err
		}
	}
	if command.workingDirectory == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		command.workingDirectory = workingDirectory
	}

	shell, err := resolvePSResourceShell()
	if err != nil {
		return err
	}

	injectRepo := command.credentialInjectionRepo()
	credentialEnv, injectCredential, err := command.resolveCredentialEnv(injectRepo)
	if err != nil {
		return err
	}

	script := command.buildScript(injectCredential)
	log.Debug(fmt.Sprintf("Running native PSResourceGet command: %s -NoProfile -Command \"%s\"",
		shell, command.debugScript()))
	if err := psresourceNativeRunner(shell, script, credentialEnv, command.workingDirectory); err != nil {
		return fmt.Errorf("%s failed: %w", command.subCommand, err)
	}

	collectBuildInfo, err := command.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if !collectBuildInfo {
		return nil
	}
	buildName, err := command.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := command.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}

	switch {
	case isPublishCmdlet(command.subCommand):
		return command.collectPublishArtifacts(buildName, buildNumber, shell)
	case isDependencyCmdlet(command.subCommand):
		return command.collectDependencies(buildName, buildNumber, shell)
	default:
		// Find-PSResource, Uninstall-PSResource, *-PSResourceRepository, etc: no build-info to
		// collect for these, so a plain passthrough is the correct (and only) outcome.
		return nil
	}
}

// isPublishCmdlet reports whether subCommand is Publish-PSResource, the one cmdlet whose build-info
// is collected from an artifact this command itself just uploaded.
func isPublishCmdlet(subCommand string) bool {
	return subCommand == SubCommandPublish
}

// isDependencyCmdlet reports whether subCommand is one of Install-/Save-/Update-PSResource, the
// cmdlets whose build-info is collected from packages PSResourceGet resolved as dependencies.
func isDependencyCmdlet(subCommand string) bool {
	switch subCommand {
	case SubCommandInstall, SubCommandSave, SubCommandUpdate:
		return true
	default:
		return false
	}
}

// credentialInjectionRepo returns the repository whose credentials should be injected for the
// current subcommand: the deploy repo for Publish-PSResource, the resolve repo for
// Install-/Save-/Update-PSResource, or "" for every other cmdlet (Find-PSResource,
// *-PSResourceRepository, ...), which never touch a configured Artifactory repo on this command's
// behalf.
func (command *PSResourceFlexPackCommand) credentialInjectionRepo() string {
	switch {
	case isPublishCmdlet(command.subCommand):
		return command.repoDeploy
	case isDependencyCmdlet(command.subCommand):
		return command.repoResolve
	default:
		return ""
	}
}

// resolveCredentialEnv decides whether this invocation should inject -Credential, and if so
// prepares the environment variables that carry the secret to the child pwsh process.
//
// Security note: the resolved username/password are placed ONLY in the returned env-var slice,
// never in a command-line argument and never in any log line. See buildScript/debugScript: the
// constructed PowerShell script references $env:JFROG_PSRESOURCE_USER / $env:JFROG_PSRESOURCE_TOKEN
// by name, not by value, so logging the script text is always safe.
func (command *PSResourceFlexPackCommand) resolveCredentialEnv(repo string) (env []string, inject bool, err error) {
	if repo == "" || command.serverDetails == nil || hasNativeCredentialOverride(command.args) {
		return nil, false, nil
	}
	_, user, password, err := dotnet.GetSourceDetails(command.serverDetails, repo, false)
	if err != nil {
		return nil, false, fmt.Errorf("get PSResource credential details: %w", err)
	}
	if user == "" || password == "" {
		// Anonymous resolution/publish is a legitimate choice when the repository allows it, so
		// this is only a note, not an error - a later 401 would otherwise look unexplained.
		log.Debug(fmt.Sprintf("No JFrog credentials configured for repository %q; running %s without an injected -Credential.", repo, command.subCommand))
		return nil, false, nil
	}
	return []string{
		"JFROG_PSRESOURCE_USER=" + user,
		"JFROG_PSRESOURCE_TOKEN=" + password,
	}, true, nil
}

// buildScript assembles the -Command script text passed to pwsh/powershell.exe: an optional
// credential block (reading the secret back out of the environment variables resolveCredentialEnv
// populated on this child process only), the native cmdlet invocation with this command's own
// arguments, and -Credential $cred appended when injectCredential is true.
func (command *PSResourceFlexPackCommand) buildScript(injectCredential bool) string {
	var statements []string
	if injectCredential {
		statements = append(statements,
			"$securePw = ConvertTo-SecureString $env:JFROG_PSRESOURCE_TOKEN -AsPlainText -Force",
			"$cred = New-Object System.Management.Automation.PSCredential($env:JFROG_PSRESOURCE_USER, $securePw)")
	}
	statements = append(statements, command.cmdletInvocation(injectCredential, false))
	return strings.Join(statements, "; ")
}

// debugScript is what buildScript's cmdlet invocation looks like with any user-supplied secret
// (-ApiKey/-Password/-Credential/-Token literal values, if the user passed their own rather than
// letting this command inject one) replaced with "***". The credential block itself is never
// included here or in buildScript's logged form, because it only ever names environment variables,
// never a resolved secret value.
func (command *PSResourceFlexPackCommand) debugScript() string {
	return command.cmdletInvocation(false, true)
}

// cmdletInvocation builds the native cmdlet invocation. redactForLogging must be false for the
// script actually handed to pwsh (buildScript) - redaction is a logging-only concern, and applying
// it to the real script would silently replace the user's own credential value with the literal
// string "***", breaking authentication. It must be true for anything rendered into a log line
// (debugScript).
func (command *PSResourceFlexPackCommand) cmdletInvocation(injectCredential, redactForLogging bool) string {
	args := command.args
	if redactForLogging {
		args = redactPSResourceArgs(args)
	}
	parts := make([]string, 0, len(args)+2)
	parts = append(parts, command.subCommand)
	for _, arg := range args {
		parts = append(parts, psresourceCommandLineArg(arg))
	}
	if injectCredential {
		parts = append(parts, "-Credential", "$cred")
	}
	return strings.Join(parts, " ")
}

// psresourceFlagPattern matches a plausible bare flag token: "-Name" or "-Name:value", where the
// flag name itself is a simple identifier and, if a colon-attached value follows, that value
// contains none of the characters that could end the -Command script's current statement early
// (semicolon, pipe, ampersand, backtick, a "$(" subexpression opener, or a line break). Anything
// that doesn't match this is treated as a value and safely quoted instead of being spliced in raw -
// closing off the case where a single crafted argument (e.g. threaded through from an
// external/templated source) happens to start with "-" but is not actually just a flag name.
var psresourceFlagPattern = regexp.MustCompile(`^-[A-Za-z][A-Za-z0-9]*(:[^;&|` + "`" + `\r\n]*)?$`)

// psresourceCommandLineArg renders one native argument for splicing into the -Command script.
// A token matching psresourceFlagPattern is a parameter name (optionally with a colon-attached
// value) and is passed through unquoted; everything else is a value and is quoted as a
// single-quoted PowerShell string literal (doubling any embedded single quote), which is always a
// safe way to hand PowerShell a literal string regardless of the parameter's declared type.
func psresourceCommandLineArg(arg string) string {
	if psresourceFlagPattern.MatchString(arg) {
		return arg
	}
	return setup.QuotePSLiteral(arg)
}

// hasNativeCredentialOverride reports whether the user already supplied their own -Credential or
// -ApiKey in the native arguments. When true, this command injects nothing: PSResourceGet only
// accepts one -Credential (or one -ApiKey for Publish-PSResource), so a second one from this
// command would either conflict or silently lose to the user's own.
func hasNativeCredentialOverride(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		for _, flag := range []string{"-credential", "-apikey"} {
			if lower == flag || strings.HasPrefix(lower, flag+":") {
				return true
			}
		}
	}
	return false
}

// redactPSResourceArgs replaces the value of any credential-bearing native flag with "***", in
// both the space-separated form (-ApiKey secret) and PowerShell's colon-attached form
// (-ApiKey:secret, a single token carrying both the flag and its value). This only ever matters
// when the user supplied their own -ApiKey/-Password/-Credential/-Token literal on the command
// line (hasNativeCredentialOverride's case) - this command's own injected credential never appears
// as literal text in the first place, only as an $env: reference.
func redactPSResourceArgs(args []string) []string {
	redacted := append([]string(nil), args...)
	sensitiveFlags := map[string]bool{"-apikey": true, "-password": true, "-credential": true, "-token": true}
	for index := range redacted {
		lower := strings.ToLower(redacted[index])
		if colonIdx := strings.Index(lower, ":"); colonIdx > 0 && sensitiveFlags[lower[:colonIdx]] {
			redacted[index] = redacted[index][:colonIdx] + ":***"
			continue
		}
		if index > 0 && sensitiveFlags[strings.ToLower(redacted[index-1])] {
			redacted[index] = "***"
		}
	}
	return redacted
}

// argValue returns the first value bound to flag (e.g. "-Repository") in args, checking both the
// space-separated form (-Repository myrepo) and the colon form (-Repository:myrepo). Returns "" if
// the flag is absent.
func argValue(args []string, flag string) string {
	values := argValues(args, flag)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// argValues returns every value bound to flag in args, splitting a comma-separated token into
// multiple values - PowerShell's own convention for binding an array parameter from the command
// line (e.g. "-Name Foo,Bar,Baz").
func argValues(args []string, flag string) []string {
	lowerFlag := strings.ToLower(flag)
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if lower == lowerFlag && index+1 < len(args) {
			return splitCommaList(args[index+1])
		}
		if strings.HasPrefix(lower, lowerFlag+":") {
			return splitCommaList(arg[len(flag)+1:])
		}
	}
	return nil
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// positionalValues returns the leading run of bare (non-flag) tokens in args, in order, stopping at
// the first token that starts with "-". This mirrors PowerShell's own positional binding: PSResourceGet
// binds an unlabelled leading argument to each cmdlet's first positional parameter - Publish-PSResource's
// is -Path, Install-/Save-/Update-PSResource's is -Name (which also accepts several bare tokens, e.g.
// "Install-PSResource Foo Bar -Repository myrepo" binds Name=@("Foo","Bar")). Without this, a command
// invoked positionally (valid, common PSResourceGet usage) is silently treated as if neither flag was
// given at all.
func positionalValues(args []string) []string {
	var values []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			break
		}
		values = append(values, arg)
	}
	return values
}

// firstPositionalValue returns the first leading bare token in args, or "" if the command was
// invoked with no positional argument (e.g. everything given via explicit flags).
func firstPositionalValue(args []string) string {
	values := positionalValues(args)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// collectPublishArtifacts collects build-info for a completed Publish-PSResource. PSResourceGet
// uploads directly from the module manifest without leaving a local .nupkg for build-info-go to
// hash, so the package's identity (name/version) and checksum both have to be resolved after the
// fact: identity from -Name/-Version if the caller passed them, otherwise from the module manifest
// at -Path; checksum from a HEAD request to the path the package must now exist at in Artifactory.
func (command *PSResourceFlexPackCommand) collectPublishArtifacts(buildName, buildNumber, shell string) error {
	repo := argValue(command.args, "-Repository")
	if repo == "" {
		repo = command.repoDeploy
	}
	if repo == "" {
		return fmt.Errorf("cannot determine the target repository for %s build-info: pass -Repository or --repo-deploy", SubCommandPublish)
	}
	name, version, err := command.resolvePublishNameVersion(shell)
	if err != nil {
		return err
	}
	path := psresourceflex.DerivePublishedPath(name, version)
	httpCtx, err := newPSResourceHTTPContext(command.serverDetails)
	if err != nil {
		return err
	}
	checksum, err := fetchArtifactChecksum(httpCtx, repo, path)
	if err != nil {
		return wrapChecksumErr(err, fmt.Sprintf("the published PSResource package at %s/%s", repo, path), repo, path)
	}
	artifact, err := psresourceflex.BuildPublishedArtifact(psresourceflex.PublishedArtifact{
		ResolvedPackage: psresourceflex.ResolvedPackage{Name: name, Version: version, Checksum: checksum},
		Repo:            repo,
	})
	if err != nil {
		return err
	}
	artifacts := []entities.Artifact{artifact}
	stampErr := stampBuildPropertiesFn(command, artifacts, buildName, buildNumber)
	return command.persistArtifactBuildInfo(stampErr, buildName, buildNumber, artifacts)
}

// resolvePublishNameVersion determines the published package's name/version. -Name/-Version on the
// command line win outright. Otherwise it locates the module manifest (.psd1) at -Path (or, absent
// -Path, in the working directory) and asks pwsh itself for ModuleVersion via
// Import-PowerShellDataFile - shelling back out to PowerShell to parse its own data-file format is
// more robust than reimplementing a .psd1 parser in Go, and it is exactly the value
// Publish-PSResource itself would have read.
func (command *PSResourceFlexPackCommand) resolvePublishNameVersion(shell string) (name, version string, err error) {
	name = argValue(command.args, "-Name")
	version = argValue(command.args, "-Version")
	if name != "" && version != "" {
		return name, version, nil
	}

	path := argValue(command.args, "-Path")
	if path == "" {
		// -Path is Publish-PSResource's first positional parameter - a bare leading token
		// ("Publish-PSResource ./MyModule ...") is valid, common usage, not an omission.
		path = firstPositionalValue(command.args)
	}
	if path == "" {
		path = command.workingDirectory
	}
	if !strings.HasPrefix(path, "/") && !strings.Contains(path, ":") {
		path = joinWorkingDirectory(command.workingDirectory, path)
	}
	manifestPath, findErr := resolvePSD1Manifest(path)
	if findErr != nil {
		return "", "", findErr
	}
	if name == "" {
		name = manifestModuleName(manifestPath)
	}
	if version == "" {
		version, err = psd1ModuleVersion(shell, manifestPath)
		if err != nil {
			return "", "", err
		}
	}
	if name == "" || version == "" {
		return "", "", fmt.Errorf("could not determine the published PSResource package name/version; pass -Name and -Version explicitly, or point -Path at a module manifest (.psd1) with a ModuleVersion")
	}
	return name, version, nil
}

func joinWorkingDirectory(workingDirectory, path string) string {
	if workingDirectory == "" || path == "" {
		return path
	}
	return strings.TrimSuffix(workingDirectory, "/") + "/" + path
}

// manifestModuleName derives a module's name from its manifest file name, following PowerShell's
// own convention that a module named "Foo" has a manifest named "Foo.psd1".
func manifestModuleName(manifestPath string) string {
	base := manifestPath
	if idx := strings.LastIndexAny(base, "/\\"); idx != -1 {
		base = base[idx+1:]
	}
	return strings.TrimSuffix(base, filepathExt(base))
}

func filepathExt(name string) string {
	if idx := strings.LastIndex(name, "."); idx != -1 {
		return name[idx:]
	}
	return ""
}

// resolvePSD1Manifest returns the module manifest (.psd1) Publish-PSResource will read for path: path
// itself if it already names a .psd1 file, or the first .psd1 found directly inside it if it is a
// directory.
func resolvePSD1Manifest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s -Path %q: %w", SubCommandPublish, path, err)
	}
	if !info.IsDir() {
		if strings.EqualFold(filepathExt(path), ".psd1") {
			return path, nil
		}
		return "", fmt.Errorf("%s -Path %q is not a module manifest (.psd1)", SubCommandPublish, path)
	}
	entries, err := os.ReadDir(path) // #nosec G304 -- path is this same command's own -Path/working-directory argument, not externally supplied
	if err != nil {
		return "", fmt.Errorf("read %s -Path %q: %w", SubCommandPublish, path, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepathExt(entry.Name()), ".psd1") {
			return joinWorkingDirectory(path, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("no module manifest (.psd1) found in %q", path)
}

// psd1ModuleVersion asks pwsh/powershell.exe for a manifest's ModuleVersion via
// Import-PowerShellDataFile, rather than parsing the .psd1 data-file format in Go.
func psd1ModuleVersion(shell, manifestPath string) (string, error) {
	script := fmt.Sprintf("(Import-PowerShellDataFile -Path %s).ModuleVersion", setup.QuotePSLiteral(manifestPath))
	output, err := psresourceQueryRunner(shell, script)
	if err != nil {
		return "", fmt.Errorf("resolve ModuleVersion from %q: %w", manifestPath, err)
	}
	version := strings.TrimSpace(output)
	if version == "" {
		return "", fmt.Errorf("manifest %q has no ModuleVersion", manifestPath)
	}
	return version, nil
}

// installedPSResource is the subset of Get-InstalledPSResource's JSON output this package reads.
type installedPSResource struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
}

// resolveInstalledVersion asks PSResourceGet which version of name is now installed, via
// Get-InstalledPSResource | ConvertTo-Json, rather than trusting the version (if any) the caller
// asked for on the command line - Install-/Save-/Update-PSResource can legitimately resolve a
// different version (e.g. a floating -Version range, or an upgrade), so the build-info must record
// what was actually installed.
func resolveInstalledVersion(shell, name string) (string, error) {
	script := fmt.Sprintf("Get-InstalledPSResource -Name %s | ConvertTo-Json", setup.QuotePSLiteral(name))
	output, err := psresourceQueryRunner(shell, script)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", fmt.Errorf("Get-InstalledPSResource -Name %q returned no result", name)
	}
	// ConvertTo-Json emits a bare object for exactly one result and a JSON array for more than one
	// (e.g. several versions installed side-by-side), so the array shape is tried first.
	var list []installedPSResource
	if jsonErr := json.Unmarshal([]byte(trimmed), &list); jsonErr == nil && len(list) > 0 {
		return latestInstalledVersion(list), nil
	}
	var single installedPSResource
	if jsonErr := json.Unmarshal([]byte(trimmed), &single); jsonErr != nil {
		return "", fmt.Errorf("parse Get-InstalledPSResource output for %q: %w", name, jsonErr)
	}
	if single.Version == "" {
		return "", fmt.Errorf("Get-InstalledPSResource -Name %q returned no version", name)
	}
	return single.Version, nil
}

// latestInstalledVersion picks the highest version by plain string comparison when several are
// installed side-by-side. This is a best-effort v1 choice, not a SemVer-correct comparison (it
// would rank "1.9.0" above "1.10.0"); PSResourceGet version strings are usually well-behaved
// enough in practice, and getting this wrong only affects which of several already-installed
// versions is recorded, never which one is used.
func latestInstalledVersion(list []installedPSResource) string {
	latest := list[0]
	for _, candidate := range list[1:] {
		if candidate.Version > latest.Version {
			latest = candidate
		}
	}
	return latest.Version
}

// collectDependencies collects build-info for a completed Install-/Save-/Update-PSResource: for
// every package named on the command line, it asks PSResourceGet what version actually ended up
// installed, then resolves that package's checksum from Artifactory the same way
// collectPublishArtifacts does.
func (command *PSResourceFlexPackCommand) collectDependencies(buildName, buildNumber, shell string) error {
	names := argValues(command.args, "-Name")
	if len(names) == 0 {
		// -Name is Install-/Save-/Update-PSResource's first positional parameter - bare leading
		// tokens ("Install-PSResource Foo Bar ...") are valid, common usage, not an omission.
		names = positionalValues(command.args)
	}
	if len(names) == 0 {
		return fmt.Errorf("cannot determine which PSResource packages to collect build-info for: no -Name argument found on the %s command", command.subCommand)
	}
	if command.repoResolve == "" {
		return fmt.Errorf("cannot determine the source repository for %s build-info: pass --repo-resolve", command.subCommand)
	}
	httpCtx, err := newPSResourceHTTPContext(command.serverDetails)
	if err != nil {
		return err
	}

	resolved := make([]psresourceflex.ResolvedPackage, 0, len(names))
	for _, name := range names {
		version, versionErr := resolveInstalledVersion(shell, name)
		if versionErr != nil {
			return fmt.Errorf("resolve installed version of %q: %w", name, versionErr)
		}
		path := psresourceflex.DerivePublishedPath(name, version)
		checksum, checksumErr := fetchArtifactChecksum(httpCtx, command.repoResolve, path)
		if checksumErr != nil {
			return wrapChecksumErr(checksumErr, fmt.Sprintf("%s %s", name, version), command.repoResolve, path)
		}
		resolved = append(resolved, psresourceflex.ResolvedPackage{Name: name, Version: version, Checksum: checksum})
	}

	collector, err := psresourceflex.NewPSResourceFlexPack(buildinfoflex.PSResourceConfig{
		WorkingDirectory: command.workingDirectory,
		Module:           command.moduleID(),
		ResolvedPackages: resolved,
	})
	if err != nil {
		return err
	}
	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("collect PSResource dependencies: %w", err)
	}
	return saveBuildInfoLocally(buildInfo, command.projectKey())
}

// wrapChecksumErr wraps a checksum-resolution failure with context for the caller: a
// errPSResourcePackageNotFound is annotated with the repo/path the package was expected at (chained
// with %w so errors.Is still matches), and any other failure is wrapped with a context-specific
// "resolve checksum for ..." message.
func wrapChecksumErr(err error, context, repo, path string) error {
	if errors.Is(err, errPSResourcePackageNotFound) {
		return fmt.Errorf("%w: expected it at %s/%s", err, repo, path)
	}
	return fmt.Errorf("resolve checksum for %s: %w", context, err)
}

// psresourceHTTPContext bundles the Artifactory auth config and HTTP client fetchArtifactChecksum
// needs. It is built once per command invocation (collectPublishArtifacts/collectDependencies) and
// passed down, instead of fetchArtifactChecksum rebuilding both on every call - which mattered most
// for collectDependencies's per-package loop, where that used to mean a separate client build and
// auth-config derivation for every package.
type psresourceHTTPContext struct {
	client            *httpclient.HttpClient
	artDetails        auth.ServiceDetails
	httpClientDetails httputils.HttpClientDetails
}

func newPSResourceHTTPContext(serverDetails *config.ServerDetails) (psresourceHTTPContext, error) {
	artDetails, err := serverDetails.CreateArtAuthConfig()
	if err != nil {
		return psresourceHTTPContext{}, fmt.Errorf("create Artifactory auth config: %w", err)
	}
	client, err := httpclient.ClientBuilder().Build()
	if err != nil {
		return psresourceHTTPContext{}, fmt.Errorf("build HTTP client: %w", err)
	}
	return psresourceHTTPContext{
		client:            client,
		artDetails:        artDetails,
		httpClientDetails: artDetails.CreateHttpClientDetails(),
	}, nil
}

// fetchArtifactChecksum HEADs (never GETs - a GET would increment Artifactory's download counters
// for a check that only confirms existence and reads checksums) the given repo/path and returns the
// checksum headers Artifactory attaches to the response. Transient failures are retried; a 404
// fails immediately with errPSResourcePackageNotFound, since retrying it would only mask a
// genuinely wrong repo/path for longer.
func fetchArtifactChecksum(httpCtx psresourceHTTPContext, repo, path string) (entities.Checksum, error) {
	artifactURL, err := clientutils.BuildUrl(httpCtx.artDetails.GetUrl(), repo+"/"+path, map[string]string{})
	if err != nil {
		return entities.Checksum{}, fmt.Errorf("build artifact URL: %w", err)
	}

	var checksum entities.Checksum
	err = executeWithPSResourceRetry("Failed to HEAD the PSResource package in Artifactory", func() (bool, error) {
		resp, body, headErr := httpCtx.client.SendHead(artifactURL, httpCtx.httpClientDetails, "")
		// httpCtx.client has its own internal retry wrapper (default zero retries) that treats any
		// 5xx/429 status as "should retry"; once ITS budget is exhausted it returns a non-nil,
		// opaque timeout error even though the real HTTP response is still available. Inspecting
		// resp first (whenever it's non-nil) lets our own retry loop see the real status - and
		// therefore actually retry a transient 5xx - instead of always giving up after one attempt
		// on an error isRetryableError has no way to recognize. headErr is only the right signal
		// when resp itself is nil (a genuine connection-level failure).
		if resp == nil {
			return isRetryableError(headErr), headErr
		}
		if resp.StatusCode == http.StatusNotFound {
			return false, errPSResourcePackageNotFound
		}
		if statusErr := errorutils.CheckResponseStatusWithBody(resp, body, http.StatusOK); statusErr != nil {
			return isRetryableError(statusErr), statusErr
		}
		checksum = entities.Checksum{
			Sha256: resp.Header.Get("X-Checksum-Sha256"),
			Sha1:   resp.Header.Get("X-Checksum-Sha1"),
			Md5:    resp.Header.Get("X-Checksum-Md5"),
		}
		return false, nil
	})
	if err != nil {
		return entities.Checksum{}, err
	}
	return checksum, nil
}

// executeWithPSResourceRetry runs handler through the retry policy shared by every retried operation
// in this package (fixed retry count/interval and log prefix), retrying only when handler itself
// reports the failure as retryable.
func executeWithPSResourceRetry(errorMessage string, handler func() (bool, error)) error {
	executor := clientutils.RetryExecutor{
		MaxRetries:               psresourceHeadRetries,
		RetriesIntervalMilliSecs: psresourceHeadRetryIntervalMilliSecs,
		ErrorMessage:             errorMessage,
		LogMsgPrefix:             "[PSResource build-info] ",
		ExecutionHandler:         handler,
	}
	return executor.Execute()
}

// retryOnPSResourceError is executeWithPSResourceRetry for the common case where retryability is
// decided solely by isRetryableError on fn's own returned error - the shape searchOnce and
// setPropsWithRetry both need.
func retryOnPSResourceError(errorMessage string, fn func() error) error {
	return executeWithPSResourceRetry(errorMessage, func() (bool, error) {
		err := fn()
		return err != nil && isRetryableError(err), err
	})
}

// isRetryableError reports whether err is worth another attempt: a server-side or throttling HTTP
// status, or a transport-level failure.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *errorutils.HttpResponseError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= http.StatusInternalServerError || httpErr.StatusCode == http.StatusTooManyRequests
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF)
}

// persistArtifactBuildInfo decides whether the collected build-info survives a stamping failure. A
// "not found" stamping failure invalidates the build-info itself (the search pattern is built from
// the very same repo/path just resolved above), so it is returned as-is instead of being persisted;
// every other stamping outcome still has a build-info worth saving.
func (command *PSResourceFlexPackCommand) persistArtifactBuildInfo(stampErr error, buildName, buildNumber string, artifacts []entities.Artifact) error {
	if errors.Is(stampErr, errPSResourcePackageNotFound) {
		return stampErr
	}
	buildInfo := &entities.BuildInfo{
		Name:    buildName,
		Number:  buildNumber,
		Modules: []entities.Module{{Id: command.moduleID(), Type: entities.Nuget, Artifacts: artifacts}},
	}
	if saveErr := saveBuildInfoLocally(buildInfo, command.projectKey()); saveErr != nil {
		if stampErr != nil {
			return errors.Join(stampErr, saveErr)
		}
		return saveErr
	}
	if stampErr != nil {
		log.Info("PSResource build info was saved locally, but the build properties were not stamped on the artifact.")
	}
	return stampErr
}

func (command *PSResourceFlexPackCommand) moduleID() string {
	if command.buildConfiguration != nil {
		if module := command.buildConfiguration.GetModule(); module != "" {
			return module
		}
	}
	return "psresource-project"
}

// projectKey is a nil-safe accessor for command.buildConfiguration.GetProject(): unlike
// IsCollectBuildInfo, BuildConfiguration.GetProject/GetModule dereference their receiver directly
// and panic when it is nil, and command.buildConfiguration is nil for every call that never set a
// build configuration (a legitimate, common case - see Run()'s own nil check before calling
// ValidateBuildParams).
func (command *PSResourceFlexPackCommand) projectKey() string {
	if command.buildConfiguration == nil {
		return ""
	}
	return command.buildConfiguration.GetProject()
}

// stampBuildPropertiesFn indirects the real property-stamping implementation so tests can replace
// it with a no-op: stamping drives a real Artifactory search-and-SetProps round trip that a plain
// httptest.NewServer handler cannot fake convincingly, and the checksum-fetch logic this package
// actually owns is exercised separately, directly against fetchArtifactChecksum.
var stampBuildPropertiesFn = func(command *PSResourceFlexPackCommand, artifacts []entities.Artifact, buildName, buildNumber string) error {
	return command.stampBuildProperties(artifacts, buildName, buildNumber)
}

// stampBuildProperties sets build.name/build.number/build.timestamp on the artifact(s) that were
// just published, exactly as choco's stampBuildProperties does: search Artifactory for the recorded
// repo/path to confirm it is really there, then SetProps on the result.
func (command *PSResourceFlexPackCommand) stampBuildProperties(artifacts []entities.Artifact, buildName, buildNumber string) error {
	if command.serverDetails == nil || len(artifacts) == 0 {
		return nil
	}
	patterns := artifactPatterns(artifacts)
	if len(patterns) == 0 {
		return nil
	}
	servicesManager, err := rtutils.CreateServiceManager(command.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("create services manager for PSResource property stamping: %w", err)
	}
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, strconv.FormatInt(time.Now().UnixMilli(), 10))
	props = civcs.MergeWithUserProps(props, command.workingDirectory)
	specFiles := &spec.SpecFiles{}
	for _, pattern := range patterns {
		specFiles.Files = append(specFiles.Files, spec.File{Pattern: pattern})
	}

	var reader *content.ContentReader
	count, err := searchOnce(func() (int, error) {
		result, searchErr := generic.SearchItems(specFiles, servicesManager)
		if searchErr != nil {
			return 0, searchErr
		}
		length, lengthErr := result.Length()
		if lengthErr != nil {
			_ = result.Close()
			return 0, lengthErr
		}
		if length > 0 {
			reader = result
		} else {
			_ = result.Close()
		}
		return length, nil
	})
	if err == nil && count == 0 {
		return fmt.Errorf("%w\nsearched: %s (no items returned)", errPSResourcePackageNotFound, strings.Join(patterns, ", "))
	}
	if err != nil {
		return fmt.Errorf("resolve the published PSResource package for property stamping: %w", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("Failed to close PSResource property search reader:", closeErr.Error())
		}
	}()

	setErr := setPropsWithRetry(func() error {
		_, propsErr := servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props})
		if propsErr != nil {
			reader.Reset()
		}
		return propsErr
	})
	if setErr != nil {
		if isForbiddenError(setErr) {
			return fmt.Errorf("stamp build properties on the published PSResource package: %w"+
				"\nhint: annotate permission is required to set properties, and it is granted separately from deploy",
				setErr)
		}
		return fmt.Errorf("stamp build properties on the published PSResource package: %w", setErr)
	}
	return nil
}

func artifactPatterns(artifacts []entities.Artifact) []string {
	patterns := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.OriginalDeploymentRepo != "" && artifact.Path != "" {
			patterns = append(patterns, artifact.OriginalDeploymentRepo+"/"+strings.TrimPrefix(artifact.Path, "/"))
		}
	}
	return patterns
}

// searchOnce runs searchFn with retries, treating both a retryable search error and a successful
// but empty (zero-count) result as worth another attempt: a freshly published artifact was already
// confirmed to exist via a direct HEAD before this is ever called, so a zero-count search means
// Artifactory's (asynchronous) search index hasn't caught up yet, not that the artifact is missing.
// Without this, a routine, common indexing delay right after Publish-PSResource turned into an
// immediate, unretried "not found" that discarded the just-collected build-info.
//
// The returned error is nil whenever no genuine search error ever occurred - including when retries
// were exhausted on nothing but zero-count results - so callers can keep relying on
// "err == nil && count == 0" to mean "still not found after giving it every chance", exactly as
// they could before retries existed for this case.
func searchOnce(searchFn func() (int, error)) (int, error) {
	var count int
	var lastSearchErr error
	err := executeWithPSResourceRetry("Failed to search for the published PSResource package", func() (bool, error) {
		var searchErr error
		count, searchErr = searchFn()
		lastSearchErr = searchErr
		if searchErr != nil {
			return isRetryableError(searchErr), searchErr
		}
		return count == 0, nil
	})
	if lastSearchErr == nil {
		return count, nil
	}
	return count, err
}

func setPropsWithRetry(setPropsFn func() error) error {
	return retryOnPSResourceError("Failed to stamp build properties on the published PSResource package", setPropsFn)
}

func isForbiddenError(err error) bool {
	var httpErr *errorutils.HttpResponseError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusForbidden
}

func saveBuildInfoLocally(buildInfo *entities.BuildInfo, projectKey string) error {
	service := buildutils.CreateBuildInfoService()
	build, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, projectKey)
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}
	if err := build.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}
	log.Info(fmt.Sprintf("PSResource build info collected. Use 'jf rt bp %s %s' to publish it.", buildInfo.Name, buildInfo.Number))
	return nil
}
