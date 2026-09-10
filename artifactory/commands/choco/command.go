package choco

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jfrog/build-info-go/entities"
	buildinfoflex "github.com/jfrog/build-info-go/flexpack"
	chocoflex "github.com/jfrog/build-info-go/flexpack/choco"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	rtutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildutils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	// chocoSearchRetries is the number of extra attempts made after a transient transport failure.
	chocoSearchRetries = 3
	// chocoSearchRetryIntervalMilliSecs is the fixed wait between those attempts.
	chocoSearchRetryIntervalMilliSecs = 1000
	// chocoSearchEmptyRetryDelay is the single short wait granted to an empty search result before
	// it is treated as proof that the recorded build-info paths are wrong.
	chocoSearchEmptyRetryDelay = 500 * time.Millisecond
)

// errChocoArtifactsNotFound reports that the artifacts recorded in the build-info do not exist at
// the recorded repository and path. The search patterns are derived from the same fields, so this
// invalidates the build-info itself and it must not be persisted.
var errChocoArtifactsNotFound = errors.New("the pushed Chocolatey package was not found in Artifactory, so no build-info was saved")

var chocoPlatformChecker = func() bool { return runtime.GOOS == "windows" }

var chocoNativeRunner = func(args []string) error {
	var stderr strings.Builder
	command := exec.Command("choco", args...) // #nosec G204 -- args are this same invocation's own CLI arguments, forwarded verbatim by design (this is the passthrough wrapper); no shell is invoked and no privilege boundary is crossed
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = io.MultiWriter(os.Stderr, &stderr)
	if err := command.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

var chocoSourceListRunner = func() (string, error) {
	output, err := exec.Command("choco", "source", "list", "-r").Output()
	return string(output), err
}

type ChocoFlexPackCommand struct {
	subCommand         string
	args               []string
	serverDetails      *config.ServerDetails
	repoResolve        string
	repoDeploy         string
	buildConfiguration *buildutils.BuildConfiguration
	workingDirectory   string
	resolvedDeployRepo string
}

func NewChocoFlexPackCommand() *ChocoFlexPackCommand {
	return &ChocoFlexPackCommand{}
}

func (command *ChocoFlexPackCommand) SetSubCommand(value string) *ChocoFlexPackCommand {
	command.subCommand = strings.ToLower(value)
	return command
}

func (command *ChocoFlexPackCommand) SetArgs(value []string) *ChocoFlexPackCommand {
	command.args = append([]string(nil), value...)
	return command
}

func (command *ChocoFlexPackCommand) SetServerDetails(value *config.ServerDetails) *ChocoFlexPackCommand {
	command.serverDetails = value
	return command
}

func (command *ChocoFlexPackCommand) SetRepoResolve(value string) *ChocoFlexPackCommand {
	command.repoResolve = value
	return command
}

func (command *ChocoFlexPackCommand) SetRepoDeploy(value string) *ChocoFlexPackCommand {
	command.repoDeploy = value
	return command
}

func (command *ChocoFlexPackCommand) SetBuildConfiguration(value *buildutils.BuildConfiguration) *ChocoFlexPackCommand {
	command.buildConfiguration = value
	return command
}

func (command *ChocoFlexPackCommand) SetWorkingDirectory(value string) *ChocoFlexPackCommand {
	command.workingDirectory = value
	return command
}

func (command *ChocoFlexPackCommand) CommandName() string {
	return "rt_choco_flexpack"
}

func (command *ChocoFlexPackCommand) ServerDetails() (*config.ServerDetails, error) {
	return command.serverDetails, nil
}

func (command *ChocoFlexPackCommand) Run() error {
	if !chocoPlatformChecker() {
		return fmt.Errorf("'jf choco' requires Chocolatey, which runs on Windows only. Detected OS: %s", runtime.GOOS)
	}
	// --build-name and --build-number are only meaningful as a pair. Reject a half-specified pair
	// before the native command runs, so a flag mistake costs nothing and the user is never left
	// believing build-info was collected when it silently was not.
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

	var snapshot chocoflex.PackageSnapshot
	if command.subCommand == "pack" {
		var err error
		// `choco pack` writes to the current directory unless --output-directory (or one of its
		// aliases) names another one, so that directory has to be snapshotted too. Missing it makes
		// the diff find nothing and the build-info come back empty, with no error to explain why.
		snapshot, err = chocoflex.SnapshotPackageFiles(command.workingDirectory, command.packOutputDirectory()...)
		if err != nil {
			return err
		}
		warnUnsubstitutedNuspecTokens(command.workingDirectory, command.args)
	}

	nativeArgs := append([]string{command.subCommand}, command.args...)
	if command.subCommand == "push" && command.repoDeploy != "" && command.serverDetails != nil {
		if err := validateExplicitPushSource(command.args, command.repoDeploy, command.serverDetails); err != nil {
			return err
		}
		resolvedDeployRepo, err := command.resolveLocalDeployRepo(command.repoDeploy)
		if err != nil {
			return err
		}
		command.resolvedDeployRepo = resolvedDeployRepo
	}
	if command.subCommand == "push" && command.serverDetails != nil && command.repoDeploy != "" && !hasNativeAuthOverride(command.args) {
		sourceURL, user, password, err := dotnet.GetSourceDetails(command.serverDetails, command.repoDeploy, true)
		if err != nil {
			return fmt.Errorf("get Chocolatey source details: %w", err)
		}
		if user == "" || password == "" {
			return errors.New("pushing to Chocolatey requires configured JFrog credentials")
		}
		nativeArgs = append(nativeArgs, "-s="+sourceURL, "-k="+user+":"+password)
		log.Debug("Injected JFrog Artifactory credentials into the Chocolatey push source " + sourceURL)
	}
	if (command.subCommand == "install" || command.subCommand == "upgrade") &&
		command.serverDetails != nil && command.repoResolve != "" && !hasNativeResolveOverride(command.args) {
		// --repo-resolve is an on-the-fly resolution override: point the native command at that
		// repository on the configured server, rather than merely labelling the build-info with it.
		sourceURL, user, password, err := dotnet.GetSourceDetails(command.serverDetails, command.repoResolve, true)
		if err != nil {
			return fmt.Errorf("get Chocolatey source details: %w", err)
		}
		nativeArgs = append(nativeArgs, "-s="+sourceURL)
		if user != "" && password != "" {
			nativeArgs = append(nativeArgs, "-u="+user, "-p="+password)
		}
		// Anonymous resolution is legitimate when the repository allows it, so missing credentials
		// are not an error here - only a note, since a 401 later would otherwise look unexplained.
		if user == "" || password == "" {
			log.Debug("No JFrog credentials configured; resolving from " + sourceURL + " anonymously.")
		}
		log.Debug("Resolving Chocolatey packages from the JFrog Artifactory source " + sourceURL)
	}

	log.Debug("Running native Chocolatey command: choco " + strings.Join(append([]string{command.subCommand}, redactChocoArgs(command.args)...), " "))
	if err := chocoNativeRunner(nativeArgs); err != nil {
		return fmt.Errorf("choco %s failed: %w", command.subCommand, err)
	}
	// Build-info is collected only when both --build-name and --build-number are supplied. Neither
	// flag means a plain passthrough, which is a legitimate way to use 'jf choco'; the half-specified
	// case was already rejected above, before the native command ran.
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

	switch command.subCommand {
	case "pack":
		return command.collectPackArtifacts(buildName, buildNumber, snapshot)
	case "push":
		return command.collectPushArtifacts(buildName, buildNumber)
	case "install", "upgrade":
		return command.collectDependencies(buildName, buildNumber)
	}
	return nil
}

func (command *ChocoFlexPackCommand) collectPackArtifacts(buildName, buildNumber string, snapshot chocoflex.PackageSnapshot) error {
	artifacts, err := chocoflex.CollectPackedArtifacts(command.workingDirectory, snapshot, command.repoDeploy,
		command.packOutputDirectory()...)
	if err != nil {
		return fmt.Errorf("collect packed Chocolatey artifacts: %w", err)
	}
	return command.saveArtifactBuildInfo(buildName, buildNumber, artifacts)
}

// packOutputDirectory returns the directory named by `choco pack --output-directory` (or one of its
// aliases) as a variadic-friendly slice, resolved against the working directory. Empty when the
// command did not pass the flag, in which case pack writes to the working directory itself.
func (command *ChocoFlexPackCommand) packOutputDirectory() []string {
	outputDirectory := chocoflex.ExtractPackOutputDirectory(command.args)
	if outputDirectory == "" {
		return nil
	}
	if !filepath.IsAbs(outputDirectory) {
		outputDirectory = filepath.Join(command.workingDirectory, outputDirectory)
	}
	return []string{outputDirectory}
}

func (command *ChocoFlexPackCommand) collectPushArtifacts(buildName, buildNumber string) error {
	deployRepo := command.resolvedDeployRepo
	if deployRepo == "" {
		var err error
		deployRepo, err = command.resolveLocalDeployRepo(command.repoDeploy)
		if err != nil {
			return err
		}
	}
	artifacts, err := chocoflex.CollectPushArtifacts(command.workingDirectory, command.args, deployRepo)
	if err != nil {
		return fmt.Errorf("collect pushed Chocolatey artifacts: %w", err)
	}
	return command.persistBuildInfoAfterStamping(command.stampBuildProperties(artifacts, buildName, buildNumber), buildName, buildNumber, artifacts)
}

// persistBuildInfoAfterStamping decides whether the collected build-info survives a stamping
// failure. It always returns a non-nil error when stampErr is non-nil: artifacts that carry no
// build.name/build.number are invisible to build promotion and search-by-build, and failing
// silently here makes that very hard to trace back.
func (command *ChocoFlexPackCommand) persistBuildInfoAfterStamping(stampErr error, buildName, buildNumber string, artifacts []entities.Artifact) error {
	// An empty search result means the repository and path recorded in the build-info do not exist
	// in Artifactory. The build-info is wrong, so persisting it would only bake the bad paths into
	// a later promotion.
	if errors.Is(stampErr, errChocoArtifactsNotFound) {
		return stampErr
	}
	// Every other outcome reached Artifactory and confirmed the recorded paths, so the build-info
	// is sound and worth keeping.
	if saveErr := command.saveArtifactBuildInfo(buildName, buildNumber, artifacts); saveErr != nil {
		return errors.Join(stampErr, saveErr)
	}
	if stampErr != nil {
		log.Info("Chocolatey build info was saved locally, but the build properties were not stamped on the artifacts.")
	}
	return stampErr
}

func (command *ChocoFlexPackCommand) collectDependencies(buildName, buildNumber string) error {
	rootModule := command.buildConfiguration.GetModule()
	if rootModule == "" {
		rootModule = filepath.Base(command.workingDirectory)
	}
	repoResolve := command.repoResolve
	if repoResolve == "" {
		repoResolve = repoFromSourceArg(command.args, command.serverDetails)
		if repoResolve == "" {
			repoResolve = repoFromConfiguredSources(command.serverDetails)
		}
	}
	collector, err := chocoflex.NewChocoFlexPack(buildinfoflex.ChocoConfig{
		WorkingDirectory: command.workingDirectory,
		Packages:         requestedPackages(command.args),
		RepoResolve:      repoResolve,
		Module:           rootModule,
	}, nil)
	if err != nil {
		return err
	}
	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("collect Chocolatey dependencies: %w", err)
	}
	setChocoCommandProperty(buildInfo.Modules, command.subCommand, command.args)
	return saveBuildInfoLocally(buildInfo, command.buildConfiguration.GetProject())
}

func (command *ChocoFlexPackCommand) resolveLocalDeployRepo(repoName string) (string, error) {
	if repoName == "" || command.serverDetails == nil {
		return repoName, nil
	}
	servicesManager, err := rtutils.CreateServiceManager(command.serverDetails, -1, 0, false)
	if err != nil {
		return "", fmt.Errorf("create services manager to resolve repo %q: %w", repoName, err)
	}
	var repoDetails services.RepositoryDetails
	if err = servicesManager.GetRepository(repoName, &repoDetails); err != nil {
		return "", fmt.Errorf("resolve repo type for %q: %w", repoName, err)
	}
	switch repoDetails.GetRepoType() {
	case services.RemoteRepositoryRepoType:
		return "", fmt.Errorf("remote repository %q cannot be used as a Chocolatey push target", repoName)
	case services.VirtualRepositoryRepoType:
		var virtualDetails services.VirtualRepositoryBaseParams
		if err = servicesManager.GetRepository(repoName, &virtualDetails); err != nil {
			return "", fmt.Errorf("resolve virtual repo %q: %w", repoName, err)
		}
		if virtualDetails.DefaultDeploymentRepo == "" {
			return "", fmt.Errorf("virtual repo %q has no defaultDeploymentRepo configured; cannot determine the local repo for build-info", repoName)
		}
		log.Debug(fmt.Sprintf("Virtual repo %q deploys to local repo %q; build-info artifacts will reference the local repo.", repoName, virtualDetails.DefaultDeploymentRepo))
		return virtualDetails.DefaultDeploymentRepo, nil
	default:
		return repoName, nil
	}
}

func hasNativeAuthOverride(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		for _, prefix := range []string{"--source", "-s", "--api-key", "--apikey", "-k"} {
			if lower == prefix || strings.HasPrefix(lower, prefix+"=") {
				return true
			}
		}
	}
	return false
}

// hasNativeResolveOverride reports whether the user already pointed the native command at a source
// or supplied source credentials. Chocolatey authenticates a resolution source with --user and
// --password (--api-key is push-only), so those are the flags that matter here. When any of them is
// present the native arguments win and nothing is injected: a second -s would give Chocolatey two
// sources, which it resolves in an order the user did not ask for.
func hasNativeResolveOverride(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		for _, prefix := range []string{"--source", "-s", "--user", "-u", "--password", "-p"} {
			if lower == prefix || strings.HasPrefix(lower, prefix+"=") {
				return true
			}
		}
	}
	return false
}

func validateExplicitPushSource(args []string, repoDeploy string, serverDetails *config.ServerDetails) error {
	sourceRepo := repoFromSourceArg(args, serverDetails)
	if sourceRepo != "" && sourceRepo != repoDeploy {
		return fmt.Errorf("the Chocolatey source targets repository %q but --repo is %q; use matching source and --repo values", sourceRepo, repoDeploy)
	}
	return nil
}

func repoFromSourceArg(args []string, serverDetails *config.ServerDetails) string {
	for index, arg := range args {
		lower := strings.ToLower(arg)
		value := ""
		switch {
		case strings.HasPrefix(lower, "--source="):
			value = arg[len("--source="):]
		case strings.HasPrefix(lower, "-s="):
			value = arg[len("-s="):]
		case (lower == "--source" || lower == "-s") && index+1 < len(args):
			value = args[index+1]
		}
		if repo := repoFromSource(value, serverDetails); repo != "" {
			return repo
		}
	}
	return ""
}

func repoFromSource(source string, serverDetails *config.ServerDetails) string {
	if source == "" {
		return ""
	}
	if serverDetails != nil {
		serverURL, err := url.Parse(serverDetails.ArtifactoryUrl)
		if err == nil && serverURL.Hostname() != "" {
			prefix := "jfrt-" + sanitizeSourceComponent(serverURL.Hostname()) + "-"
			if strings.HasPrefix(strings.ToLower(source), prefix) {
				return source[len(prefix):]
			}
		}
	}
	parsedURL, err := url.Parse(source)
	if err != nil {
		return ""
	}
	const nugetPrefix = "/api/nuget/"
	index := strings.Index(parsedURL.Path, nugetPrefix)
	if index == -1 {
		return ""
	}
	repo := strings.Trim(strings.TrimPrefix(parsedURL.Path[index:], nugetPrefix), "/")
	repo = strings.TrimPrefix(repo, "v3/")
	return strings.Split(repo, "/")[0]
}

func sanitizeSourceComponent(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func repoFromConfiguredSources(serverDetails *config.ServerDetails) string {
	if serverDetails == nil {
		return ""
	}
	listing, err := chocoSourceListRunner()
	if err != nil {
		log.Warn("Could not read Chocolatey sources; dependency repository will be omitted.")
		return ""
	}
	type candidate struct {
		repo     string
		priority int
	}
	candidates := make([]candidate, 0)
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		fields := strings.Split(strings.TrimSpace(line), "|")
		if len(fields) < 2 {
			continue
		}
		if containsDisabled(fields) || !sourceMatchesServer(fields[1], serverDetails) {
			continue
		}
		repo := repoFromSource(fields[1], serverDetails)
		if repo == "" {
			continue
		}
		priority := sourcePriority(fields)
		if priority == 0 {
			priority = int(^uint(0) >> 1)
		}
		candidates = append(candidates, candidate{repo: repo, priority: priority})
	}
	if len(candidates) == 0 {
		return ""
	}
	best := candidates[0]
	ambiguous := false
	for _, candidate := range candidates[1:] {
		if candidate.priority < best.priority {
			best = candidate
			ambiguous = false
		} else if candidate.priority == best.priority && candidate.repo != best.repo {
			ambiguous = true
		}
	}
	if ambiguous {
		log.Warn("Multiple Chocolatey Artifactory sources have the same priority; dependency repository will be omitted.")
		return ""
	}
	return best.repo
}

func containsDisabled(fields []string) bool {
	if len(fields) > 2 && strings.EqualFold(strings.TrimSpace(fields[2]), "true") {
		return true
	}
	for _, field := range fields[2:] {
		if strings.EqualFold(strings.TrimSpace(field), "disabled") {
			return true
		}
	}
	return false
}

func sourcePriority(fields []string) int {
	for _, field := range fields[2:] {
		if priority, err := strconv.Atoi(strings.TrimSpace(field)); err == nil {
			if priority > 0 {
				return priority
			}
			return int(^uint(0) >> 1)
		}
	}
	return int(^uint(0) >> 1)
}

func sourceMatchesServer(source string, serverDetails *config.ServerDetails) bool {
	sourceURL, sourceErr := url.Parse(source)
	serverURL, serverErr := url.Parse(serverDetails.ArtifactoryUrl)
	return sourceErr == nil && serverErr == nil && sourceURL.Hostname() != "" && strings.EqualFold(sourceURL.Hostname(), serverURL.Hostname())
}

func (command *ChocoFlexPackCommand) stampBuildProperties(artifacts []entities.Artifact, buildName, buildNumber string) error {
	if command.serverDetails == nil || command.repoDeploy == "" || len(artifacts) == 0 {
		return nil
	}
	patterns := artifactPatterns(artifacts)
	if len(patterns) == 0 {
		return nil
	}
	servicesManager, err := rtutils.CreateServiceManager(command.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("create services manager for Chocolatey property stamping: %w", err)
	}
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, strconv.FormatInt(time.Now().UnixMilli(), 10))
	props = civcs.MergeWithUserProps(props, command.workingDirectory)
	specFiles := &spec.SpecFiles{}
	for _, pattern := range patterns {
		specFiles.Files = append(specFiles.Files, spec.File{Pattern: pattern})
	}
	var reader *content.ContentReader
	_, err = searchWithRetry(chocoSearchRetries, chocoSearchRetryIntervalMilliSecs, chocoSearchEmptyRetryDelay, patterns, func() (int, error) {
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
	if err != nil {
		// The empty-result error already explains that the recorded build-info paths are wrong;
		// wrapping it in a "resolve ... for property stamping" prefix would hide that.
		if errors.Is(err, errChocoArtifactsNotFound) {
			return err
		}
		return fmt.Errorf("resolve uploaded Chocolatey artifacts for property stamping: %w", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("Failed to close Chocolatey property search reader:", closeErr.Error())
		}
	}()
	if err = setPropsWithRetry(chocoSearchRetries, chocoSearchRetryIntervalMilliSecs, func() error {
		_, setErr := servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props})
		if setErr != nil {
			// SetProps consumed the reader; rewind it so a retry sends the same item list.
			reader.Reset()
		}
		return setErr
	}); err != nil {
		if isForbiddenError(err) {
			return fmt.Errorf("stamp build properties on uploaded Chocolatey artifacts: %w"+
				"\nhint: annotate permission is required to set properties, and it is granted separately from deploy."+
				" Verify the permission target for repository %q grants annotate to this user", err, command.repoDeploy)
		}
		return fmt.Errorf("stamp build properties on uploaded Chocolatey artifacts: %w", err)
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

// searchWithRetry locates the artifacts that were just pushed, distinguishing failures that a
// retry can fix from failures that it cannot.
//
// Transient failures (5xx, 429, 403 under rate limiting, connection resets, timeouts) are retried
// at a short fixed interval. An empty result is different in kind: the search patterns are built
// from the very same repository and path that were recorded in the build-info, so finding nothing
// means those recorded values are wrong rather than late. It gets one quick re-attempt to absorb a
// momentary visibility gap, then fails with errChocoArtifactsNotFound. Any other 4xx fails fast.
func searchWithRetry(retries, retryIntervalMilliSecs int, emptyResultRetryDelay time.Duration, patterns []string, searchFn func() (int, error)) (int, error) {
	count, err := searchOnce(retries, retryIntervalMilliSecs, searchFn)
	if err != nil || count > 0 {
		return count, err
	}
	log.Debug(fmt.Sprintf("No uploaded Chocolatey artifacts found at %s; re-checking once in %s.",
		strings.Join(patterns, ", "), emptyResultRetryDelay))
	if emptyResultRetryDelay > 0 {
		time.Sleep(emptyResultRetryDelay)
	}
	if count, err = searchOnce(retries, retryIntervalMilliSecs, searchFn); err != nil || count > 0 {
		return count, err
	}
	// An empty search result has two causes that Artifactory cannot distinguish for us: a wrong
	// path, and a right path this user cannot read (permission denials come back as an empty
	// result set, not as a 403). Name both, and say what to look at to tell them apart.
	return 0, fmt.Errorf("%w"+
		"\nsearched: %s (no items returned, re-checked once)"+
		"\nthe search cannot tell these two causes apart:"+
		"\n  1. the package is not at that path - the repository or path recorded in the build-info is wrong"+
		"\n  2. the package is there, but this user has no read permission on the repository - Artifactory returns an empty result, not an error"+
		"\nopen the repository in Artifactory: if the package is visible, grant read and annotate; if it is not, re-run the push against the intended repository",
		errChocoArtifactsNotFound, strings.Join(patterns, ", "))
}

// searchOnce runs searchFn once, retrying only transient transport failures.
func searchOnce(retries, retryIntervalMilliSecs int, searchFn func() (int, error)) (count int, err error) {
	executor := clientutils.RetryExecutor{
		MaxRetries:               retries,
		RetriesIntervalMilliSecs: retryIntervalMilliSecs,
		ErrorMessage:             "Failed to search for the uploaded Chocolatey artifacts",
		LogMsgPrefix:             "[Chocolatey build-info] ",
		ExecutionHandler: func() (bool, error) {
			count, err = searchFn()
			return err != nil && isRetryableError(err), err
		},
	}
	if executeErr := executor.Execute(); executeErr != nil {
		return 0, executeErr
	}
	if count > 0 {
		log.Debug(fmt.Sprintf("Found %d uploaded Chocolatey artifact(s) to stamp.", count))
	}
	return count, nil
}

// setPropsWithRetry stamps the properties, retrying only transient transport failures.
func setPropsWithRetry(retries, retryIntervalMilliSecs int, setPropsFn func() error) error {
	executor := clientutils.RetryExecutor{
		MaxRetries:               retries,
		RetriesIntervalMilliSecs: retryIntervalMilliSecs,
		ErrorMessage:             "Failed to stamp build properties on the uploaded Chocolatey artifacts",
		LogMsgPrefix:             "[Chocolatey build-info] ",
		ExecutionHandler: func() (bool, error) {
			err := setPropsFn()
			return err != nil && isRetryableError(err), err
		},
	}
	return executor.Execute()
}

// isRetryableError reports whether err is worth another attempt: a server-side or throttling
// status, or a transport-level failure. A 403 is included because Artifactory gateways and WAFs
// answer rate limiting with 403 as often as with 429; a genuine missing-annotate-permission 403
// survives the retries and is then reported with that hint.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *errorutils.HttpResponseError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= http.StatusInternalServerError ||
			httpErr.StatusCode == http.StatusTooManyRequests ||
			httpErr.StatusCode == http.StatusForbidden
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

// isForbiddenError reports whether err carries a 403 status.
func isForbiddenError(err error) bool {
	var httpErr *errorutils.HttpResponseError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusForbidden
}

func (command *ChocoFlexPackCommand) saveArtifactBuildInfo(buildName, buildNumber string, artifacts []entities.Artifact) error {
	buildInfo := &entities.BuildInfo{
		Name:    buildName,
		Number:  buildNumber,
		Modules: chocoflex.BuildArtifactModules(artifacts, command.buildConfiguration.GetModule()),
	}
	setChocoCommandProperty(buildInfo.Modules, command.subCommand, command.args)
	log.Debug(fmt.Sprintf("Collected %d Chocolatey artifact(s) into %d module(s) for build %s/%s.", len(artifacts), len(buildInfo.Modules), buildName, buildNumber))
	return saveBuildInfoLocally(buildInfo, command.buildConfiguration.GetProject())
}

func setChocoCommandProperty(modules []entities.Module, subCommand string, args []string) {
	command := append([]string{subCommand}, redactChocoArgs(args)...)
	for index := range modules {
		modules[index].Properties = map[string]string{
			entities.BuildInfoEnvPrefix + "CHOCO_COMMAND": strings.Join(command, " "),
		}
	}
}

func redactChocoArgs(args []string) []string {
	redacted := append([]string(nil), args...)
	for index, arg := range redacted {
		lower := strings.ToLower(arg)
		// Every Chocolatey option that carries a secret: the push API key, the source password
		// (--user/--password is how a resolution source is authenticated), the proxy password and
		// the client-certificate password. These reach the debug log and the CHOCO_COMMAND
		// build-info property, so a missed one is published with the build.
		for _, flag := range []string{"--api-key", "--apikey", "-k", "--password", "-p",
			"--proxy-password", "--certpassword", "--cp"} {
			if lower == flag && index+1 < len(redacted) {
				redacted[index+1] = "***"
			}
			if strings.HasPrefix(lower, flag+"=") {
				redacted[index] = arg[:len(flag)+1] + "***"
			}
		}
	}
	return redacted
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
	log.Info(fmt.Sprintf("Chocolatey build info collected. Use 'jf rt bp %s %s' to publish it.", buildInfo.Name, buildInfo.Number))
	return nil
}

// nuspecReplacementToken matches a NuGet replacement token such as $version$ or $id$.
var nuspecReplacementToken = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*\$`)

// warnUnsubstitutedNuspecTokens warns when a manifest about to be packed still contains NuGet
// replacement tokens. Those are substituted from a project file, which 'choco pack' does not read:
// it packs the literal "$version$" and neither fails nor warns (chocolatey/choco#1354). The result is
// a package carrying a nonsense version, and build-info whose artifact module ID records that same
// nonsense version. A warning here is the only place the user is told.
//
// Any problem reading a manifest is ignored: this is advisory, and 'choco pack' is itself about to
// report a manifest it cannot read.
func warnUnsubstitutedNuspecTokens(workingDirectory string, args []string) {
	for _, nuspecPath := range packNuspecPaths(workingDirectory, args) {
		content, err := os.ReadFile(nuspecPath) // #nosec G304 -- nuspecPath comes from packNuspecPaths: either a manifest the user named on this same command line or one found in the working directory 'choco pack' is about to read anyway; the read is advisory and its errors are ignored
		if err != nil {
			continue
		}
		if tokens := nuspecReplacementToken.FindAllString(string(content), -1); len(tokens) > 0 {
			log.Warn("The Chocolatey manifest " + nuspecPath + " contains the unsubstituted replacement" +
				" token(s) " + strings.Join(uniqueStrings(tokens), ", ") + ". 'choco pack' does not" +
				" substitute these - it packs them literally - so the package metadata, and the" +
				" build-info collected from it, will contain the token text instead of a version." +
				" Replace them with literal values, or pass them to 'choco pack' as properties" +
				" (for example 'choco pack version=1.0.0').")
		}
	}
}

// packNuspecPaths returns the manifests 'choco pack' will read: the .nuspec named on the command line
// if there is one, otherwise every .nuspec in the working directory, which is what choco packs when
// given no path.
func packNuspecPaths(workingDirectory string, args []string) []string {
	for _, arg := range requestedPackages(args) {
		if !strings.EqualFold(filepath.Ext(arg), ".nuspec") {
			continue
		}
		if filepath.IsAbs(arg) {
			return []string{arg}
		}
		return []string{filepath.Join(workingDirectory, arg)}
	}
	entries, err := os.ReadDir(workingDirectory)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".nuspec") {
			paths = append(paths, filepath.Join(workingDirectory, entry.Name()))
		}
	}
	return paths
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func requestedPackages(args []string) []string {
	packages := make([]string, 0)
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if !strings.Contains(arg, "=") && chocoOptionTakesValue(arg) {
				skipNext = true
			}
			continue
		}
		packages = append(packages, arg)
	}
	return packages
}

// "-v" is deliberately absent: it is Chocolatey's global --verbose boolean switch, not a short form
// of --version. ChocolateyInstallCommand registers the version option as "version=" with no "v|"
// alias. Treating "-v" as value-taking made "choco install -v pkgname" swallow "pkgname", so the
// build-info recorded no dependencies at all.
func chocoOptionTakesValue(option string) bool {
	switch strings.ToLower(option) {
	case "-s", "--source", "--version", "--package-parameters", "--install-arguments",
		"--execution-timeout", "--cache-location", "--proxy", "--proxy-user", "--proxy-password",
		"--cert", "--certpassword":
		return true
	default:
		return false
	}
}
