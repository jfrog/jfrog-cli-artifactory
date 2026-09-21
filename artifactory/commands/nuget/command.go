package nuget

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
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
	rtutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
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

	// Inject credentials per NuGet's credential priority hierarchy so no nuget.config is
	// created or modified. Customers who manage their own credentials simply omit
	// --repo-resolve/--repo; FlexPack then skips injection and collects build-info only.
	if c.serverDetails != nil {
		repo := c.repoResolve
		if isPushCommand(c.subCommand) {
			repo = c.repoDeploy
		}
		if repo != "" && isRestoreCommand(c.subCommand) {
			// Inject credentials via a temp nuget.config for all restore-family commands.
			// Both nuget.exe and dotnet CLI use -ConfigFile / --configfile so credentials
			// are never embedded in the process argv (invisible to ps/proc); the flag style
			// is selected inside injectCredentialsViaTempConfig based on toolchainType.
			// Push, pack, and passthrough commands are excluded: push goes through
			// pushPackagesToArtifactory (the shared upload service, which authenticates from
			// the configured server details), and pack/passthrough are local-only.
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
		var snapErr error
		packSnapshot, snapErr = nugetflex.SnapshotPackageFiles(c.workingDir, extraDirs...)
		if snapErr != nil {
			return snapErr
		}
	}

	// Push bypasses the native tool and uploads through the shared Artifactory upload service.
	//
	// For nuget.exe this is a design choice, not a necessity: Artifactory does accept nuget.exe's
	// X-NuGet-ApiKey header — access tokens included — but only when the value is
	// "<username>:<token>", since it splits the header on the colon to recover credentials. A
	// bare token has nothing to split and is rejected. Note also that nuget.exe only sends that
	// header when an API key is actually resolved (-ApiKey, NUGET_API_KEY, or <apikeys> in a
	// config file); credentials supplied via <packageSourceCredentials> go out as Basic auth
	// instead, which is why push is excluded from the temp nuget.config injection above.
	//
	// For dotnet the problem is real and unrelated: dotnet nuget push cannot load a V3
	// index.json with URL-embedded credentials (401), because it does not forward that auth on
	// the service-index fetch.
	//
	// Uploading via the upload service sidesteps both, authenticates from the configured JFrog
	// server details, and shares the code path used by npm/alpine/terraform — inheriting proxy
	// handling, retries and checksum-optimised deploys. When the user supplies their own
	// -Source/-ApiKey the bypass is skipped and their intent wins.
	//
	// resolvedPushPaths is set when the Artifactory bypass handles the push so that
	// collectAndStampPushArtifacts can reuse the already-resolved paths instead of
	// re-expanding globs from c.args a second time.
	var resolvedPushPaths []string
	if isPushCommand(c.subCommand) && c.serverDetails != nil && c.repoDeploy != "" && !hasNativeAuthOverride(c.args) {
		log.Info("Pushing NuGet package to Artifactory...")
		var pushErr error
		resolvedPushPaths, pushErr = c.pushPackagesToArtifactory()
		if pushErr != nil {
			return fmt.Errorf("nuget push: %w", pushErr)
		}
	} else {
		log.Info(fmt.Sprintf("Running %s %s", c.toolchainType, c.subCommand))
		nativeCmd := c.buildCmd()
		nativeCmd.Stdin = os.Stdin
		nativeCmd.Stdout = os.Stdout
		nativeCmd.Stderr = os.Stderr
		if err := nativeCmd.Run(); err != nil {
			return fmt.Errorf("%s %s failed: %w", c.toolchainType, c.subCommand, err)
		}
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
		return c.collectAndStampPushArtifacts(buildName, buildNumber, resolvedPushPaths)
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

// injectCredentialsViaTempConfig writes a temporary nuget.config with the Artifactory
// source URL and <packageSourceCredentials>, then appends -ConfigFile / --configfile to
// c.args so the native tool reads it. The returned cleanup func removes the temp file
// and restores c.args to its original value. The caller must defer it immediately after
// a nil-error return.
//
// A V3 source URL is used. nuget.exe (mono) re-embeds -Source values into MSBuild's
// /p:RestoreSources, but sources read from /p:RestoreConfigFile are NOT re-embedded, so
// the V3 index.json URL in the config file is passed as-is and NU1301 is avoided.
// ClearTextPassword is used (not Password) because nuget.exe's encrypted Password
// storage is Windows DPAPI-only; ClearTextPassword works on all platforms including mono/macOS.
func (c *NuGetFlexPackCommand) injectCredentialsViaTempConfig(repo string) (func(), error) {
	sourceURL, user, password, err := NuGetExeV3SourceDetails(c.serverDetails, repo)
	if err != nil {
		return nil, fmt.Errorf("get NuGet source details: %w", err)
	}

	const sourceName = "JFrog"

	// NuGet 6.8+ rejects HTTP sources unless allowInsecureConnections="true" is set.
	// Local Artifactory instances in CI typically run on plain HTTP.
	allowInsecure := ""
	if c.allowInsecureConnections || strings.HasPrefix(strings.ToLower(sourceURL), "http://") {
		allowInsecure = ` allowInsecureConnections="true"`
	}

	// <clear/> ensures no other sources (nuget.org, system config) interfere — all traffic
	// is routed exclusively through Artifactory.
	configContent := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <clear />
    <add key=` + xmlAttrValue(sourceName) + ` value=` + xmlAttrValue(sourceURL) + allowInsecure + ` />
  </packageSources>
  <packageSourceCredentials>
    <` + sourceName + `>
      <add key="Username" value=` + xmlAttrValue(user) + ` />
      <add key="ClearTextPassword" value=` + xmlAttrValue(password) + ` />
    </` + sourceName + `>
  </packageSourceCredentials>
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
	c.args = append(c.args, configFlag, tmpFile.Name())

	return func() {
		c.args = origArgs
		_ = os.Remove(tmpFile.Name())
	}, nil
}

// pushPackagesToArtifactory resolves all .nupkg/.snupkg paths from push args (expanding globs),
// uploads each through the shared Artifactory upload service, and replicates nuget.exe's sibling
// .snupkg auto-push behaviour. Using the upload service instead of driving the native push tool
// keeps authentication, proxy handling, retries and checksum-optimised deploys consistent with
// the other package managers, and avoids dotnet nuget push's inability to authenticate against
// a V3 index.json with URL-embedded credentials (401). See Run for the full rationale.
// It returns the resolved absolute paths of all packages that were pushed so callers can
// reuse them without re-expanding globs from c.args.
func (c *NuGetFlexPackCommand) pushPackagesToArtifactory() ([]string, error) {
	// Warn about flags that the native tool would have honoured but the bypass cannot
	// forward. Flags in this list are silently accepted (or handled below); any unrecognised
	// flag produces a visible warning. Includes both nuget.exe style (single-dash) and dotnet
	// CLI style (double-dash) equivalents for each option.
	recognisedPushFlags := map[string]bool{
		// skip-duplicate: handled explicitly below
		"-skipduplicate": true, "--skip-duplicate": true,
		// no-symbols: handled explicitly below
		"-nosymbols": true, "--no-symbols": true, "-n": true,
		// timeout: advisory to the native tool only; irrelevant for the direct PUT
		"-timeout": true, "--timeout": true,
		// verbosity: no-op for the bypass (we use jf log levels)
		"-verbosity": true, "--verbosity": true, "-v": true,
		// disable-buffering: streaming hint for the native tool; no-op for the bypass
		"-disablebuffering": true, "--disable-buffering": true,
		// non-interactive / --interactive: interactive auth prompts are not used by the bypass
		"-noninteractive": true, "--interactive": true,
		// config-file: the bypass authenticates from the configured JFrog server; no config needed
		"-configfile": true, "--configfile": true,
		// force-english-output: locale hint for the native tool; no-op for the bypass
		"-forceenglishoutput": true, "--force-english-output": true,
	}
	for _, arg := range c.args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		flagOnly := strings.ToLower(arg)
		if idx := strings.IndexByte(flagOnly, '='); idx != -1 {
			flagOnly = flagOnly[:idx]
		}
		if !recognisedPushFlags[flagOnly] {
			log.Warn(fmt.Sprintf("Flag %q is not forwarded when pushing directly to Artifactory; it will have no effect.", arg))
		}
	}

	packages, err := resolvePackagePaths(c.workingDir, c.args)
	if err != nil {
		return nil, fmt.Errorf("resolve package paths: %w", err)
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("no .nupkg or .snupkg files found in push arguments: %v", c.args)
	}
	// Replicate nuget.exe behaviour: when pushing a .nupkg, also push the sibling .snupkg
	// if one exists alongside it (unless -NoSymbols was passed).
	if !hasNoSymbols(c.args) {
		packages = appendSiblingSymbolPackages(packages)
	}

	servicesManager, err := rtutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return nil, fmt.Errorf("create services manager for NuGet push: %w", err)
	}
	// Validate the target and resolve a virtual repo to the local repo artifacts land in, so the
	// upload target and the OriginalDeploymentRepo in build-info both name the local repository.
	deployRepo, err := resolveAndValidateDeployRepo(servicesManager, c.repoDeploy)
	if err != nil {
		return nil, err
	}

	// Derive each package's Artifactory storage path with the same build-info logic that
	// collectAndStampPushArtifacts uses, so the upload target and the later property-stamping
	// path can never diverge: .nupkg lands flat at the repository root, .snupkg under
	// symbolpackage/<id>.<version>.nupkg.
	artifacts, err := nugetflex.CollectPushArtifacts(c.workingDir, packages, deployRepo)
	if err != nil {
		return nil, fmt.Errorf("resolve NuGet storage paths: %w", err)
	}
	targetByName := make(map[string]string, len(artifacts))
	for _, a := range artifacts {
		targetByName[a.Name] = deployRepo + "/" + strings.TrimPrefix(a.Path, "/")
	}

	skipDuplicate := hasSkipDuplicate(c.args)
	for _, pkgPath := range packages {
		target, ok := targetByName[filepath.Base(pkgPath)]
		if !ok {
			return nil, fmt.Errorf("could not resolve the Artifactory path for %q", pkgPath)
		}
		if err := uploadPackage(servicesManager, pkgPath, target, skipDuplicate); err != nil {
			return nil, err
		}
	}
	return packages, nil
}

// resolveAndValidateDeployRepo validates that repoKey is a NuGet local or virtual repository and
// returns the local repository key that artifacts actually land in.
//
// Validation is explicit because packages are uploaded through the generic artifact API, which
// would otherwise silently accept a .nupkg into, say, a Maven repository. The NuGet gallery
// endpoint used to reject that implicitly (it does not exist for non-NuGet repos).
//
// For a virtual repository the returned key is its defaultDeploymentRepo, so both the upload
// target and the OriginalDeploymentRepo recorded in build-info name the local repository.
// Recording the virtual key would make downstream tools 404 when they try to locate the artifact.
func resolveAndValidateDeployRepo(servicesManager artifactory.ArtifactoryServicesManager, repoKey string) (string, error) {
	var params services.VirtualRepositoryBaseParams
	if err := servicesManager.GetRepository(repoKey, &params); err != nil {
		return "", fmt.Errorf("resolve repository %q: %w", repoKey, err)
	}
	if !strings.EqualFold(params.PackageType, "nuget") {
		return "", fmt.Errorf("repository %q is of type %q, not NuGet; NuGet packages cannot be pushed to it", repoKey, params.PackageType)
	}
	if strings.EqualFold(params.Rclass, "remote") {
		return "", fmt.Errorf("repository %q is a remote repository; NuGet packages can only be pushed to a local or virtual repository", repoKey)
	}
	if !strings.EqualFold(params.Rclass, "virtual") {
		return repoKey, nil
	}
	if params.DefaultDeploymentRepo == "" {
		return "", fmt.Errorf("virtual repo %q has no defaultDeploymentRepo configured; cannot determine the local repo to push to", repoKey)
	}
	log.Debug(fmt.Sprintf("Resolved virtual repo %q → local repo %q for push", repoKey, params.DefaultDeploymentRepo))
	return params.DefaultDeploymentRepo, nil
}

// appendSiblingSymbolPackages returns packages plus the sibling .snupkg of every .nupkg that
// has one on disk, preserving order and skipping any path already present.
func appendSiblingSymbolPackages(packages []string) []string {
	seen := make(map[string]bool, len(packages))
	for _, p := range packages {
		seen[p] = true
	}
	withSymbols := packages
	for _, pkgPath := range packages {
		if !strings.HasSuffix(strings.ToLower(pkgPath), ".nupkg") {
			continue
		}
		snupkgPath := pkgPath[:len(pkgPath)-len(".nupkg")] + ".snupkg"
		if seen[snupkgPath] {
			continue
		}
		if _, statErr := os.Stat(snupkgPath); statErr == nil {
			seen[snupkgPath] = true
			withSymbols = append(withSymbols, snupkgPath)
		}
	}
	return withSymbols
}

// uploadPackage deploys a single package file to target ("<repo>/<path>") using the shared
// Artifactory upload service, which handles authentication, proxies, retries and checksum
// optimisation. When skipDuplicate is set, an existing artifact at target is left untouched.
func uploadPackage(servicesManager artifactory.ArtifactoryServicesManager, pkgPath, target string, skipDuplicate bool) error {
	if skipDuplicate {
		exists, err := artifactExists(servicesManager, target)
		if err != nil {
			return err
		}
		if exists {
			log.Warn(fmt.Sprintf("Package %q already exists — skipping duplicate", filepath.Base(pkgPath)))
			return nil
		}
	}
	up := services.NewUploadParams()
	up.CommonParams = &specutils.CommonParams{Pattern: pkgPath, Target: target}
	up.Flat = true
	_, totalFailed, err := servicesManager.UploadFiles(artifactory.UploadServiceOptions{}, up)
	if err != nil {
		return fmt.Errorf("push %q: %w", filepath.Base(pkgPath), err)
	}
	if totalFailed > 0 {
		return fmt.Errorf("failed to push %q to Artifactory; see the Artifactory logs for details", filepath.Base(pkgPath))
	}
	log.Info(fmt.Sprintf("Package %q pushed successfully", filepath.Base(pkgPath)))
	return nil
}

// artifactExists reports whether repoPath ("<repo>/<path>") already exists in Artifactory.
func artifactExists(servicesManager artifactory.ArtifactoryServicesManager, repoPath string) (bool, error) {
	httpDetails := servicesManager.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	itemURL := strings.TrimSuffix(servicesManager.GetConfig().GetServiceDetails().GetUrl(), "/") + "/" + repoPath
	resp, _, err := servicesManager.Client().SendHead(itemURL, &httpDetails)
	if err != nil {
		return false, fmt.Errorf("check whether %q already exists: %w", repoPath, err)
	}
	return resp.StatusCode == http.StatusOK, nil
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

// resolvePackagePaths returns all .nupkg and .snupkg file paths from args, expanding
// glob patterns relative to workingDir. Flags (args starting with -) are skipped.
func resolvePackagePaths(workingDir string, args []string) ([]string, error) {
	var paths []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(arg))
		if ext != ".nupkg" && ext != ".snupkg" {
			continue
		}
		pattern := arg
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(workingDir, pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("expand glob %q: %w", arg, err)
		}
		// Re-filter glob matches to only include NuGet package files; a glob like
		// "bin/*" could expand to non-package files if the pattern is broad.
		for _, m := range matches {
			ext := strings.ToLower(filepath.Ext(m))
			if ext == ".nupkg" || ext == ".snupkg" {
				paths = append(paths, m)
			}
		}
	}
	return paths, nil
}

// hasSkipDuplicate reports whether the skip-duplicate flag is present in args.
// Handles both nuget.exe style (-SkipDuplicate) and dotnet CLI style (--skip-duplicate).
func hasSkipDuplicate(args []string) bool {
	for _, arg := range args {
		switch strings.ToLower(arg) {
		case "-skipduplicate", "--skip-duplicate":
			return true
		}
	}
	return false
}

// hasNoSymbols reports whether the no-symbols flag is present in args.
// Handles nuget.exe style (-NoSymbols), dotnet CLI style (--no-symbols), and the short
// form (-n used by dotnet nuget push).
func hasNoSymbols(args []string) bool {
	for _, arg := range args {
		switch strings.ToLower(arg) {
		case "-nosymbols", "--no-symbols", "-n":
			return true
		}
	}
	return false
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
//
// resolvedPaths contains the absolute package paths already resolved by pushPackagesToArtifactory
// (Artifactory bypass path). When nil (native-tool push path), paths are re-resolved from c.args.
func (c *NuGetFlexPackCommand) collectAndStampPushArtifacts(buildName, buildNumber string, resolvedPaths []string) error {
	log.Info(fmt.Sprintf("Collecting NuGet artifact info for %s/%s", buildName, buildNumber))
	// Resolve the actual local repo so OriginalDeploymentRepo is always a local repo key.
	// When the user pushes to a virtual repo, Artifactory routes to its defaultDeploymentRepo;
	// we need that local key in build-info so downstream tools can locate the artifact.
	deployRepo, err := c.resolveLocalDeployRepo(c.repoDeploy)
	if err != nil {
		return fmt.Errorf("resolve deployment repo: %w", err)
	}
	// Use pre-resolved paths when available (Artifactory bypass) to avoid re-expanding globs.
	// Pass them as pushArgs: resolvePushPackagePaths handles absolute literal paths correctly.
	pushArgs := c.args
	if len(resolvedPaths) > 0 {
		pushArgs = resolvedPaths
	}
	artifacts, err := nugetflex.CollectPushArtifacts(c.workingDir, pushArgs, deployRepo)
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
// package at its exact, deterministic Artifactory path. Primary packages (.nupkg) are stored
// flat at the repository root; symbol packages (.snupkg) are stored at
// symbolpackage/<id>.<version>.nupkg. Both paths are captured in artifact.Path by
// newArtifactFromFile. Fully-qualified patterns are used so no repository-wide scan is performed.
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
func hasSlnxTarget(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(filepath.Ext(arg), ".slnx") {
			return true
		}
	}
	return false
}

// isRestoreCommand returns true for commands that download packages (need dependency collection).
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
