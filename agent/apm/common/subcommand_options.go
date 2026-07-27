package apmcommon

import (
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

// ApmSubcommandOptions holds jf's own flags, manually extracted from a SkipFlagParsing
// subcommand's raw arguments, plus whatever args remained afterward.
type ApmSubcommandOptions struct {
	// RemainingArgs is what's left after stripping jf's own flags - passed straight through
	// to the real apm binary, so apm-native flags (--package, --registry, --zip, --dry-run,
	// etc.) survive untouched.
	RemainingArgs []string
	ServerDetails *config.ServerDetails
	BuildConfig   *buildUtils.BuildConfiguration
}

// ExtractApmSubcommandOptions extracts every flag declared for install/publish/update
// (--server-id, --build-name, --build-number, --module, --project, --url, --user,
// --password, --access-token) from args, and resolves them into ServerDetails and a
// BuildConfiguration.
//
// This exists because install/publish/update set SkipFlagParsing (so apm-native flags that
// aren't in jf's own declared flag set - --package, --registry, --zip, --dry-run - don't get
// rejected by urfave/cli before ever reaching apm). SkipFlagParsing means urfave/cli parses
// NONE of the flags itself, jf's own included, so every one of them has to be pulled out by
// hand. Unlike passthrough (which still supports --repo to declare a new registry inline),
// install/publish/update require a registry to already be declared, so --repo isn't one of
// jf's own flags here - it passes straight through in RemainingArgs like any other
// apm-native flag (where apm itself will reject it, since apm has no --repo flag either).
func ExtractApmSubcommandOptions(args []string) (*ApmSubcommandOptions, error) {
	rest := args
	var serverID, buildName, buildNumber, module, project string
	var url, user, password, accessToken string
	var err error

	for _, opt := range []struct {
		name string
		dest *string
	}{
		{"server-id", &serverID},
		{"build-name", &buildName},
		{"build-number", &buildNumber},
		{"module", &module},
		{"project", &project},
		{"url", &url},
		{"user", &user},
		{"password", &password},
		{"access-token", &accessToken},
	} {
		rest, *opt.dest, err = coreutils.ExtractStringOptionFromArgs(rest, opt.name)
		if err != nil {
			return nil, err
		}
	}

	sd, err := resolveServerDetails(serverID, url, user, password, accessToken)
	if err != nil {
		return nil, err
	}

	buildConfig := new(buildUtils.BuildConfiguration)
	if err = buildConfig.SetBuildName(buildName).SetBuildNumber(buildNumber).SetProject(project).SetModule(module).ValidateBuildAndModuleParams(); err != nil {
		return nil, err
	}

	return &ApmSubcommandOptions{
		RemainingArgs: rest,
		ServerDetails: sd,
		BuildConfig:   buildConfig,
	}, nil
}

// resolveServerDetails mirrors agent/common.GetServerDetails's two cases (explicit flags vs.
// default config), extended with a third for --server-id specifically: explicit server-id
// always wins over url/user/password/access-token, matching how the framework's own flag
// parsing would have resolved these before SkipFlagParsing made manual extraction necessary.
func resolveServerDetails(serverID, url, user, password, accessToken string) (*config.ServerDetails, error) {
	if serverID != "" {
		return config.GetSpecificConfig(serverID, true, true)
	}
	if url != "" || user != "" || password != "" || accessToken != "" {
		details := &config.ServerDetails{
			// --url is the Artifactory URL for this command domain, same as
			// createServerDetailsFromFlags's cliutils.Rt case in jfrog-cli-core -
			// NormalizeArtifactoryUrl (and everything downstream, e.g. AgentPackagesBaseURL)
			// reads ArtifactoryUrl, not Url.
			ArtifactoryUrl: url,
			User:           user,
			Password:       password,
			AccessToken:    accessToken,
		}
		agentcommon.NormalizeArtifactoryUrl(details)
		return details, nil
	}
	// No server flags at all - same fallback as GetSpecificConfig("", true, true).
	return config.GetSpecificConfig("", true, true)
}
