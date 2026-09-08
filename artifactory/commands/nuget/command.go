package nuget

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	dotnetutils "github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/build-info-go/entities"
	buildinfoflex "github.com/jfrog/build-info-go/flexpack"
	nugetflex "github.com/jfrog/build-info-go/flexpack/nuget"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	rtutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// NuGetFlexPackCommand runs a NuGet or dotnet command natively and collects build-info.
type NuGetFlexPackCommand struct {
	toolchainType            dotnetutils.ToolchainType
	subCommand               string
	args                     []string
	serverDetails            *config.ServerDetails
	repoResolve              string
	repoDeploy               string
	useNugetV2               bool
	allowInsecureConnections bool
	buildConfiguration       *buildUtils.BuildConfiguration
	workingDir               string
	// credentialEnv is the NuGetPackageSourceCredentials_<source> entry handed to the native
	// client's environment. Empty when no credentials were injected. Set by
	// injectCredentialsViaTempConfig and reset by its cleanup func.
	credentialEnv string
}

// NewNuGetFlexPackCommand creates a new NuGetFlexPackCommand.
func NewNuGetFlexPackCommand() *NuGetFlexPackCommand {
	return &NuGetFlexPackCommand{}
}

func (c *NuGetFlexPackCommand) SetToolchainType(t dotnetutils.ToolchainType) *NuGetFlexPackCommand {
	c.toolchainType = t
	return c
}

func (c *NuGetFlexPackCommand) SetSubCommand(s string) *NuGetFlexPackCommand {
	c.subCommand = s
	return c
}

func (c *NuGetFlexPackCommand) SetArgs(a []string) *NuGetFlexPackCommand {
	c.args = a
	return c
}

func (c *NuGetFlexPackCommand) SetServerDetails(s *config.ServerDetails) *NuGetFlexPackCommand {
	c.serverDetails = s
	return c
}

func (c *NuGetFlexPackCommand) SetRepoResolve(r string) *NuGetFlexPackCommand {
	c.repoResolve = r
	return c
}

func (c *NuGetFlexPackCommand) SetRepoDeploy(r string) *NuGetFlexPackCommand {
	c.repoDeploy = r
	return c
}

func (c *NuGetFlexPackCommand) SetUseNugetV2(v bool) *NuGetFlexPackCommand {
	c.useNugetV2 = v
	return c
}

func (c *NuGetFlexPackCommand) SetAllowInsecureConnections(a bool) *NuGetFlexPackCommand {
	c.allowInsecureConnections = a
	return c
}

func (c *NuGetFlexPackCommand) SetBuildConfiguration(b *buildUtils.BuildConfiguration) *NuGetFlexPackCommand {
	c.buildConfiguration = b
	return c
}

func (c *NuGetFlexPackCommand) SetWorkingDir(d string) *NuGetFlexPackCommand {
	c.workingDir = d
	return c
}

// RequiresServerDetails reports whether the command needs JFrog server configuration.
// RequiresServerDetails returns true only when the command will actually use server
// credentials: push targeting a deploy repo, or restore targeting a resolve repo.
// Anonymous push/restore, pack, and passthrough commands do not require server details.
func (c *NuGetFlexPackCommand) RequiresServerDetails() bool {
	return (isPushCommand(c.subCommand) && c.repoDeploy != "") ||
		(isRestoreCommand(c.subCommand) && c.repoResolve != "")
}

func (c *NuGetFlexPackCommand) CommandName() string { return "rt_nuget_flexpack" }
func (c *NuGetFlexPackCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// Run executes the native NuGet/dotnet command and collects build-info.
func (c *NuGetFlexPackCommand) Run() error {
	workingDir := c.workingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		c.workingDir = workingDir
	}

	// .slnx is an SDK-only solution format; nuget.exe has no parser for it.
	if c.toolchainType == dotnetutils.Nuget && hasSlnxTarget(c.args) {
		return fmt.Errorf(".slnx solution files are not supported by nuget.exe; use 'jf dotnet restore' instead")
	}

	// Decide up front whether the native client performs the upload itself, so the decision is
	// made against the user's own arguments before jf appends anything to them.
	pushViaNativeClient := c.shouldPushViaNativeClient()

	// Inject credentials per NuGet's credential priority hierarchy so no nuget.config is
	// created or modified. Customers who manage their own credentials simply omit
	// --repo-resolve/--repo; FlexPack then skips injection and collects build-info only.
	// A configured server means one with a URL, not merely a non-nil struct: when nothing is
	// configured, GetSpecificConfig hands back an empty *ServerDetails and a nil error, and an
	// empty ArtifactoryUrl turns the source into the relative path "api/nuget/v3/<repo>/index.json"
	// that NuGet then reports as a missing LOCAL folder ("NU1301: The local source ... doesn't
	// exist"), never naming the real problem.
	repo := c.repoResolve
	if isPushCommand(c.subCommand) {
		repo = c.repoDeploy
	}
	if repo != "" && (c.serverDetails == nil || c.serverDetails.ArtifactoryUrl == "") {
		return fmt.Errorf("a repository was requested (%q) but no JFrog server is configured; run 'jf c add' or pass --server-id", repo)
	}
	if c.serverDetails != nil && c.serverDetails.ArtifactoryUrl != "" {
		if repo != "" && hasUserConfigFile(c.args) {
			// The user brought their own config file. Injecting a second -ConfigFile/--configfile
			// is not additive: the dotnet CLI rejects the duplicate outright ("Option
			// '--configfile' expects a single argument but 2 were provided"), and nuget.exe
			// silently honours only the last one, discarding the user's sources and any
			// packageSourceCredentials they declared for their other private feeds. Step aside
			// and say so, the same way an explicit -Source/-ApiKey suppresses injection.
			log.Warn(fmt.Sprintf("A NuGet config file was supplied on the command line, so %q is being used as-is and no credentials are injected for repository %q. Remove the config file flag to let JFrog CLI configure the source, or add the Artifactory source to that file yourself.", userConfigFilePath(c.args), repo))
		} else if repo != "" && performsRestore(c.subCommand) && !c.acceptsConfigFile() {
			// The subcommand restores packages but has no config-file option to inject into.
			// Say so rather than letting --repo-resolve look effective: the restore will go to
			// whatever sources the user's own configuration names.
			log.Warn(fmt.Sprintf("'%s %s' does not accept a NuGet config file, so --repo-resolve=%s cannot be applied and packages will resolve from your configured sources. Run 'jf %s restore --repo-resolve=%s' first, or pass the source explicitly.", c.toolchainType, c.subCommand, repo, c.toolchainType, repo))
		} else if repo != "" && (performsRestore(c.subCommand) || pushViaNativeClient) {
			// Inject credentials via a temp nuget.config for restore-family commands and for
			// pushes the native client performs. Both nuget.exe and dotnet CLI use
			// -ConfigFile / --configfile so credentials are never embedded in the process argv
			// (invisible to ps/proc); the flag style is selected inside
			// injectCredentialsViaTempConfig based on toolchainType. The same file also carries
			// defaultPushSource, so push needs no source flag on the command line.
			// Pack and passthrough commands are excluded: they are local-only.
			cleanup, err := c.injectCredentialsViaTempConfig(repo)
			if err != nil {
				return err
			}
			defer cleanup()
		}
	}

	// For pack, snapshot existing package files before running so we can deterministically
	// identify the packages this command produces (including custom --output directories and
	// bin/<Configuration> defaults), instead of scanning the working directory for stale files.
	var packSnapshot nugetflex.PackageSnapshot
	var packOutputDir string
	if isPackCommand(c.subCommand) {
		packOutputDir = extractPackOutputDir(c.args)
		var extraDirs []string
		if packOutputDir != "" {
			extraDirs = append(extraDirs, packOutputDir)
		}
		// Without --output, each project writes to its OWN bin/<Configuration>. When the target
		// lives below the working directory - "pack src/Lib/Lib.csproj", or any .sln whose
		// projects sit in sub-directories - none of that is under <workingDir>/bin, so nothing
		// would be collected and build-info would be persisted with no modules at all, while the
		// command still reported success. Snapshot the target's own directory too.
		extraDirs = append(extraDirs, packTargetDirs(c.workingDir, c.args)...)
		var snapErr error
		packSnapshot, snapErr = nugetflex.SnapshotPackageFiles(c.workingDir, extraDirs...)
		if snapErr != nil {
			return snapErr
		}
	}

	// Every command - push included - is executed by the native client. jf's role is to supply
	// credentials through a temporary nuget.config and to observe the result for build-info; it
	// does not upload on the tool's behalf. See shouldPushViaNativeClient for why the push no
	// longer goes through the Artifactory upload service.
	log.Info(fmt.Sprintf("Running %s %s", c.toolchainType, c.subCommand))
	nativeCmd := c.buildCmd()
	nativeCmd.Stdin = os.Stdin
	nativeCmd.Stdout = os.Stdout
	nativeCmd.Stderr = os.Stderr
	// Inherit the caller's environment and add the source credentials, so the native client
	// authenticates without a secret being written into the temp nuget.config.
	if c.credentialEnv != "" {
		nativeCmd.Env = append(os.Environ(), c.credentialEnv)
	}
	if err := nativeCmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w", c.toolchainType, c.subCommand, err)
	}

	if c.buildConfiguration == nil {
		return nil
	}
	buildName, err := c.buildConfiguration.GetBuildName()
	if err != nil || buildName == "" {
		return nil
	}
	buildNumber, err := c.buildConfiguration.GetBuildNumber()
	if err != nil || buildNumber == "" {
		return nil
	}

	switch {
	case isRestoreCommand(c.subCommand):
		return c.collectDependencies(buildName, buildNumber)
	case isPushCommand(c.subCommand):
		return c.collectAndStampPushArtifacts(buildName, buildNumber)
	case isPackCommand(c.subCommand):
		return c.collectPackArtifacts(buildName, buildNumber, packSnapshot, packOutputDir)
	}
	return nil
}

// buildCmd builds the exec.Cmd for the native nuget.exe or dotnet CLI.
// Credentials are already injected into c.args (via -ConfigFile) before this is called.
func (c *NuGetFlexPackCommand) buildCmd() *exec.Cmd {
	if c.toolchainType == dotnetutils.DotnetCore {
		return exec.Command("dotnet", append(strings.Fields(c.subCommand), c.args...)...)
	}
	return exec.Command("nuget", append([]string{c.subCommand}, c.args...)...)
}

// injectCredentialsViaTempConfig writes a temporary nuget.config declaring the Artifactory
// source, then appends -ConfigFile / --configfile to c.args so the native tool reads it, and
// records the matching credential environment entry on c.credentialEnv. The returned cleanup
// func removes the temp file and restores c.args and c.credentialEnv to their original values.
// The caller must defer it immediately after a nil-error return.
//
// The config file carries no secret: credentials travel in the environment (see
// credentialEnvEntry), which keeps them off disk and out of argv.
//
// A V3 source URL is used. nuget.exe (mono) re-embeds -Source values into MSBuild's
// /p:RestoreSources, but sources read from /p:RestoreConfigFile are NOT re-embedded, so
// the V3 index.json URL in the config file is passed as-is and NU1301 is avoided.
func (c *NuGetFlexPackCommand) injectCredentialsViaTempConfig(repo string) (func(), error) {
	sourceURL, user, password, err := NuGetExeV3SourceDetails(c.serverDetails, repo)
	if err != nil {
		return nil, fmt.Errorf("get NuGet source details: %w", err)
	}

	const sourceName = injectedSourceName

	// NuGet 6.8+ rejects HTTP sources unless allowInsecureConnections="true" is set.
	// Local Artifactory instances in CI typically run on plain HTTP.
	allowInsecure := ""
	if c.allowInsecureConnections || strings.HasPrefix(strings.ToLower(sourceURL), "http://") {
		allowInsecure = ` allowInsecureConnections="true"`
	}

	// <clear/> ensures no other sources (nuget.org, system config) interfere — all traffic
	// is routed exclusively through Artifactory.
	// defaultPushSource names the same source as the push target so that 'nuget push' and
	// 'dotnet nuget push' find it without jf appending -Source/--source to the user's command
	// line. Keeping the target in configuration rather than argv means the native client is
	// invoked exactly as the user wrote it, and avoids branching on per-toolchain flag
	// spelling. It is inert for restore, which never consults defaultPushSource.
	//
	// Note the absence of a <packageSourceCredentials> block: credentials are handed to the
	// native client through the NuGetPackageSourceCredentials_<source> environment variable
	// instead (see credentialEnvEntry), so no secret is ever written to disk.
	configContent := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <clear />
    <add key=` + xmlAttrValue(sourceName) + ` value=` + xmlAttrValue(sourceURL) + allowInsecure + ` />
  </packageSources>
  <config>
    <add key="defaultPushSource" value=` + xmlAttrValue(sourceName) + ` />
  </config>
</configuration>`

	tmpFile, err := os.CreateTemp("", "jfrog-nuget-*.config")
	if err != nil {
		return nil, fmt.Errorf("create temp nuget.config: %w", err)
	}
	if _, err := tmpFile.WriteString(configContent); err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("write temp nuget.config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("close temp nuget.config: %w", err)
	}

	// nuget.exe uses single-dash POSIX style; dotnet CLI uses double-dash POSIX style.
	configFlag := "-ConfigFile"
	if c.toolchainType != dotnetutils.Nuget {
		configFlag = "--configfile"
	}
	origArgs := c.args
	c.args = insertBeforeSeparator(c.args, configFlag, tmpFile.Name())

	origCredentialEnv := c.credentialEnv
	// Only pass credentials when there are credentials. With both empty the entry would be
	// "Username=;Password=", which NuGet sends as an empty Basic header rather than omitting
	// authentication - turning a working anonymous repository into a rejected request. The legacy
	// config writer makes the same distinction.
	if user != "" || password != "" {
		c.credentialEnv = credentialEnvEntry(sourceName, user, password)
	}

	return func() {
		c.args = origArgs
		c.credentialEnv = origCredentialEnv
		_ = os.Remove(tmpFile.Name())
	}, nil
}

// insertBeforeSeparator places extra arguments ahead of a bare "--" in args, appending them at
// the end when there is no separator.
//
// The ruby command package solves the identical problem in rubyAppendToolArgs
// (commands/ruby/native_ruby.go) - gem forwards everything after "--" to the C extension
// build the same way. Keep the two in step; they are a candidate for one shared helper.
//
// The dotnet CLI forwards everything after "--" to MSBuild, so appending blindly puts jf's own
// --configfile on the wrong side of it and the restore dies on MSBuild's own parser:
//
//	MSBUILD : error MSB1001: Unknown switch.
//	Switch: --configfile
//
// The flag belongs to the dotnet command itself, so it has to precede the separator. Only the
// first "--" is meaningful; anything after it is the user's payload and is left untouched.
func insertBeforeSeparator(args []string, extra ...string) []string {
	for i, arg := range args {
		if arg == "--" {
			combined := make([]string, 0, len(args)+len(extra))
			combined = append(combined, args[:i]...)
			combined = append(combined, extra...)
			combined = append(combined, args[i:]...)
			return combined
		}
	}
	return append(args, extra...)
}

// credentialEnvEntry builds the NuGet environment-variable credential entry for a package
// source, in the "KEY=Username=<u>;Password=<p>" form both nuget.exe and the dotnet CLI read.
//
// Passing credentials this way keeps them out of the temp nuget.config, so a secret is never
// written to disk and cannot survive a crash that skips cleanup. It also keeps them out of the
// process argv, which is world-readable via ps. The value is still visible to other processes
// running as the same user (ps -E, /proc/<pid>/environ), so this narrows the exposure rather
// than eliminating it - but it is the mechanism NuGet documents for exactly this purpose.
func credentialEnvEntry(sourceName, user, password string) string {
	return fmt.Sprintf("NuGetPackageSourceCredentials_%s=Username=%s;Password=%s", sourceName, user, password)
}

// searchWithRetry calls searchFn up to maxAttempts times with exponential backoff starting
// at initialDelay, returning the first positive count. Used to tolerate Artifactory's
// asynchronous NuGet indexing. Pass initialDelay=0 in tests to skip sleeping.
func searchWithRetry(maxAttempts int, initialDelay time.Duration, patterns []string, searchFn func() (int, error)) (int, error) {
	delay := initialDelay
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		n, err := searchFn()
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return n, nil
		}
		if attempt < maxAttempts {
			log.Debug(fmt.Sprintf("NuGet artifacts not yet indexed (attempt %d/%d); retrying in %s", attempt, maxAttempts, delay))
			if delay > 0 {
				time.Sleep(delay)
				delay *= 2
			}
		}
	}
	return 0, fmt.Errorf("no uploaded NuGet artifacts found at the expected paths after %d attempts: %s", maxAttempts, strings.Join(patterns, ", "))
}

// hasNativeAuthOverride reports whether the user passed a flag that explicitly controls
// NuGet's own auth for push. Covers both nuget.exe style (-Source, -ApiKey, -SymbolApiKey)
// and dotnet CLI style (--source, -s, --api-key, -k, --symbol-api-key). Handles both the
// space-separated form (flag as its own token) and the inline-equals form (--api-key=VALUE
// as a single token). When any of these are present the user's intent takes precedence over
// --repo and the bypass must not fire.
func hasNativeAuthOverride(args []string) bool {
	for _, arg := range args {
		// Strip an optional inline value (--flag=value → --flag) before matching.
		flag := strings.ToLower(arg)
		if idx := strings.IndexByte(flag, '='); idx != -1 {
			flag = flag[:idx]
		}
		switch flag {
		case "-source", "-s", "--source",
			"-apikey", "--api-key", "-k",
			"-symbolapikey", "--symbol-api-key",
			"-ss", "--symbol-source":
			return true
		}
	}
	return false
}

// configFileFlags are the spellings both clients accept for "read settings from this file":
// nuget.exe uses -ConfigFile, the dotnet CLI --configfile. Matching is case-insensitive because
// nuget.exe's own parser is.
var configFileFlags = map[string]bool{"-configfile": true, "--configfile": true}

// hasUserConfigFile reports whether the user already passed a NuGet config file, in either the
// space-separated or the inline-equals form. Both clients take a single value for it, so jf must
// not append another.
func hasUserConfigFile(args []string) bool {
	return userConfigFilePath(args) != ""
}

// userConfigFilePath returns the config file the user asked for, or "" if they did not. When the
// flag is present with no value the flag itself is returned, which is enough for a warning.
func userConfigFilePath(args []string) string {
	for i, arg := range args {
		flag, inlineValue, hasInline := strings.Cut(arg, "=")
		if !configFileFlags[strings.ToLower(flag)] {
			continue
		}
		if hasInline {
			return inlineValue
		}
		if i+1 < len(args) {
			return args[i+1]
		}
		return arg
	}
	return ""
}

func (c *NuGetFlexPackCommand) collectDependencies(buildName, buildNumber string) error {
	log.Info(fmt.Sprintf("Collecting NuGet build info for %s/%s", buildName, buildNumber))
	collector, err := nugetflex.NewNuGetFlexPack(buildinfoflex.NuGetConfig{
		WorkingDirectory: c.workingDir,
		TargetPath:       restoreTarget(c.workingDir, c.args),
		Module:           c.buildConfiguration.GetModule(),
	}, nil)
	if err != nil {
		return fmt.Errorf("create NuGet flexpack: %w", err)
	}
	bi, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("collect NuGet build info: %w", err)
	}
	log.Info(fmt.Sprintf("NuGet build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return saveBuildInfoLocally(bi, c.buildConfiguration.GetProject())
}

// collectAndStampPushArtifacts identifies the exact packages a push uploaded, stamps
// build properties on their exact Artifactory paths, and records them in local build-info.
// The native push has already succeeded at this point, so it is never re-run; a stamping
// failure is surfaced as an error without masking the push.
func (c *NuGetFlexPackCommand) collectAndStampPushArtifacts(buildName, buildNumber string) error {
	log.Info(fmt.Sprintf("Collecting NuGet artifact info for %s/%s", buildName, buildNumber))
	// Resolve the actual local repo so OriginalDeploymentRepo is always a local repo key.
	// When the user pushes to a virtual repo, Artifactory routes to its defaultDeploymentRepo;
	// we need that local key in build-info so downstream tools can locate the artifact.
	deployRepo, err := c.resolveLocalDeployRepo(c.repoDeploy)
	if err != nil {
		return fmt.Errorf("resolve deployment repo: %w", err)
	}
	// c.args is passed verbatim: CollectPushArtifacts needs the flags as well as the package
	// paths, since -NoSymbols / --no-symbols decides whether a sibling .snupkg is recorded.
	artifacts, err := nugetflex.CollectPushArtifacts(c.workingDir, c.args, deployRepo)
	if err != nil {
		return fmt.Errorf("collect pushed NuGet artifacts: %w", err)
	}
	if err := c.stampBuildProperties(artifacts, buildName, buildNumber); err != nil {
		return err
	}
	return c.saveArtifactsBuildInfo(buildName, buildNumber, artifacts)
}

// resolveLocalDeployRepo returns the local repo key where artifacts actually land.
// If repoKey is a virtual repo it returns the virtual repo's defaultDeploymentRepo;
// for local/remote repos it returns repoKey unchanged.
// Failures are hard errors: build-info must never record a virtual repo key that
// downstream tools would 404 on.
func (c *NuGetFlexPackCommand) resolveLocalDeployRepo(repoKey string) (string, error) {
	if repoKey == "" || c.serverDetails == nil {
		return repoKey, nil
	}
	servicesManager, err := rtutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return "", fmt.Errorf("create services manager to resolve repo %q: %w", repoKey, err)
	}
	var params services.VirtualRepositoryBaseParams
	if err := servicesManager.GetRepository(repoKey, &params); err != nil {
		return "", fmt.Errorf("resolve repo type for %q: %w", repoKey, err)
	}
	if params.Rclass != "virtual" {
		return repoKey, nil
	}
	if params.DefaultDeploymentRepo == "" {
		return "", fmt.Errorf("virtual repo %q has no defaultDeploymentRepo configured; cannot determine the local repo for build-info", repoKey)
	}
	log.Debug(fmt.Sprintf("Resolved virtual repo %q → local repo %q for OriginalDeploymentRepo", repoKey, params.DefaultDeploymentRepo))
	return params.DefaultDeploymentRepo, nil
}

// collectPackArtifacts records the packages produced by a pack command, detected by comparing
// the pre-command package snapshot with the current filesystem state. outputDir is the
// explicit --output directory passed to the pack command (empty string if not provided).
func (c *NuGetFlexPackCommand) collectPackArtifacts(buildName, buildNumber string, before nugetflex.PackageSnapshot, outputDir string) error {
	log.Info(fmt.Sprintf("Collecting NuGet artifact info for %s/%s", buildName, buildNumber))
	var extraDirs []string
	if outputDir != "" {
		extraDirs = append(extraDirs, outputDir)
	}
	artifacts, err := nugetflex.CollectPackedArtifacts(c.workingDir, before, c.repoDeploy, extraDirs...)
	if err != nil {
		return fmt.Errorf("collect packed NuGet artifacts: %w", err)
	}
	return c.saveArtifactsBuildInfo(buildName, buildNumber, artifacts)
}

// saveArtifactsBuildInfo builds and persists NuGet artifact modules for later publishing.
// Modules use the fixed "<PackageId>:<Version>" ID, or the user-supplied --module override.
func (c *NuGetFlexPackCommand) saveArtifactsBuildInfo(buildName, buildNumber string, artifacts []entities.Artifact) error {
	bi := &entities.BuildInfo{
		Name:    buildName,
		Number:  buildNumber,
		Modules: nugetflex.BuildArtifactModules(artifacts, c.buildConfiguration.GetModule()),
	}
	log.Info(fmt.Sprintf("NuGet artifact info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return saveBuildInfoLocally(bi, c.buildConfiguration.GetProject())
}

// artifactPatterns returns exact repository paths for property stamping. Invalid artifacts are
// ignored so callers never broaden a request to a repository-level pattern.
func artifactPatterns(artifacts []entities.Artifact) []string {
	patterns := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.OriginalDeploymentRepo == "" || artifact.Path == "" {
			continue
		}
		patterns = append(patterns, artifact.OriginalDeploymentRepo+"/"+strings.TrimPrefix(artifact.Path, "/"))
	}
	return patterns
}

// stampBuildProperties attaches build.name/build.number/build.timestamp to each uploaded
// package at its exact Artifactory path, taken verbatim from artifact.Path - build-info-go's
// newArtifactFromFile owns where a package lands, so no storage-layout knowledge is needed or
// duplicated here. Fully-qualified patterns are used so no repository-wide scan is performed.
func (c *NuGetFlexPackCommand) stampBuildProperties(artifacts []entities.Artifact, buildName, buildNumber string) error {
	if c.serverDetails == nil || c.repoDeploy == "" {
		// Anonymous push or no deploy repo: there is no JFrog target to stamp.
		return nil
	}
	patterns := artifactPatterns(artifacts)
	if len(patterns) == 0 {
		return nil
	}

	servicesManager, err := rtutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("create services manager for NuGet property stamping: %w", err)
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	// Stamp the CI/VCS coordinates too, so pushed NuGet packages carry the same vcs.*/ci.*
	// properties that the other FlexPack package managers (npm, Go, Maven, Terraform) attach.
	// Without these an artifact records which build produced it but not which commit, branch,
	// or pipeline run it came from. MergeWithUserProps is a no-op when the props are disabled
	// or no CI/Git context can be detected, so this is safe outside a repository.
	props = civcs.MergeWithUserProps(props, c.workingDir)

	specFiles := &spec.SpecFiles{}
	for _, pattern := range patterns {
		specFiles.Files = append(specFiles.Files, spec.File{Pattern: pattern})
	}
	// Artifactory indexes NuGet packages asynchronously after upload. Retry the search with
	// exponential backoff so a briefly-empty index does not cause a spurious error.
	var reader *content.ContentReader
	length, retryErr := searchWithRetry(5, 2*time.Second, patterns, func() (int, error) {
		r, searchErr := generic.SearchItems(specFiles, servicesManager)
		if searchErr != nil {
			return 0, fmt.Errorf("resolve uploaded NuGet artifacts for property stamping: %w", searchErr)
		}
		n, lenErr := r.Length()
		if lenErr != nil {
			_ = r.Close()
			return 0, fmt.Errorf("read search result length for NuGet property stamping: %w", lenErr)
		}
		if n > 0 {
			reader = r
		} else {
			_ = r.Close()
		}
		return n, nil
	})
	if retryErr != nil {
		return retryErr
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("Failed to close search reader:", closeErr.Error())
		}
	}()
	if _, err := servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props}); err != nil {
		return fmt.Errorf("stamp build properties on uploaded NuGet artifacts: %w", err)
	}
	log.Info(fmt.Sprintf("Stamped build properties on %d NuGet artifact(s).", length))
	return nil
}

// extractPackOutputDir returns the explicit --output / -OutputDirectory directory from pack
// command args, or an empty string when the flag is absent. Handles both nuget.exe style
// (-OutputDirectory <dir>) and dotnet CLI style (--output <dir> / -o <dir> / --output=<dir>).
func extractPackOutputDir(args []string) string {
	for i, arg := range args {
		lower := strings.ToLower(arg)
		// Inline-value form: --output=dir or -outputdirectory:dir
		for _, prefix := range []string{"--output=", "-outputdirectory=", "-outputdirectory:"} {
			if strings.HasPrefix(lower, prefix) {
				return arg[len(prefix):]
			}
		}
		// Space-separated form: --output dir / -o dir / -OutputDirectory dir
		if lower == "--output" || lower == "-o" || lower == "-outputdirectory" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}

// restoreTarget returns the solution, project, or directory explicitly supplied to the
// native restore command. It skips values belonging to known NuGet and dotnet restore
// options, then prefers an explicit solution/project over a directory target.
func restoreTarget(workingDir string, args []string) string {
	var directoryTarget string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			skipNext = restoreOptionTakesValue(arg)
			continue
		}

		ext := strings.ToLower(filepath.Ext(arg))
		if ext == ".sln" || ext == ".slnf" || ext == ".slnx" || strings.HasSuffix(ext, "proj") {
			return arg
		}
		if directoryTarget != "" {
			continue
		}
		path := arg
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			directoryTarget = arg
		}
	}
	return directoryTarget
}

// restoreOptionTakesValue reports whether a NuGet or dotnet restore option consumes the
// following argument. Values supplied inline (for example, --verbosity=minimal) do not.
func restoreOptionTakesValue(arg string) bool {
	if strings.ContainsAny(arg, "=:") {
		return false
	}
	switch strings.ToLower(arg) {
	case "-a", "--arch",
		"-c", "--configuration",
		"--configfile", "-configfile",
		"-f", "--framework",
		"--lock-file-path",
		"-msbuildpath", "-msbuildversion",
		"-o", "--output",
		"--os",
		"-outputdirectory",
		"--packages", "-packagesavemode", "-packagesdirectory",
		"-p", "--property", "-project2projecttimeout",
		"-r", "--runtime",
		"-s", "--source", "-source",
		"-solutiondirectory",
		"--tl",
		"-v", "--verbosity", "-verbosity":
		return true
	default:
		return false
	}
}

// hasSlnxTarget returns true if any positional arg is a .slnx file. Used to detect SDK-only
// solution formats before passing them to nuget.exe, which has no .slnx parser.
// performsRestore reports whether the subcommand downloads packages and therefore needs the
// Artifactory source declared.
//
// It is wider than isRestoreCommand: "pack" and "publish" restore implicitly unless --no-restore
// is passed, and both accept -ConfigFile/--configfile, which steers that restore (verified
// against SDK 10.0.302 - a config naming an unreachable source makes both fail in NuGet.targets).
// Leaving them out meant --repo-resolve was accepted, stripped from argv and then silently
// dropped, so the implicit restore went to whatever sources the user's own configuration named,
// typically nuget.org - no curation, no audit trail, no warning. Injecting for a command that
// turns out not to restore is harmless: the config file is simply unused.
func performsRestore(sub string) bool {
	return isRestoreCommand(sub) || isPackCommand(sub) || sub == "publish"
}

// packTargetSuffixes are the positional targets a pack command accepts. A directory argument is
// covered by the generic non-flag branch in packTargetDirs.
var packTargetSuffixes = []string{".csproj", ".fsproj", ".vbproj", ".sln", ".slnx", ".nuspec"}

// packTargetDirs returns the directories that hold the project or solution being packed, resolved
// against workingDir. Without an explicit --output every project writes to its own
// bin/<Configuration>, so these are where produced packages appear.
func packTargetDirs(workingDir string, args []string) []string {
	var dirs []string
	seen := map[string]bool{}
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if !strings.Contains(arg, "=") {
				skipNext = true
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(arg))
		isTarget := false
		for _, suffix := range packTargetSuffixes {
			if ext == suffix {
				isTarget = true
				break
			}
		}
		candidate := arg
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(workingDir, candidate)
		}
		dir := candidate
		if isTarget {
			dir = filepath.Dir(candidate)
		} else if info, err := os.Stat(candidate); err != nil || !info.IsDir() {
			// Neither a recognised project/solution file nor an existing directory.
			continue
		}
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func hasSlnxTarget(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(filepath.Ext(arg), ".slnx") {
			return true
		}
	}
	return false
}

// isRestoreCommand returns true for commands that download packages (need dependency collection).
// acceptsConfigFile reports whether the subcommand has a -ConfigFile/--configfile option to
// inject into. "dotnet add package" restores, and so is in the restore family, but the SDK gives
// it no config-file option at all - it takes -s/--source instead - so injecting one makes the
// command fail with an unknown-option error. nuget.exe's own "add" does accept -ConfigFile.
func (c *NuGetFlexPackCommand) acceptsConfigFile() bool {
	if c.toolchainType == dotnetutils.DotnetCore && c.subCommand == "add" {
		return false
	}
	return true
}

func isRestoreCommand(sub string) bool {
	switch sub {
	case "restore", "install", "update", "build", "add":
		return true
	}
	return false
}

// isPushCommand returns true for push subcommands.
func isPushCommand(sub string) bool {
	return sub == "push" || sub == "nuget push"
}

// injectedSourceName is the package-source key written into the temporary nuget.config, and
// the value of its defaultPushSource setting.
const injectedSourceName = "JFrog"

// shouldPushViaNativeClient reports whether the native client performs the upload itself
// instead of jf taking the publish over.
//
// FlexPack's contract is that the native tool does the work and jf observes it, so the upload
// belongs to nuget.exe / the dotnet CLI. Both push to Artifactory successfully when their
// credentials come from the NuGetPackageSourceCredentials_<source> environment variable and
// the target comes from defaultPushSource - which is what injectCredentialsViaTempConfig sets up.
//
// The 401 that originally motivated the Artifactory-side bypass is specific to credentials
// carried in the source URL: fetching the V3 service index that way is unauthenticated and
// fails for both clients. Supplying credentials through the config file avoids it entirely.
// Verified against Artifactory for nuget.exe 6.6.2 and dotnet SDK 10.0.302.
//
// Build-info collection and property stamping are unaffected - they run after the command
// either way, so the artifact record and build.*/vcs.* properties are identical.
//
// Skipped when the user supplied their own -Source/-ApiKey, or when there is no server or
// deploy repo to build a source from; those cases already run the native tool directly.
func (c *NuGetFlexPackCommand) shouldPushViaNativeClient() bool {
	return isPushCommand(c.subCommand) &&
		c.serverDetails != nil &&
		c.repoDeploy != "" &&
		!hasNativeAuthOverride(c.args)
}

// isPackCommand returns true for the pack subcommand, which produces .nupkg/.snupkg files locally.
func isPackCommand(sub string) bool {
	return sub == "pack"
}

// xmlAttrValue returns s with XML special characters escaped, wrapped in double quotes,
// suitable for use as an XML attribute value (e.g. key="foo&amp;bar").
// It uses encoding/xml.EscapeText to ensure &, <, >, ", and ' are properly escaped.
func xmlAttrValue(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		// EscapeText only fails on unsupported code points; fall back to the raw value.
		return `"` + s + `"`
	}
	return `"` + buf.String() + `"`
}

// saveBuildInfoLocally saves build-info for later publishing with 'jf rt bp'.
func saveBuildInfoLocally(bi *entities.BuildInfo, projectKey string) error {
	service := buildUtils.CreateBuildInfoService()
	build, err := service.GetOrCreateBuildWithProject(bi.Name, bi.Number, projectKey)
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}
	if err := build.SaveBuildInfo(bi); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}
	return nil
}
