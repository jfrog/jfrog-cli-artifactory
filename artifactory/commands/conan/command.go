package conan

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
// When build info is configured, it obtains structured JSON output from Conan in one of two ways:
//  1. If the user already specified --out-file, the command runs normally and the JSON is read from that file.
//  2. Otherwise, --format=json is added and stdout (containing JSON) is captured via RunCmdOutput.
func (c *ConanCommand) runUploadCommand() error {
	log.Info(fmt.Sprintf("Running Conan %s", c.commandName))

	if c.buildConfiguration == nil {
		if err := gofrogcmd.RunCmd(c); err != nil {
			return fmt.Errorf("conan %s failed: %w", c.commandName, err)
		}
		return nil
	}

	// Check if the user already provided --out-file (implies a Conan version that supports it)
	if outFile := extractOutFilePath(c.args); outFile != "" {
		if !hasFormatFlag(c.args) {
			log.Debug("Adding --format=json to conan upload for build info collection")
			c.args = append(c.args, "--format=json")
		}
		if err := gofrogcmd.RunCmd(c); err != nil {
			return fmt.Errorf("conan %s failed: %w", c.commandName, err)
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			return fmt.Errorf("could not read upload output file %s: %w", outFile, err)
		}
		return c.processBuildInfoFromJSON(string(data))
	}

	// No --out-file: add --format=json and capture stdout
	if !hasFormatFlag(c.args) {
		log.Debug("Adding --format=json to conan upload for build info collection")
		c.args = append(c.args, "--format=json")
	}
	jsonOutput, err := gofrogcmd.RunCmdOutput(c)
	if err != nil {
		return fmt.Errorf("conan %s failed: %w", c.commandName, err)
	}
	return c.processBuildInfoFromJSON(jsonOutput)
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

// processBuildInfoFromJSON parses the JSON string captured from conan upload stdout
// and processes it for build info collection.
func (c *ConanCommand) processBuildInfoFromJSON(jsonData string) error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Processing Conan upload with build info: %s/%s", buildName, buildNumber))

	var uploadOutput ConanUploadOutput
	if err := json.Unmarshal([]byte(jsonData), &uploadOutput); err != nil {
		preview := jsonData
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return fmt.Errorf("could not parse upload JSON output: %w\nJSON preview: %s", err, preview)
	}

	processor := NewUploadProcessor(c.workingDir, c.buildConfiguration, c.serverDetails, c.buildConanFlexConfig())
	if err := processor.ProcessJSON(uploadOutput); err != nil {
		return fmt.Errorf("failed to process Conan upload: %w", err)
	}

	log.Info(fmt.Sprintf("Conan build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

// hasFormatFlag checks if the user already specified --format or -f in their args.
func hasFormatFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--format" || arg == "-f" ||
			strings.HasPrefix(arg, "--format=") || strings.HasPrefix(arg, "-f=") {
			return true
		}
	}
	return false
}

// extractOutFilePath returns the file path from --out-file if specified in args, or "" if absent.
func extractOutFilePath(args []string) string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "--out-file=") {
			return strings.TrimPrefix(arg, "--out-file=")
		}
		if arg == "--out-file" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// collectAndSaveBuildInfo collects dependencies and saves build info locally.
func (c *ConanCommand) collectAndSaveBuildInfo() error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil // No build info configured, skip silently
	}

	log.Info(fmt.Sprintf("Collecting build info for Conan project: %s/%s", buildName, buildNumber))

	conanConfig := c.buildConanFlexConfig()

	collector, err := conanflex.NewConanFlexPack(conanConfig)
	if err != nil {
		return fmt.Errorf("failed to create Conan FlexPack: %w", err)
	}
	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect Conan build info: %w", err)
	}
	projectKey := c.buildConfiguration.GetProject()
	if err := saveBuildInfoLocally(buildInfo, projectKey); err != nil {
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

// buildConanFlexConfig builds a ConanConfig with recipe path and name/version overrides
// extracted from the command arguments. This ensures build info collection finds the
// conanfile even when the recipe is not in the current working directory.
func (c *ConanCommand) buildConanFlexConfig() conanflex.ConanConfig {
	recipePath := extractRecipePathFromArgs(c.workingDir, c.args)
	overrides := extractReferenceOverridesFromArgs(c.args)
	return conanflex.ConanConfig{
		WorkingDirectory:       c.workingDir,
		RecipeFilePath:         recipePath,
		ProjectNameOverride:    overrides.name,
		ProjectVersionOverride: overrides.version,
		UserOverride:           overrides.user,
		ChannelOverride:        overrides.channel,
		ConanArgs:              c.args,
	}
}

// extractRecipePathFromArgs finds the recipe path from the Conan command arguments.
// Returns "" if no recipe path is found (callers fall back to WorkingDirectory).
func extractRecipePathFromArgs(workingDir string, args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.Contains(arg, "@") {
			continue
		}

		candidate := arg
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(workingDir, candidate)
		}
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}

		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			return absPath
		}
		return filepath.Dir(absPath)
	}
	return ""
}

type referenceOverrides struct {
	name, version, user, channel string
}

// extractReferenceOverridesFromArgs parses --name, --version, --user, and --channel
// from Conan command arguments. Supports both --flag=value and --flag value forms.
func extractReferenceOverridesFromArgs(args []string) referenceOverrides {
	var o referenceOverrides
	for i, arg := range args {
		o.name = extractFlagValue(arg, i, args, "--name", o.name)
		o.version = extractFlagValue(arg, i, args, "--version", o.version)
		o.user = extractFlagValue(arg, i, args, "--user", o.user)
		o.channel = extractFlagValue(arg, i, args, "--channel", o.channel)
	}
	return o
}

// extractFlagValue checks if arg matches the given flag (either --flag=value or --flag value form)
// and returns the extracted value, or the current value if no match.
func extractFlagValue(arg string, idx int, args []string, flag, current string) string {
	prefix := flag + "="
	if strings.HasPrefix(arg, prefix) {
		return strings.TrimPrefix(arg, prefix)
	}
	if arg == flag && idx+1 < len(args) && !strings.HasPrefix(args[idx+1], "-") {
		return args[idx+1]
	}
	return current
}

// saveBuildInfoLocally saves the build info for later publishing with 'jf rt bp'.
func saveBuildInfoLocally(buildInfo *entities.BuildInfo, projectKey string) error {
	service := buildUtils.CreateBuildInfoService()

	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, projectKey)
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}

	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}

	log.Debug("Build info saved locally")
	return nil
}
