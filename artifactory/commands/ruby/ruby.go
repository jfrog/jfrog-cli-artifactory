package ruby

import (
	"fmt"
	"net/url"
	"os/exec"

	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

const (
	gemRemoteRegistryFlag = "--source"
)

type RubyCommand struct {
	serverDetails *config.ServerDetails
	commandName   string
	args          []string
	repository    string
}

func NewRubyCommand() *RubyCommand {
	return &RubyCommand{}
}

func (rc *RubyCommand) SetRepo(repo string) *RubyCommand {
	rc.repository = repo
	return rc
}

func (rc *RubyCommand) SetArgs(arguments []string) *RubyCommand {
	rc.args = arguments
	return rc
}

func (rc *RubyCommand) SetCommandName(commandName string) *RubyCommand {
	rc.commandName = commandName
	return rc
}

func (rc *RubyCommand) SetServerDetails(serverDetails *config.ServerDetails) *RubyCommand {
	rc.serverDetails = serverDetails
	return rc
}

func (rc *RubyCommand) ServerDetails() (*config.ServerDetails, error) {
	return rc.serverDetails, nil
}

// GetRubyGemsRepoUrlWithCredentials gets the RubyGems repository url and the credentials.
func GetRubyGemsRepoUrlWithCredentials(serverDetails *config.ServerDetails, repository string) (*url.URL, string, string, error) {
	rtUrl, err := url.Parse(serverDetails.GetArtifactoryUrl())
	if err != nil {
		return nil, "", "", errorutils.CheckError(err)
	}

	username := serverDetails.GetUser()
	password := serverDetails.GetPassword()

	// Get credentials from access-token if exists.
	if serverDetails.GetAccessToken() != "" {
		if username == "" {
			username = auth.ExtractUsernameFromAccessToken(serverDetails.GetAccessToken())
		}
		password = serverDetails.GetAccessToken()
	}

	rtUrl = rtUrl.JoinPath("api/gems", repository)
	return rtUrl, username, password, err
}

// GetRubyGemsRepoUrl gets the RubyGems repository embedded credentials URL (https://<user>:<password/token>@<your-artifactory-url>/artifactory/api/gems/<repo-name>/)
func GetRubyGemsRepoUrl(serverDetails *config.ServerDetails, repository string) (string, error) {
	rtUrl, username, password, err := GetRubyGemsRepoUrlWithCredentials(serverDetails, repository)
	if err != nil {
		return "", err
	}
	if password != "" {
		rtUrl.User = url.UserPassword(username, password)
	}
	return rtUrl.String(), err
}

func RunConfigCommand(buildTool project.ProjectType, args []string) error {
	configCmd := gofrogcmd.NewCommand(buildTool.String(), "config", args)
	if err := gofrogcmd.RunCmd(configCmd); err != nil {
		return errorutils.CheckErrorf("%s config command failed with: %q", buildTool.String(), err)
	}
	return nil
}

// RunGemCommand runs a gem command with the provided arguments
func RunGemCommand(args []string) error {
	cmd := exec.Command("gem", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorutils.CheckErrorf("gem command failed with: %q, output: %s", err, string(output))
	}
	return nil
}

// CreateGemrc creates a .gemrc configuration file for authentication
func CreateGemrc(repoUrl, username, password string) error {
	// TODO: Implement .gemrc creation logic
	// This would create a .gemrc file in the user's home directory
	// with the appropriate authentication configuration
	return fmt.Errorf("CreateGemrc not yet implemented")
}
