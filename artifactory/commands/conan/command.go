package conan

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	gofrogcmd "github.com/jfrog/gofrog/io"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ConanCommand represents a Conan CLI command with build info support.
type ConanCommand struct {
	commandName        string
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
	workingDir         string
}

// NewConanCommand creates a new ConanCommand instance.
func NewConanCommand() *ConanCommand {
	return &ConanCommand{}
}

// SetCommandName sets the Conan subcommand name (install, create, upload, etc.).
func (c *ConanCommand) SetCommandName(name string) *ConanCommand {
	c.commandName = name
	return c
}

// SetArgs sets the command arguments.
func (c *ConanCommand) SetArgs(args []string) *ConanCommand {
	c.args = args
	return c
}

// SetServerDetails sets the Artifactory server configuration.
func (c *ConanCommand) SetServerDetails(details *config.ServerDetails) *ConanCommand {
	c.serverDetails = details
	return c
}

// SetBuildConfiguration sets the build configuration for build info collection.
func (c *ConanCommand) SetBuildConfiguration(config *buildUtils.BuildConfiguration) *ConanCommand {
	c.buildConfiguration = config
	return c
}

// Run executes the Conan command with build info collection.
func (c *ConanCommand) Run() error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	c.workingDir = workingDir

	// Handle upload command with auto-login
	if c.commandName == "upload" {
		return c.runUpload()
	}

	// Run standard Conan command
	return c.runStandardCommand()
}

// runUpload handles the upload command with auto-login and build info collection.
func (c *ConanCommand) runUpload() error {
	// Extract remote name and perform auto-login
	remoteName := ExtractRemoteName(c.args)
	if remoteName != "" {
		matchedServer, err := ValidateAndLogin(remoteName)
		if err != nil {
			return err
		}
		c.serverDetails = matchedServer
	}

	log.Info(fmt.Sprintf("Running Conan %s.", c.commandName))

	// Execute conan upload and capture output
	output, err := c.executeAndCaptureOutput()
	if err != nil {
		fmt.Print(string(output))
		return fmt.Errorf("conan %s failed: %w", c.commandName, err)
	}
	fmt.Print(string(output))

	// Process upload for build info
	if c.buildConfiguration != nil {
		return c.processUploadBuildInfo(string(output))
	}

	return nil
}

// runStandardCommand runs non-upload Conan commands.
func (c *ConanCommand) runStandardCommand() error {
	log.Info(fmt.Sprintf("Running Conan %s.", c.commandName))

	if err := gofrogcmd.RunCmd(c); err != nil {
		return fmt.Errorf("conan %s failed: %w", c.commandName, err)
	}

	// Collect build info for dependency commands
	if c.buildConfiguration != nil {
		return c.collectDependencyBuildInfo()
	}

	return nil
}

// processUploadBuildInfo collects build info after a successful upload.
func (c *ConanCommand) processUploadBuildInfo(uploadOutput string) error {
	buildName, err := c.buildConfiguration.GetBuildName()
	if err != nil || buildName == "" {
		return nil
	}

	buildNumber, err := c.buildConfiguration.GetBuildNumber()
	if err != nil || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Processing Conan upload with build info: %s/%s", buildName, buildNumber))

	processor := NewUploadProcessor(c.workingDir, c.buildConfiguration, c.serverDetails)
	if err := processor.Process(uploadOutput); err != nil {
		log.Warn("Failed to process Conan upload: " + err.Error())
	}

	log.Info(fmt.Sprintf("Conan build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

// collectDependencyBuildInfo collects dependencies using FlexPack.
func (c *ConanCommand) collectDependencyBuildInfo() error {
	buildName, err := c.buildConfiguration.GetBuildName()
	if err != nil || buildName == "" {
		return nil
	}

	buildNumber, err := c.buildConfiguration.GetBuildNumber()
	if err != nil || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Collecting build info for Conan project: %s/%s", buildName, buildNumber))

	collector := NewDependencyCollector(c.workingDir, c.buildConfiguration)
	if err := collector.Collect(); err != nil {
		log.Warn("Failed to collect Conan build info: " + err.Error())
		return nil
	}

	log.Info(fmt.Sprintf("Conan build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

// executeAndCaptureOutput runs the command and returns the combined output.
func (c *ConanCommand) executeAndCaptureOutput() ([]byte, error) {
	cmd := c.GetCmd()
	return cmd.CombinedOutput()
}

// GetCmd returns the exec.Cmd for the Conan command.
func (c *ConanCommand) GetCmd() *exec.Cmd {
	args := append([]string{c.commandName}, c.args...)
	return exec.Command("conan", args...)
}

// GetEnv returns environment variables for the command.
func (c *ConanCommand) GetEnv() map[string]string {
	return map[string]string{}
}

// GetStdWriter returns the stdout writer.
func (c *ConanCommand) GetStdWriter() io.WriteCloser {
	return nil
}

// GetErrWriter returns the stderr writer.
func (c *ConanCommand) GetErrWriter() io.WriteCloser {
	return nil
}

// CommandName returns the command identifier for logging.
func (c *ConanCommand) CommandName() string {
	return "rt_conan"
}

// ServerDetails returns the server configuration.
func (c *ConanCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// DependencyCollector handles Conan dependency collection using FlexPack.
type DependencyCollector struct {
	workingDir         string
	buildConfiguration *buildUtils.BuildConfiguration
}

// NewDependencyCollector creates a new dependency collector.
func NewDependencyCollector(workingDir string, buildConfig *buildUtils.BuildConfiguration) *DependencyCollector {
	return &DependencyCollector{
		workingDir:         workingDir,
		buildConfiguration: buildConfig,
	}
}

// Collect collects dependencies and saves build info.
func (dc *DependencyCollector) Collect() error {
	buildName, err := dc.buildConfiguration.GetBuildName()
	if err != nil {
		return fmt.Errorf("get build name: %w", err)
	}

	buildNumber, err := dc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return fmt.Errorf("get build number: %w", err)
	}

	// Use FlexPack to collect dependencies
	collector, err := NewFlexPackCollector(dc.workingDir)
	if err != nil {
		return fmt.Errorf("create flexpack collector: %w", err)
	}

	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("collect build info: %w", err)
	}

	// Save build info locally
	return saveBuildInfoLocally(buildInfo)
}

// saveBuildInfoLocally saves the build info for later publishing with 'jf rt bp'.
func saveBuildInfoLocally(buildInfo *entities.BuildInfo) error {
	service := build.NewBuildInfoService()

	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, "")
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}

	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}

	log.Info("Build info saved locally.")
	return nil
}
