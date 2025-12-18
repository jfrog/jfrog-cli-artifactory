package helm

import (
	"fmt"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"io"
	"net/url"
	"os"
	"os/exec"
)

// HelmCommand represents a Helm command execution
type HelmCommand struct {
	cmdName            string
	helmArgs           []string
	serverDetails      *config.ServerDetails
	workingDirectory   string
	serverId           string
	username           string
	password           string
	buildConfiguration *buildUtils.BuildConfiguration
}

// NewHelmCommand creates a new HelmCommand instance
func NewHelmCommand() *HelmCommand {
	return &HelmCommand{}
}

// CommandName returns the command name for this Helm command
func (hc *HelmCommand) CommandName() string {
	return hc.cmdName
}

// SetHelmCmdName sets the Helm command name
func (hc *HelmCommand) SetHelmCmdName(cmdName string) *HelmCommand {
	hc.cmdName = cmdName
	return hc
}

// SetHelmArgs sets the Helm command arguments
func (hc *HelmCommand) SetHelmArgs(helmArgs []string) *HelmCommand {
	hc.helmArgs = helmArgs
	return hc
}

// SetBuildConfiguration sets the build configuration
func (hc *HelmCommand) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *HelmCommand {
	hc.buildConfiguration = buildConfiguration
	return hc
}

// SetServerDetails sets the server details
func (hc *HelmCommand) SetServerDetails(serverDetails *config.ServerDetails) *HelmCommand {
	hc.serverDetails = serverDetails
	return hc
}

// SetWorkingDirectory sets the working directory
func (hc *HelmCommand) SetWorkingDirectory(workingDirectory string) *HelmCommand {
	hc.workingDirectory = workingDirectory
	return hc
}

// SetUsername sets the username
func (hc *HelmCommand) SetUsername(username string) *HelmCommand {
	hc.username = username
	return hc
}

// SetPassword sets the password
func (hc *HelmCommand) SetPassword(password string) *HelmCommand {
	hc.password = password
	return hc
}

// SetServerId sets the server ID
func (hc *HelmCommand) SetServerId(serverId string) *HelmCommand {
	hc.serverId = serverId
	return hc
}

// ServerDetails returns the server details
func (hc *HelmCommand) ServerDetails() (*config.ServerDetails, error) {
	return hc.serverDetails, nil
}

// Run executes the Helm command
func (hc *HelmCommand) Run() error {
	hc.appendCredentialsInArguments()
	if err := hc.executeHelmCommand(); err != nil {
		return errorutils.CheckErrorf("helm %s failed: %w", hc.cmdName, err)
	}
	if err := hc.collectBuildInfoIfNeeded(); err != nil {
		return errorutils.CheckError(err)
	}
	return nil
}

// appendCredentialsInArguments appends the username and password to arguments
func (hc *HelmCommand) appendCredentialsInArguments() {
	if hc.username != "" && hc.password != "" {
		hc.helmArgs = append(hc.helmArgs, "--username", hc.username)
		hc.helmArgs = append(hc.helmArgs, "--password", hc.password)
		return
	}
	if hc.cmdName != "registry" && hc.serverId == "" {
		return
	}
	username, password := hc.getCredentials()
	if username == "" || password == "" {
		log.Debug("No credentials available for helm registry login")
		return
	}
	hc.helmArgs = append(hc.helmArgs, fmt.Sprintf("--username=%s", username))
	hc.helmArgs = append(hc.helmArgs, fmt.Sprintf("--password=%s", password))
}

// executeHelmCommand executes the native Helm command
func (hc *HelmCommand) executeHelmCommand() error {
	log.Info("Running Helm ", hc.cmdName, ".")
	args := append([]string{hc.cmdName}, hc.helmArgs...)
	helmCmd := exec.Command("helm", args...)
	helmCmd.Stdout = os.Stdout
	helmCmd.Stderr = os.Stderr
	helmCmd.Stdin = os.Stdin
	helmCmd.Dir = hc.workingDirectory
	return helmCmd.Run()
}

// collectBuildInfoIfNeeded collects build info if configuration is provided
func (hc *HelmCommand) collectBuildInfoIfNeeded() error {
	if hc.buildConfiguration == nil {
		return nil
	}
	isCollectBuildInfo, err := hc.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return errorutils.CheckError(err)
	}
	if !isCollectBuildInfo {
		return nil
	}
	if !needBuildInfo(hc.cmdName) {
		log.Debug("Skipping build info for ", hc.cmdName)
		return nil
	}
	log.Info("Collecting build info for executed helm ", hc.cmdName, "command")
	buildName, err := hc.buildConfiguration.GetBuildName()
	if err != nil {
		return errorutils.CheckError(err)
	}
	buildNumber, err := hc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return errorutils.CheckError(err)
	}
	project := hc.buildConfiguration.GetProject()
	err = CollectHelmBuildInfoWithFlexPack(hc.workingDirectory, buildName, buildNumber, project, hc.cmdName, hc.helmArgs, hc.serverDetails)
	return errorutils.CheckError(err)
}

// performRegistryLogin performs helm registry login using stored credentials
func (hc *HelmCommand) performRegistryLogin() error {
	if hc.serverDetails == nil {
		log.Debug("No server details available for helm registry login")
		return nil
	}
	registryURL, err := hc.getRegistryURL()
	if err != nil {
		log.Debug("Failed to get registry URL: ", err)
		return nil
	}
	if registryURL == "" {
		log.Debug("No server URL available for helm registry login")
		return nil
	}
	user, pass := hc.getCredentials()
	if user == "" || pass == "" {
		log.Debug("No credentials available for helm registry login")
		return nil
	}
	return hc.executeHelmLogin(registryURL, user, pass)
}

// getRegistryURL extracts registry URL from server details
func (hc *HelmCommand) getRegistryURL() (string, error) {
	if hc.serverDetails.ArtifactoryUrl != "" {
		parsedURL, err := url.Parse(hc.serverDetails.ArtifactoryUrl)
		if err != nil {
			return "", fmt.Errorf("failed to parse Artifactory URL: %w", err)
		}
		return parsedURL.Host, nil
	}
	return "", nil
}

// getCredentials extracts credentials from command or server details
func (hc *HelmCommand) getCredentials() (string, string) {
	user := hc.username
	pass := hc.password

	if user == "" && hc.serverDetails != nil {
		user = hc.serverDetails.User
	}

	if hc.serverDetails != nil {
		if hc.serverDetails.AccessToken != "" {
			if user == "" {
				user = auth.ExtractUsernameFromAccessToken(hc.serverDetails.AccessToken)
			}
			pass = hc.serverDetails.AccessToken
		} else if pass == "" {
			pass = hc.serverDetails.Password
		}
	}
	return user, pass
}

// executeHelmLogin executes the helm registry login command
func (hc *HelmCommand) executeHelmLogin(registryURL, user, pass string) error {
	log.Debug("Performing helm registry login to", registryURL, " with user ", user)
	cmdLogin := exec.Command("helm", "registry", "login", registryURL, "--username", user, "--password", pass)
	cmdLogin.Stdout = io.Discard
	cmdLogin.Stderr = os.Stderr
	if err := cmdLogin.Run(); err != nil {
		return fmt.Errorf("helm registry login failed: %w", err)
	}
	log.Debug("Helm registry login to successful, ", registryURL)
	return nil
}
