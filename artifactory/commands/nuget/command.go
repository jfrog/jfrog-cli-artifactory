package nuget

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
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
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	rtutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
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
		if repo != "" {
			if c.toolchainType == dotnetutils.Nuget && isRestoreCommand(c.subCommand) {
				// nuget.exe (mono) re-embeds credentials into MSBuild's /p:RestoreSources=
				// regardless of whether they come from -Source or a -ConfigFile config. MSBuild
				// can authenticate V2 feeds with embedded Basic Auth but fails to load a V3
				// service index (index.json) the same way, causing NU1301. Use a temp nuget.config
				// with a V2 source URL so MSBuild sees a V2 feed URL with embedded credentials.
				//
				// Push is excluded: nuget.exe push always sends credentials as X-NuGet-ApiKey
				// regardless of the credential source; Artifactory rejects access tokens via that
				// header with 403. pushNupkgToArtifactory handles push directly via Basic Auth.
				// Pack and passthrough commands are also excluded: they are local-only and do not
				// need a NuGet source.
				cleanup, err := c.injectCredentialsViaTempConfig(repo)
				if err != nil {
					return err
				}
				defer cleanup()
			} else if c.toolchainType != dotnetutils.Nuget && isRestoreCommand(c.subCommand) {
				// dotnet CLI restore: use a temp nuget.config (same approach as nuget.exe) to
				// avoid embedding credentials in the --source process argument, which is visible
				// to all local users via /proc/<pid>/cmdline or ps aux.
				// dotnet restore supports --configfile (double-dash, POSIX style).
				cleanup, err := c.injectCredentialsViaTempConfig(repo)
				if err != nil {
					return err
				}
				defer cleanup()
			}
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

	// Both nuget.exe and dotnet CLI have push auth issues with Artifactory:
	//   nuget.exe sends X-NuGet-ApiKey which Artifactory rejects for access tokens (403).
	//   dotnet nuget push with embedded credentials in a V3 URL fails to load the service
	//   index (401) because dotnet does not forward URL-embedded auth for index.json fetches.
	// When targeting a known Artifactory repo (and the user hasn't overridden the source or
	// API key flags), bypass the native tool entirely and push directly via the NuGet gallery
	// REST endpoint using Basic Auth — which Artifactory accepts for both toolchains.
	if isPushCommand(c.subCommand) && c.serverDetails != nil && c.repoDeploy != "" && !hasNativeAuthOverride(c.args) {
		log.Info("Pushing NuGet package to Artifactory...")
		if err := c.pushPackagesToArtifactory(); err != nil {
			return fmt.Errorf("nuget push: %w", err)
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
		return c.collectAndStampPushArtifacts(buildName, buildNumber)
	case isPackCommand(c.subCommand):
		return c.collectPackArtifacts(buildName, buildNumber, packSnapshot, packOutputDir)
	}
	return nil
}

// buildCmd builds the exec.Cmd for the native nuget.exe or dotnet CLI.
// Credentials are already injected into c.args (via -Source <url> or -ConfigFile) and
// into the process environment (via NuGetPackageSourceCredentials_) before this is called.
func (c *NuGetFlexPackCommand) buildCmd() *exec.Cmd {
	if c.toolchainType == dotnetutils.DotnetCore {
		return exec.Command("dotnet", append(strings.Fields(c.subCommand), c.args...)...)
	}
	return exec.Command("nuget", append([]string{c.subCommand}, c.args...)...)
}

// injectCredentialsViaTempConfig handles credential injection for nuget.exe. It writes a
// temporary nuget.config with a V2 source URL and <packageSourceCredentials>.
//
// V2 is required because nuget.exe (mono) re-embeds credentials from any source — including
// a -ConfigFile config — into MSBuild's /p:RestoreSources property. MSBuild can load a V2
// feed with embedded Basic Auth but fails on V3 (index.json) URLs (NU1301). Using
// ClearTextPassword (not Password) because nuget.exe's encrypted Password storage is
// Windows DPAPI-only; ClearTextPassword works on all platforms including mono/macOS.
//
// For restore: only -ConfigFile is appended (no -Source). MSBuild's RestoreTask reads
// sources from the config file directly via /p:RestoreConfigFile. Adding -Source <name>
// would cause nuget.exe to pass /p:RestoreSources=<name> to MSBuild, which then
// resolves the name as a local filesystem path instead of a named source — breaking
// restore for SDK-style (.csproj) projects whenever the global packages cache is empty.
//
// For push: -Source <name> is also appended because push is handled by nuget.exe directly
// (not MSBuild), so it CAN look up named sources from the config file.
//
// The returned cleanup func removes the temp file. The caller must defer it immediately
// after a nil-error return.
func (c *NuGetFlexPackCommand) injectCredentialsViaTempConfig(repo string) (func(), error) {
	// Only called for non-push nuget.exe commands (restore, update, install).
	// Push is handled by pushPackagesToArtifactory which uses Basic Auth directly.
	//
	// restore delegates to MSBuild (for SDK-style projects), so it:
	//   - must NOT receive -Source <name> (MSBuild resolves names as filesystem paths)
	//   - reads sources directly from /p:RestoreConfigFile — V3 URL is safe here because
	//     the URL is in the config file, not re-embedded by nuget.exe into RestoreSources
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
	c.args = append(c.args, configFlag, tmpFile.Name())

	return func() { _ = os.Remove(tmpFile.Name()) }, nil
}

// pushPackagesToArtifactory resolves all .nupkg/.snupkg paths from push args (expanding
// globs), pushes each directly to Artifactory's NuGet gallery endpoint using Basic Auth,
// and replicates nuget.exe's sibling .snupkg auto-push behaviour. This bypasses the native
// push tool (nuget.exe or dotnet) to avoid authentication issues: nuget.exe sends
// X-NuGet-ApiKey which Artifactory rejects for access tokens (403), and dotnet nuget push
// cannot authenticate against a V3 index.json with embedded URL credentials (401).
func (c *NuGetFlexPackCommand) pushPackagesToArtifactory() error {
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
		// config-file: credentials are supplied via Basic Auth in the bypass; no config needed
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
		return fmt.Errorf("resolve package paths: %w", err)
	}
	if len(packages) == 0 {
		return fmt.Errorf("no .nupkg or .snupkg files found in push arguments: %v", c.args)
	}

	_, user, password, err := NuGetExeV2SourceDetails(c.serverDetails, c.repoDeploy)
	if err != nil {
		return fmt.Errorf("get credentials: %w", err)
	}

	rtURL := strings.TrimSuffix(c.serverDetails.ArtifactoryUrl, "/")
	rtBase, err := url.Parse(rtURL)
	if err != nil {
		return fmt.Errorf("parse Artifactory URL: %w", err)
	}
	nupkgPushURL, snupkgPushURL := buildPushURLs(rtBase, c.repoDeploy)
	skipDuplicate := hasSkipDuplicate(c.args)
	noSymbols := hasNoSymbols(c.args)

	httpClient := &http.Client{
		Timeout:   5 * time.Minute,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
	}

	pushURL := func(pkgPath string) *url.URL {
		if strings.HasSuffix(strings.ToLower(pkgPath), ".snupkg") {
			return snupkgPushURL
		}
		return nupkgPushURL
	}

	allowedHost := rtBase.Host
	for _, pkgPath := range packages {
		if err := pushSinglePackage(httpClient, pushURL(pkgPath), allowedHost, pkgPath, user, password, skipDuplicate); err != nil {
			return err
		}
		// Replicate nuget.exe behaviour: when pushing a .nupkg, also push the sibling
		// .snupkg if one exists alongside it (unless -NoSymbols was passed).
		if !noSymbols && strings.HasSuffix(strings.ToLower(pkgPath), ".nupkg") {
			snupkgPath := pkgPath[:len(pkgPath)-len(".nupkg")] + ".snupkg"
			if _, statErr := os.Stat(snupkgPath); statErr == nil {
				if err := pushSinglePackage(httpClient, snupkgPushURL, allowedHost, snupkgPath, user, password, skipDuplicate); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func pushSinglePackage(client *http.Client, pushURL *url.URL, allowedHost, pkgPath, user, password string, skipDuplicate bool) error {
	// Allowlist check: guard against any URL manipulation causing the request to reach
	// a host other than the configured Artifactory server.
	if pushURL.Host != allowedHost {
		return fmt.Errorf("security: push URL host %q does not match Artifactory host %q", pushURL.Host, allowedHost)
	}
	f, err := os.Open(pkgPath)
	if err != nil {
		return fmt.Errorf("open %q: %w", pkgPath, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Debug("Failed to close package file:", closeErr.Error())
		}
	}()

	// Stream the multipart body directly into the request using io.Pipe so the entire
	// package is never buffered in memory — large .nupkg files (100+ MB) would OOM otherwise.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	writeErr := make(chan error, 1)
	go func() {
		defer func() {
			if closeErr := pw.Close(); closeErr != nil {
				log.Debug("Failed to close pipe writer:", closeErr.Error())
			}
		}()
		part, err := mw.CreateFormFile("package", filepath.Base(pkgPath))
		if err != nil {
			writeErr <- fmt.Errorf("create form file: %w", err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			writeErr <- fmt.Errorf("stream package: %w", err)
			return
		}
		if err := mw.Close(); err != nil {
			writeErr <- fmt.Errorf("close multipart writer: %w", err)
			return
		}
		writeErr <- nil
	}()

	req, err := http.NewRequest(http.MethodPut, pushURL.String(), pr)
	if err != nil {
		_ = pr.CloseWithError(err)
		<-writeErr
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.SetBasicAuth(user, password)

	resp, doErr := client.Do(req)
	if werr := <-writeErr; werr != nil && doErr == nil {
		doErr = werr
	}
	if doErr != nil {
		return fmt.Errorf("push request: %w", doErr)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Debug("Failed to close push response body:", closeErr.Error())
		}
	}()
	// Cap response body read to avoid unbounded memory on large error responses.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK, http.StatusNoContent:
		log.Info(fmt.Sprintf("Package %q pushed successfully", filepath.Base(pkgPath)))
		return nil
	case http.StatusConflict:
		if skipDuplicate {
			log.Warn(fmt.Sprintf("Package %q already exists — skipping duplicate", filepath.Base(pkgPath)))
			return nil
		}
		return fmt.Errorf("package already exists (409 Conflict); use -SkipDuplicate to skip")
	default:
		return fmt.Errorf("push failed: HTTP %d — %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}

// buildPushURLs constructs the Artifactory NuGet gallery endpoints for pushing packages.
// URLs are built by copying rtBase and setting only the path fields, so the host is always
// taken from the configured Artifactory server and can never be influenced by the repo name.
// url.PathEscape ensures repo names with special characters (/, space, …) are correctly encoded.
func buildPushURLs(rtBase *url.URL, repo string) (nupkgURL, snupkgURL *url.URL) {
	basePath := strings.TrimSuffix(rtBase.Path, "/") + "/api/nuget/v2/" + repo
	baseRawPath := strings.TrimSuffix(rtBase.EscapedPath(), "/") + "/api/nuget/v2/" + url.PathEscape(repo)

	nupkg := *rtBase
	nupkg.Path = basePath + "/"
	nupkg.RawPath = baseRawPath + "/"
	nupkgURL = &nupkg

	snupkg := *rtBase
	snupkg.Path = basePath + "/symbolpackage"
	snupkg.RawPath = baseRawPath + "/symbolpackage"
	snupkgURL = &snupkg
	return
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

// collectAndStampPushArtifacts identifies the exact packages a push uploaded (from the
// explicit push arguments), stamps build properties on their exact Artifactory paths, and
// records them in local build-info. The native push has already succeeded at this point, so
// it is never re-run; a stamping failure is surfaced as an error without masking the push.
func (c *NuGetFlexPackCommand) collectAndStampPushArtifacts(buildName, buildNumber string) error {
	log.Info(fmt.Sprintf("Collecting NuGet artifact info for %s/%s", buildName, buildNumber))
	// Resolve the actual local repo so OriginalDeploymentRepo is always a local repo key.
	// When the user pushes to a virtual repo, Artifactory routes to its defaultDeploymentRepo;
	// we need that local key in build-info so downstream tools can locate the artifact.
	deployRepo, err := c.resolveLocalDeployRepo(c.repoDeploy)
	if err != nil {
		return fmt.Errorf("resolve deployment repo: %w", err)
	}
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
