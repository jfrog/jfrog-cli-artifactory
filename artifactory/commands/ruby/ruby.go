package ruby

import (
	"net/url"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
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
