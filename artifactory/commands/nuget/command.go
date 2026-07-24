package nuget

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	dotnetutils "github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/build-info-go/entities"
	buildinfoflex "github.com/jfrog/build-info-go/flexpack"
	nugetflex "github.com/jfrog/build-info-go/flexpack/nuget"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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

	log.Info(fmt.Sprintf("Running %s %s", c.toolchainType, c.subCommand))
	nativeCmd := c.buildCmd(configFilePath)
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
	case isPackOrPushCommand(c.subCommand):
		return c.collectArtifacts(buildName, buildNumber)
	}
	return nil
}

// buildCmd builds the exec.Cmd for the native nuget.exe or dotnet CLI.
func (c *NuGetFlexPackCommand) buildCmd(configFilePath string) *exec.Cmd {
	if c.toolchainType == dotnetutils.DotnetCore {
		cmdArgs := append(strings.Fields(c.subCommand), c.args...)
		if configFilePath != "" {
			cmdArgs = append(cmdArgs, "--configfile", configFilePath)
		}
		return exec.Command("dotnet", cmdArgs...)
	}
	args := append([]string{c.subCommand}, c.args...)
	if configFilePath != "" {
		args = append(args, "-ConfigFile", configFilePath)
	}
	return exec.Command("nuget", args...)
}

func (c *NuGetFlexPackCommand) collectDependencies(buildName, buildNumber string) error {
	log.Info(fmt.Sprintf("Collecting NuGet build info for %s/%s", buildName, buildNumber))
	collector, err := nugetflex.NewNuGetFlexPack(buildinfoflex.NuGetConfig{
		WorkingDirectory: c.workingDir,
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

func (c *NuGetFlexPackCommand) collectArtifacts(buildName, buildNumber string) error {
	log.Info(fmt.Sprintf("Collecting NuGet artifact info for %s/%s", buildName, buildNumber))
	artifacts, err := nugetflex.FindNupkgArtifacts(c.workingDir, c.repoDeploy)
	if err != nil {
		return fmt.Errorf("find nupkg artifacts: %w", err)
	}
	bi := &entities.BuildInfo{
		Name:   buildName,
		Number: buildNumber,
		Modules: []entities.Module{
			{
				Type:      entities.Nuget,
				Artifacts: artifacts,
			},
		},
	}
	log.Info(fmt.Sprintf("NuGet artifact info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return saveBuildInfoLocally(bi, c.buildConfiguration.GetProject())
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

// isPackOrPushCommand returns true for commands that produce or upload .nupkg files.
func isPackOrPushCommand(sub string) bool {
	return sub == "pack" || isPushCommand(sub)
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
