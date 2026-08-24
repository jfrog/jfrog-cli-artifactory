package apmcommon

import (
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

// ApmSubcommandOptions holds jf's own flags, manually extracted from a SkipFlagParsing
// subcommand's raw arguments, plus whatever args remained afterward.
type ApmSubcommandOptions struct {
	// ApmNativeArgs is what's left after stripping jf's own flags - passed straight through
	// to the real apm binary, so apm-native flags (--package, --registry, --zip, --dry-run,
	// --repo, etc.) survive untouched.
	ApmNativeArgs []string
	BuildConfig   *buildUtils.BuildConfiguration
	// ServerID is jf's own --server-id value, if provided. Empty means "use the default
	// configured server" (see agentcommon.GetServerDetailsByID).
	ServerID string
}

// ExtractApmSubcommandOptions extracts install/publish/update's own flags (--build-name,
// --build-number, --module, --project, --server-id) from args and resolves the build-info
// ones into a BuildConfiguration. Needed because those commands set SkipFlagParsing (so
// apm-native flags reach apm unrejected), which means urfave/cli parses none of jf's own
// flags either - they must be pulled out by hand.
func ExtractApmSubcommandOptions(args []string) (*ApmSubcommandOptions, error) {
	rest := args
	var buildName, buildNumber, module, project, serverID string
	var err error

	for _, opt := range []struct {
		name string
		dest *string
	}{
		{"build-name", &buildName},
		{"build-number", &buildNumber},
		{"module", &module},
		{"project", &project},
		{"server-id", &serverID},
	} {
		rest, *opt.dest, err = coreutils.ExtractStringOptionFromArgs(rest, opt.name)
		if err != nil {
			return nil, err
		}
	}

	buildConfig := new(buildUtils.BuildConfiguration)
	if err = buildConfig.SetBuildName(buildName).SetBuildNumber(buildNumber).SetProject(project).SetModule(module).ValidateBuildAndModuleParams(); err != nil {
		return nil, err
	}

	return &ApmSubcommandOptions{
		ApmNativeArgs: rest,
		BuildConfig:   buildConfig,
		ServerID:      serverID,
	}, nil
}
