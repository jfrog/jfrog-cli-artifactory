package conan

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	conanflex "github.com/jfrog/build-info-go/flexpack/conan"
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

// Commands that may need remote access for downloading dependencies or packages.
// These commands might interact with Conan remotes and require authentication.
var commandsNeedingRemoteAccess = []string{
	"install",
	"create",
	"build",
	"download",
	"upload",
	"search",
	"list",
}

// needsRemoteAccess checks if a command might need remote access.
func needsRemoteAccess(cmd string) bool {
	for _, c := range commandsNeedingRemoteAccess {
		if c == cmd {
			return true
		}
	}
	return false
}

// Run executes the Conan command with build info collection.
func (c *ConanCommand) Run() error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	c.workingDir = workingDir

	// Perform auto-login for commands that need remote access
	if needsRemoteAccess(c.commandName) {
		if err := c.autoLoginToRemotes(); err != nil {
			log.Debug(fmt.Sprintf("Auto-login warning: %v", err))
		}
	}

	// Upload command requires special handling to parse output and collect artifacts
	if c.commandName == "upload" {
		return c.runUploadCommand()
	}

	// Run other Conan commands
	return c.runConanCommand()
}

// autoLoginToRemotes attempts to log into all configured Conan remotes that match JFrog CLI configs.
func (c *ConanCommand) autoLoginToRemotes() error {
	// First check if a specific remote is specified in args
	remoteName := ExtractRemoteName(c.args)
	if remoteName != "" {
		matchedServer, err := ValidateAndLogin(remoteName)
		if err != nil {
			log.Debug(fmt.Sprintf("Could not auto-login to remote '%s': %v", remoteName, err))
		} else {
			c.serverDetails = matchedServer
		}
		return nil
	}

	// No specific remote specified, try to login to all Artifactory remotes
	return c.loginToAllArtifactoryRemotes()
}

// loginToAllArtifactoryRemotes attempts to log into all Conan remotes that point to Artifactory.
func (c *ConanCommand) loginToAllArtifactoryRemotes() error {
	remotes, err := ListConanRemotes()
	if err != nil {
		return fmt.Errorf("list conan remotes: %w", err)
	}

	loggedInCount := 0
	for _, remote := range remotes {
		// Only process remotes with /api/conan/ URL pattern (Artifactory Conan repos)
		if !isArtifactoryConanRemote(remote.URL) {
			log.Debug(fmt.Sprintf("Skipping remote '%s': not an Artifactory Conan URL", remote.Name))
			continue
		}

		log.Debug(fmt.Sprintf("Found Artifactory Conan remote: %s -> %s", remote.Name, remote.URL))

		matchedServer, err := ValidateAndLogin(remote.Name)
		if err != nil {
			log.Debug(fmt.Sprintf("Could not auto-login to remote '%s': %v", remote.Name, err))
			continue
		}

		loggedInCount++
		log.Debug(fmt.Sprintf("Successfully logged into remote '%s'", remote.Name))

		// Use the first successfully logged-in server for artifact collection
		if c.serverDetails == nil {
			c.serverDetails = matchedServer
		}
	}

	if loggedInCount > 0 {
		log.Debug(fmt.Sprintf("Auto-login completed: logged into %d Artifactory remote(s)", loggedInCount))
	}

	return nil
}

// isArtifactoryConanRemote checks if a URL points to an Artifactory Conan repository.
// Artifactory Conan URLs contain /api/conan/ in the path.
func isArtifactoryConanRemote(url string) bool {
	return strings.Contains(url, "/api/conan/")
}

// runUploadCommand handles the upload command with build info collection.
// Upload requires special handling because:
// 1. We need to capture the output to determine which artifacts were uploaded
// 2. We need to collect artifacts from Artifactory and set build properties
func (c *ConanCommand) runUploadCommand() error {
	log.Info(fmt.Sprintf("Running Conan %s", c.commandName))

	// Execute conan upload and capture output
	output, err := c.executeAndCaptureOutput()
	if err != nil {
		fmt.Print(string(output))
		return fmt.Errorf("conan %s failed: %w", c.commandName, err)
	}
	fmt.Print(string(output))

	// Process upload for build info if build configuration is provided
	if c.buildConfiguration != nil {
		return c.processBuildInfo(string(output))
	}

	return nil
}

// runConanCommand runs non-upload Conan commands.
func (c *ConanCommand) runConanCommand() error {
	log.Info(fmt.Sprintf("Running Conan %s", c.commandName))

	if err := gofrogcmd.RunCmd(c); err != nil {
		return fmt.Errorf("conan %s failed: %w", c.commandName, err)
	}

	// Collect build info for dependency commands
	if c.buildConfiguration != nil {
		return c.collectAndSaveBuildInfo()
	}

	return nil
}

// processBuildInfo processes build info after a successful upload.
func (c *ConanCommand) processBuildInfo(uploadOutput string) error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil // No build info configured, skip silently
	}

	log.Info(fmt.Sprintf("Processing Conan upload with build info: %s/%s", buildName, buildNumber))

	processor := NewUploadProcessor(c.workingDir, c.buildConfiguration, c.serverDetails)
	if err := processor.Process(uploadOutput); err != nil {
		log.Warn("Failed to process Conan upload: " + err.Error())
	}

	log.Info(fmt.Sprintf("Conan build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

// collectAndSaveBuildInfo collects dependencies and saves build info locally.
func (c *ConanCommand) collectAndSaveBuildInfo() error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil // No build info configured, skip silently
	}

	log.Info(fmt.Sprintf("Collecting build info for Conan project: %s/%s", buildName, buildNumber))

	// Create FlexPack collector
	conanConfig := conanflex.ConanConfig{
		WorkingDirectory: c.workingDir,
	}

	collector, err := conanflex.NewConanFlexPack(conanConfig)
	if err != nil {
		return fmt.Errorf("failed to create Conan FlexPack: %w", err)
	}
	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect Conan build info: %w", err)
	}
	if err := saveBuildInfoLocally(buildInfo); err != nil {
		return fmt.Errorf("failed to save build info: %w", err)
	}

	log.Info(fmt.Sprintf("Conan build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

// getBuildNameAndNumber returns build name and number from configuration.
// Returns error if either is missing.
func (c *ConanCommand) getBuildNameAndNumber() (string, string, error) {
	buildName, err := c.buildConfiguration.GetBuildName()
	if err != nil || buildName == "" {
		return "", "", fmt.Errorf("build name not configured")
	}

	buildNumber, err := c.buildConfiguration.GetBuildNumber()
	if err != nil || buildNumber == "" {
		return "", "", fmt.Errorf("build number not configured")
	}

	return buildName, buildNumber, nil
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

	log.Debug("Build info saved locally")
	return nil
}
