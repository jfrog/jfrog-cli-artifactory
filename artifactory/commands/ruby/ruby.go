package ruby

import (
	"net/url"

	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// RubyCommand runs a native RubyGems (`gem`) or Bundler (`bundle`) command directly,
// injecting Artifactory authentication and optionally collecting build info.
//
// It is a config-less native flow (like `jf uv`): the server is resolved from
// --server-id or the default server, and the Artifactory repository is discovered
// from the project's own Ruby configuration (Gemfile source, .bundle/config,
// gem sources). The `.jfrog/projects/ruby.yaml` produced by `jf ruby-config` is NOT
// read by this command.
type RubyCommand struct {
	serverDetails *config.ServerDetails
	// nativeTool is the underlying binary to run: "gem" or "bundle".
	nativeTool string
	// args are the arguments passed to the native tool, including its sub-command
	// (e.g. ["install", "--without", "test"]).
	args []string
	// serverID is the explicit --server-id, empty for the default server.
	serverID string
	// repository optionally overrides the Artifactory repo (otherwise auto-discovered).
	repository         string
	buildConfiguration *buildUtils.BuildConfiguration
}

func NewRubyCommand() *RubyCommand {
	return &RubyCommand{}
}

func (rc *RubyCommand) SetNativeTool(tool string) *RubyCommand {
	rc.nativeTool = tool
	return rc
}

func (rc *RubyCommand) SetRepo(repo string) *RubyCommand {
	rc.repository = repo
	return rc
}

func (rc *RubyCommand) SetArgs(arguments []string) *RubyCommand {
	rc.args = arguments
	return rc
}

func (rc *RubyCommand) SetServerID(serverID string) *RubyCommand {
	rc.serverID = serverID
	return rc
}

func (rc *RubyCommand) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) *RubyCommand {
	rc.buildConfiguration = bc
	return rc
}

func (rc *RubyCommand) SetServerDetails(serverDetails *config.ServerDetails) *RubyCommand {
	rc.serverDetails = serverDetails
	return rc
}

func (rc *RubyCommand) ServerDetails() (*config.ServerDetails, error) {
	if rc.serverDetails != nil {
		return rc.serverDetails, nil
	}
	return rubyResolveServerDetails(rc.serverID)
}

// CommandName is the usage-report metric id for native Ruby commands.
func (rc *RubyCommand) CommandName() string {
	return "rt_ruby_native"
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
