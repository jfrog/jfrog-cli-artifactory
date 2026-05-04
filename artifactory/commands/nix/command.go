package nix

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	nixflex "github.com/jfrog/build-info-go/flexpack/nix"
	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// NixCommand represents a Nix CLI command with build info support.
// Follows the FlexPack native-first pattern:
//   - No config command (like Conan, unlike npm/maven)
//   - User configures Nix substituters to point to Artifactory (one-time setup)
//   - JFrog CLI handles auth via netrc and collects build info after native execution
type NixCommand struct {
	commandName        string
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
	workingDir         string
	repo               string // Target repo for artifact upload and substituter (--repo flag)
	netrcPath          string // Temp netrc file, cleaned up after Run
	nixConfPath        string // Path to user nix.conf (for restore)
	originalNixConf    string // Original nix.conf content (for restore)
}

func NewNixCommand() *NixCommand {
	return &NixCommand{}
}

func (c *NixCommand) SetCommandName(name string) *NixCommand {
	c.commandName = name
	return c
}

func (c *NixCommand) SetArgs(args []string) *NixCommand {
	c.args = args
	return c
}

func (c *NixCommand) SetServerDetails(details *config.ServerDetails) *NixCommand {
	c.serverDetails = details
	return c
}

func (c *NixCommand) SetBuildConfiguration(config *buildUtils.BuildConfiguration) *NixCommand {
	c.buildConfiguration = config
	return c
}

func (c *NixCommand) SetRepo(repo string) *NixCommand {
	c.repo = repo
	return c
}

// Commands that resolve dependencies and should trigger build-info collection.
var commandsCollectingDeps = []string{
	"build",
	"develop",
	"flake",
	"run",
	"shell",
	"profile",
}

func shouldCollectDeps(cmd string) bool {
	for _, c := range commandsCollectingDeps {
		if c == cmd {
			return true
		}
	}
	return false
}

// Commands that produce uploadable artifacts.
var commandsProducingArtifacts = []string{
	"build",
}

func shouldCollectArtifacts(cmd string) bool {
	for _, c := range commandsProducingArtifacts {
		if c == cmd {
			return true
		}
	}
	return false
}

func (c *NixCommand) Run() error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	c.workingDir = workingDir

	// Set up Artifactory substituter in nix.conf and create netrc for auth.
	// Like npm modifying .npmrc or Conan's auto-login — everything is restored after.
	if c.serverDetails != nil && c.repo != "" {
		c.setupSubstituterAndAuth()
		defer func() {
			c.restoreNixConf()
			if c.netrcPath != "" {
				os.Remove(c.netrcPath)
			}
		}()
	}

	log.Info(fmt.Sprintf("Running Nix %s", c.commandName))

	if err := gofrogcmd.RunCmd(c); err != nil {
		return fmt.Errorf("nix %s failed: %w", c.commandName, err)
	}

	if c.buildConfiguration != nil && shouldCollectDeps(c.commandName) {
		return c.collectAndSaveBuildInfo()
	}

	return nil
}

// setupSubstituterAndAuth configures Nix to resolve deps through Artifactory.
// This is the Nix equivalent of npm modifying .npmrc or Conan's autoLoginToRemotes():
//  1. Writes the Artifactory substituter URL to ~/.config/nix/nix.conf
//  2. Creates a temporary netrc file for HTTP auth
//  3. Passes --option netrc-file via CLI flags
//
// The original nix.conf is backed up and restored after the command completes.
// Prerequisite: user must be in trusted-users in /etc/nix/nix.conf (one-time admin setup).
func (c *NixCommand) setupSubstituterAndAuth() {
	if c.serverDetails == nil || c.repo == "" {
		return
	}

	substituterURL := strings.TrimSuffix(c.serverDetails.ArtifactoryUrl, "/") + "/api/nix/" + c.repo

	// Step 1: Write substituter to user nix.conf (like npm modifying .npmrc)
	if err := c.addSubstituterToNixConf(substituterURL); err != nil {
		log.Warn("Failed to configure Artifactory substituter in nix.conf: " + err.Error())
		log.Warn("Nix will resolve deps from default substituters (cache.nixos.org)")
	} else {
		log.Info(fmt.Sprintf("Configured Artifactory substituter: %s", substituterURL))
	}

	// Step 2: Create netrc file for HTTP auth (like Conan's remote login)
	c.createNetrcFile()

	// Netrc file is passed via NIX_CONFIG env var in GetEnv(), not via CLI flags,
	// because --option must precede the subcommand (e.g. nix --option X flake lock)
	// but our command routing puts flags after the subcommand.
}

// nixConfMarker is used to identify lines added by JFrog CLI, for clean restore.
const nixConfMarker = "# jfrog-cli-managed"

// addSubstituterToNixConf adds the Artifactory substituter URL to ~/.config/nix/nix.conf.
// Backs up the original content so it can be restored after the command completes.
func (c *NixCommand) addSubstituterToNixConf(substituterURL string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	nixConfDir := filepath.Join(homeDir, ".config", "nix")
	nixConfPath := filepath.Join(nixConfDir, "nix.conf")

	// Read existing content
	existingContent, _ := os.ReadFile(nixConfPath)
	c.originalNixConf = string(existingContent)
	c.nixConfPath = nixConfPath

	// Check if substituter is already configured
	if strings.Contains(string(existingContent), substituterURL) {
		log.Debug("Artifactory substituter already configured in nix.conf")
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(nixConfDir, 0755); err != nil {
		return fmt.Errorf("create nix config dir: %w", err)
	}

	// Append substituter lines
	extraLines := fmt.Sprintf("\nextra-substituters = %s %s\n", substituterURL, nixConfMarker)

	newContent := string(existingContent) + extraLines

	if err := os.WriteFile(nixConfPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("write nix.conf: %w", err)
	}

	return nil
}

// restoreNixConf restores the original ~/.config/nix/nix.conf content.
func (c *NixCommand) restoreNixConf() {
	if c.nixConfPath == "" {
		return
	}
	// Remove only the lines we added (lines containing our marker)
	currentContent, err := os.ReadFile(c.nixConfPath)
	if err != nil {
		log.Debug("Failed to read nix.conf for restore: " + err.Error())
		return
	}

	var cleanedLines []string
	for _, line := range strings.Split(string(currentContent), "\n") {
		if !strings.Contains(line, nixConfMarker) {
			cleanedLines = append(cleanedLines, line)
		}
	}
	cleaned := strings.Join(cleanedLines, "\n")
	// Remove trailing empty lines we may have introduced
	cleaned = strings.TrimRight(cleaned, "\n") + "\n"

	if err := os.WriteFile(c.nixConfPath, []byte(cleaned), 0644); err != nil {
		log.Debug("Failed to restore nix.conf: " + err.Error())
	} else {
		log.Debug("Restored nix.conf")
	}
}

// createNetrcFile creates a temporary netrc file for Nix to authenticate to Artifactory.
func (c *NixCommand) createNetrcFile() {
	user := c.serverDetails.User
	password := c.serverDetails.Password
	if password == "" {
		password = c.serverDetails.AccessToken
	}
	if user == "" || password == "" {
		return
	}

	host := c.serverDetails.ArtifactoryUrl
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	host = strings.TrimSuffix(host, "/")
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}

	netrcContent := fmt.Sprintf("machine %s\nlogin %s\npassword %s\n", host, user, password)

	tmpFile, err := os.CreateTemp("", "nix-netrc-")
	if err != nil {
		log.Debug("Failed to create netrc file: " + err.Error())
		return
	}
	if _, err := tmpFile.WriteString(netrcContent); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		log.Debug("Failed to write netrc file: " + err.Error())
		return
	}
	tmpFile.Close()

	c.netrcPath = tmpFile.Name()
	log.Debug("Created netrc file for Artifactory auth")
}

func (c *NixCommand) collectAndSaveBuildInfo() error {
	buildName, buildNumber, err := c.getBuildNameAndNumber()
	if err != nil || buildName == "" || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Collecting build info for Nix project: %s/%s", buildName, buildNumber))

	collector, err := nixflex.NewNixFlexPack(nixflex.NixConfig{
		WorkingDirectory: c.workingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to create Nix FlexPack: %w", err)
	}

	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect Nix build info: %w", err)
	}

	// Collect and upload artifacts for build commands
	if shouldCollectArtifacts(c.commandName) && c.repo != "" {
		artifacts, err := c.collectAndUploadArtifacts(buildName, buildNumber)
		if err != nil {
			log.Warn("Failed to collect artifacts: " + err.Error())
		} else if len(artifacts) > 0 && len(buildInfo.Modules) > 0 {
			buildInfo.Modules[0].Artifacts = artifacts
			log.Info(fmt.Sprintf("Added %d artifact(s) to build info", len(artifacts)))
		}
	}

	// Apply --module override if specified
	moduleOverride := c.buildConfiguration.GetModule()
	if moduleOverride != "" && len(buildInfo.Modules) > 0 {
		buildInfo.Modules[0].Id = moduleOverride
	}

	projectKey := c.buildConfiguration.GetProject()
	if err := saveBuildInfoLocally(buildInfo, projectKey); err != nil {
		return fmt.Errorf("failed to save build info: %w", err)
	}

	log.Info(fmt.Sprintf("Nix build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

func (c *NixCommand) collectAndUploadArtifacts(buildName, buildNumber string) ([]entities.Artifact, error) {
	// Read the ./result symlink to find the store path
	resultLink := filepath.Join(c.workingDir, "result")
	storePath, err := os.Readlink(resultLink)
	if err != nil {
		return nil, fmt.Errorf("no build output found (./result symlink missing): %w", err)
	}

	baseName := filepath.Base(storePath)
	if idx := strings.Index(baseName, "-"); idx != -1 {
		baseName = baseName[idx+1:]
	}
	narFileName := baseName + ".nar"

	// Export the store path as NAR using nix nar dump-path
	tmpDir, err := os.MkdirTemp("", "nix-nar-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	narPath := filepath.Join(tmpDir, narFileName)
	narFile, err := os.Create(narPath)
	if err != nil {
		return nil, fmt.Errorf("create NAR file: %w", err)
	}

	cmd := exec.Command("nix", "nar", "dump-path", storePath)
	cmd.Stdout = narFile
	if err := cmd.Run(); err != nil {
		narFile.Close()
		return nil, fmt.Errorf("nix nar dump-path failed: %w", err)
	}
	narFile.Close()
	log.Info(fmt.Sprintf("Exported NAR: %s", narPath))

	// Calculate checksums
	fi, err := os.Stat(narPath)
	if err != nil {
		return nil, fmt.Errorf("stat NAR: %w", err)
	}
	_ = fi // NAR file exists

	uploadPath := fmt.Sprintf("%s/%s", baseName, narFileName)

	artifact := entities.Artifact{
		Name:                   narFileName,
		Type:                   "nar",
		Path:                   uploadPath,
		OriginalDeploymentRepo: c.repo,
	}

	// Upload to Artifactory if server details are available
	if c.serverDetails != nil {
		err := c.uploadNAR(narPath, c.repo, uploadPath)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to upload NAR to Artifactory: %v", err))
		} else {
			log.Info(fmt.Sprintf("Uploaded %s to %s/%s", narFileName, c.repo, uploadPath))
			c.setBuildProperties(c.repo, narFileName, buildName, buildNumber)
		}
	}

	return []entities.Artifact{artifact}, nil
}

// uploadNAR uploads a NAR file to Artifactory using the client-go upload API.
func (c *NixCommand) uploadNAR(localPath, repo, targetPath string) error {
	servicesManager, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("create service manager: %w", err)
	}

	uploadParams := services.NewUploadParams()
	uploadParams.CommonParams = &specutils.CommonParams{
		Pattern: localPath,
		Target:  repo + "/" + targetPath,
	}
	uploadParams.Flat = true

	uploaded, failed, err := servicesManager.UploadFiles(artifactory.UploadServiceOptions{}, uploadParams)
	if err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("failed to upload %d file(s)", failed)
	}
	log.Debug(fmt.Sprintf("Uploaded %d file(s)", uploaded))
	return nil
}

// setBuildProperties sets build.name, build.number properties on uploaded artifacts.
func (c *NixCommand) setBuildProperties(repo, artifactName, buildName, buildNumber string) {
	servicesManager, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		log.Debug("Failed to create service manager for properties: " + err.Error())
		return
	}

	props := fmt.Sprintf("build.name=%s;build.number=%s", buildName, buildNumber)

	searchQuery := fmt.Sprintf(`{"repo": "%s", "$or": [{"$and":[{"path": {"$match": "*"},"name": {"$match": "%s"}}]}]}`, repo, artifactName)
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: searchQuery},
		},
	}

	reader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		log.Debug("Failed to search for artifact: " + err.Error())
		return
	}

	propsParams := services.PropsParams{
		Reader: reader,
		Props:  props,
	}
	_, err = servicesManager.SetProps(propsParams)
	if err != nil {
		log.Debug("Failed to set build properties: " + err.Error())
	}
}

func (c *NixCommand) getBuildNameAndNumber() (string, string, error) {
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

func (c *NixCommand) GetCmd() *exec.Cmd {
	args := append([]string{c.commandName}, c.args...)
	return exec.Command("nix", args...)
}

func (c *NixCommand) GetEnv() map[string]string {
	env := map[string]string{}
	if c.netrcPath != "" {
		// Pass netrc-file via NIX_CONFIG rather than --option flag.
		// The --option flag must precede the subcommand in nix CLI,
		// but our command routing places args after the subcommand.
		env["NIX_CONFIG"] = "netrc-file = " + c.netrcPath
	}
	return env
}

func (c *NixCommand) GetStdWriter() io.WriteCloser {
	return nil
}

func (c *NixCommand) GetErrWriter() io.WriteCloser {
	return nil
}

func (c *NixCommand) CommandName() string {
	return "rt_nix"
}

func (c *NixCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

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
