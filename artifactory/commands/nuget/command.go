package nuget

import (
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
	dotnetcmd "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
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

// RequiresServerDetails reports whether the command needs JFrog server configuration
// to create a repository-specific NuGet configuration or stamp pushed artifacts.
func (c *NuGetFlexPackCommand) RequiresServerDetails() bool {
	if isPushCommand(c.subCommand) {
		return c.repoDeploy != ""
	}
	return isRestoreCommand(c.subCommand) && c.repoResolve != ""
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

	// Write temp nuget.config for commands that need a source (restore/install/update/build/push).
	// pack does not need a source config.
	var configFilePath string
	if c.serverDetails != nil && needsConfig(c.subCommand) {
		repo := c.repoResolve
		if isPushCommand(c.subCommand) {
			repo = c.repoDeploy
		}
		if repo != "" {
			tmpConfig, cleanupFn, err := WriteTempNuGetConfig(c.serverDetails, repo, c.useNugetV2, c.allowInsecureConnections)
			if err != nil {
				return err
			}
			defer cleanupFn()
			configFilePath = tmpConfig
		}
	}

	// For pack, snapshot existing package files before running so we can deterministically
	// identify the packages this command produces (including custom --output directories and
	// bin/<Configuration> defaults), instead of scanning the working directory for stale files.
	var packSnapshot nugetflex.PackageSnapshot
	if isPackCommand(c.subCommand) {
		var snapErr error
		packSnapshot, snapErr = nugetflex.SnapshotPackageFiles(c.workingDir)
		if snapErr != nil {
			return snapErr
		}
	}

	log.Info(fmt.Sprintf("Running %s %s", c.toolchainType, c.subCommand))
	nativeCmd := c.buildCmd(configFilePath)
	nativeCmd.Stdin = os.Stdin
	nativeCmd.Stdout = os.Stdout
	nativeCmd.Stderr = os.Stderr
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
		return c.collectPackArtifacts(buildName, buildNumber, packSnapshot)
	}
	return nil
}

// buildCmd builds the exec.Cmd for the native nuget.exe or dotnet CLI.
func (c *NuGetFlexPackCommand) buildCmd(configFilePath string) *exec.Cmd {
	if c.toolchainType == dotnetutils.DotnetCore {
		cmdArgs := append(strings.Fields(c.subCommand), c.args...)
		if configFilePath != "" {
			cmdArgs = append(cmdArgs, "--configfile", configFilePath)
			if isPushCommand(c.subCommand) && !hasSourceFlag(c.args) {
				// 'dotnet nuget push' requires an explicit --source even when the config
				// file defines exactly one source; it never falls back to it implicitly.
				cmdArgs = append(cmdArgs, "--source", dotnetcmd.SourceName)
			}
		}
		return exec.Command("dotnet", cmdArgs...)
	}
	args := append([]string{c.subCommand}, c.args...)
	if configFilePath != "" {
		args = append(args, "-ConfigFile", configFilePath)
		if isPushCommand(c.subCommand) && !hasSourceFlag(c.args) {
			args = append(args, "-Source", dotnetcmd.SourceName)
		}
	}
	return exec.Command("nuget", args...)
}

// hasSourceFlag reports whether the user already passed an explicit source flag
// (-s/--source/-source for dotnet, -Source for nuget.exe), inline or as a separate value.
func hasSourceFlag(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "-s" || lower == "--source" || lower == "-source" ||
			strings.HasPrefix(lower, "--source=") || strings.HasPrefix(lower, "-source=") {
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
	artifacts, err := nugetflex.CollectPushArtifacts(c.workingDir, c.args, c.repoDeploy)
	if err != nil {
		return fmt.Errorf("collect pushed NuGet artifacts: %w", err)
	}
	if err := c.stampBuildProperties(artifacts, buildName, buildNumber); err != nil {
		return err
	}
	return c.saveArtifactsBuildInfo(buildName, buildNumber, artifacts)
}

// collectPackArtifacts records the packages produced by a pack command, detected by comparing
// the pre-command package snapshot with the current filesystem state.
func (c *NuGetFlexPackCommand) collectPackArtifacts(buildName, buildNumber string, before nugetflex.PackageSnapshot) error {
	log.Info(fmt.Sprintf("Collecting NuGet artifact info for %s/%s", buildName, buildNumber))
	artifacts, err := nugetflex.CollectPackedArtifacts(c.workingDir, before, c.repoDeploy)
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
// package at its exact, deterministic Artifactory path (<repo>/<file>, flat at the repository
// root - Artifactory's NuGet push API never nests packages under <id>/<version>/). It uses
// fully-qualified path patterns so no repository-wide scan is performed. Symbol packages
// (.snupkg) are stamped at their own exact paths alongside the primary packages.
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
	reader, err := generic.SearchItems(specFiles, servicesManager)
	if err != nil {
		return fmt.Errorf("resolve uploaded NuGet artifacts for property stamping: %w", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("Failed to close search reader:", closeErr.Error())
		}
	}()
	length, _ := reader.Length()
	if length == 0 {
		return fmt.Errorf("no uploaded NuGet artifacts found at the expected paths for property stamping: %s", strings.Join(patterns, ", "))
	}
	if _, err := servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props}); err != nil {
		return fmt.Errorf("stamp build properties on uploaded NuGet artifacts: %w", err)
	}
	log.Info(fmt.Sprintf("Stamped build properties on %d NuGet artifact(s).", length))
	return nil
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
		"--configfile", "-configfile",
		"--lock-file-path",
		"-msbuildpath", "-msbuildversion",
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

// needsConfig returns true when the subcommand needs a NuGet source (restore, push, etc.).
// pack is a local-only operation and does not use a NuGet source.
func needsConfig(sub string) bool {
	return isRestoreCommand(sub) || isPushCommand(sub)
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
