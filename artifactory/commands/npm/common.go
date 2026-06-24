package npm

import (
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type CommonArgs struct {
	repo               string
	buildConfiguration *build.BuildConfiguration
	npmArgs            []string
	serverDetails      *config.ServerDetails
	useNative          bool
	configFilePath     string
	executablePath     string
}

func (ca *CommonArgs) SetServerDetails(serverDetails *config.ServerDetails) *CommonArgs {
	ca.serverDetails = serverDetails
	return ca
}

func (ca *CommonArgs) SetNpmArgs(npmArgs []string) *CommonArgs {
	ca.npmArgs = npmArgs
	return ca
}

func (ca *CommonArgs) SetBuildConfiguration(buildConfiguration *build.BuildConfiguration) *CommonArgs {
	ca.buildConfiguration = buildConfiguration
	return ca
}

func (ca *CommonArgs) SetRepo(repo string) *CommonArgs {
	ca.repo = repo
	return ca
}

func (ca *CommonArgs) UseNative() bool {
	return ca.useNative
}

func (ca *CommonArgs) SetUseNative(useNpmRc bool) *CommonArgs {
	ca.useNative = useNpmRc
	return ca
}

func (ca *CommonArgs) SetConfigFilePath(configFilePath string) *CommonArgs {
	ca.configFilePath = configFilePath
	return ca
}

func (ca *CommonArgs) SetExecutablePath(executablePath string) *CommonArgs {
	ca.executablePath = executablePath
	return ca
}

// CheckIsNativeAndFetchFilteredArgs checks if native mode should be enabled.
// It first checks the JFROG_RUN_NATIVE environment variable (preferred),
// then falls back to the deprecated --run-native flag for backward compatibility.
// Returns: useNative flag, filtered args (with --run-native removed if present), error
func CheckIsNativeAndFetchFilteredArgs(args []string) (useNative bool, filteredArgs []string, err error) {
	// Always strip --run-native from args so it never reaches the npm binary,
	// regardless of whether native mode is triggered by env var or flag.
	filteredArgs, useNativeFlag, err := coreutils.ExtractUseNativeFromArgs(args)
	if err != nil {
		return false, args, err
	}

	if flexpack.IsFlexPackEnabled() {
		log.Info("Running npm in native mode (JFROG_RUN_NATIVE=true)")
		useNative = true
	} else if useNativeFlag {
		log.Warn("The --run-native flag is deprecated. Please use JFROG_RUN_NATIVE=true environment variable instead.")
		log.Info("Running npm in native mode")
		useNative = true
	}
	return
}
