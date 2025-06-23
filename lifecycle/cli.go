package lifecycle

import (
	"errors"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/cmddefs"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/distribution"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	lifecycle "github.com/jfrog/jfrog-cli-artifactory/lifecycle/commands"
	rbAnnotate "github.com/jfrog/jfrog-cli-artifactory/lifecycle/docs/annotate"
	rbCreate "github.com/jfrog/jfrog-cli-artifactory/lifecycle/docs/create"
	rbDeleteLocal "github.com/jfrog/jfrog-cli-artifactory/lifecycle/docs/deletelocal"
	rbDeleteRemote "github.com/jfrog/jfrog-cli-artifactory/lifecycle/docs/deleteremote"
	rbDistribute "github.com/jfrog/jfrog-cli-artifactory/lifecycle/docs/distribute"
	rbExport "github.com/jfrog/jfrog-cli-artifactory/lifecycle/docs/export"
	rbImport "github.com/jfrog/jfrog-cli-artifactory/lifecycle/docs/importbundle"
	rbPromote "github.com/jfrog/jfrog-cli-artifactory/lifecycle/docs/promote"
	artifactoryUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	speccore "github.com/jfrog/jfrog-cli-core/v2/common/spec"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	artClientUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"os"
	"strconv"
	"strings"
)

const (
	lcCategory                                 = "Lifecycle"
	minArtifactoryVersionForMultiSourceSupport = "7.114.0"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        cmddefs.ReleaseBundleCreate,
			Aliases:     []string{"rbc"},
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleCreate),
			Description: rbCreate.GetDescription(),
			Arguments:   rbCreate.GetArguments(),
			Category:    lcCategory,
			Action:      create,
		},
		{
			Name:        "release-bundle-promote",
			Aliases:     []string{"rbp"},
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundlePromote),
			Description: rbPromote.GetDescription(),
			Arguments:   rbPromote.GetArguments(),
			Category:    lcCategory,
			Action:      promote,
		},
		{
			Name:        "release-bundle-distribute",
			Aliases:     []string{"rbd"},
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleDistribute),
			Description: rbDistribute.GetDescription(),
			Arguments:   rbDistribute.GetArguments(),
			Category:    lcCategory,
			Action:      distribute,
		},
		{
			Name:        "release-bundle-export",
			Aliases:     []string{"rbe"},
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleExport),
			Description: rbExport.GetDescription(),
			Arguments:   rbExport.GetArguments(),
			Category:    lcCategory,
			Action:      export,
		},
		{
			Name:        "release-bundle-delete-local",
			Aliases:     []string{"rbdell"},
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleDeleteLocal),
			Description: rbDeleteLocal.GetDescription(),
			Arguments:   rbDeleteLocal.GetArguments(),
			Category:    lcCategory,
			Action:      deleteLocal,
		},
		{
			Name:        "release-bundle-delete-remote",
			Aliases:     []string{"rbdelr"},
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleDeleteRemote),
			Description: rbDeleteRemote.GetDescription(),
			Arguments:   rbDeleteLocal.GetArguments(),
			Category:    lcCategory,
			Action:      deleteRemote,
		},
		{
			Name:        "release-bundle-import",
			Aliases:     []string{"rbi"},
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleImport),
			Description: rbImport.GetDescription(),
			Arguments:   rbImport.GetArguments(),
			Category:    lcCategory,
			Action:      releaseBundleImport,
		},
		{
			Name:        "release-bundle-annotate",
			Aliases:     []string{"rba"},
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleAnnotate),
			Description: rbAnnotate.GetDescription(),
			Arguments:   rbAnnotate.GetArguments(),
			Category:    lcCategory,
			Action:      annotate,
		},
	}
}

func validateCreateReleaseBundleContext(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) != 2 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	return assertValidCreationMethod(c)
}

func assertValidCreationMethod(c *components.Context) error {
	// Determine the methods provided
	monoReleaseBundleSource := []bool{
		c.IsFlagSet("spec"),
		c.IsFlagSet(flagkit.Builds),
		c.IsFlagSet(flagkit.ReleaseBundles),
	}
	methodCount := coreutils.SumTrueValues(monoReleaseBundleSource)

	multiReleaseBundleSources := []bool{
		c.IsFlagSet(flagkit.SourceTypeReleaseBundles),
		c.IsFlagSet(flagkit.SourceTypeBuilds),
	}

	multiReleaseBundleSourcesCount := coreutils.SumTrueValues(multiReleaseBundleSources)

	return validateCreationMethods(c, methodCount, multiReleaseBundleSourcesCount)
}

func validateRegularMethods(c *components.Context, methodCount int) error {
	if err := validateSingleCreationMethod(methodCount); err != nil {
		return err
	}

	if err := validateCreationValuesPresence(c, methodCount); err != nil {
		return err
	}
	return nil
}

func validateCreationMethods(c *components.Context, regularMethodsCount int, multiSrcMethodsCount int) error {
	if multiSrcMethodsCount > 0 {
		if err := multipleSourcesSupported(c); err != nil {
			return err
		}
		if regularMethodsCount > 0 {
			errMsg := fmt.Sprintf("only multiple sources must be supplied: --%s, --%s,\n"+
				"or one of: --%s, --%s or --%s",
				flagkit.SourceTypeReleaseBundles, flagkit.SourceTypeBuilds,
				"spec", flagkit.Builds, flagkit.ReleaseBundles)
			return errorutils.CheckError(errors.New(errMsg))
		}
		return nil
	}
	return validateRegularMethods(c, regularMethodsCount)
}

func validateSingleCreationMethod(methodCount int) error {
	if methodCount > 1 {
		return errorutils.CheckErrorf(
			"exactly one creation source must be supplied: --%s, --%s, or --%s.\n"+
				"Opt to use the --%s option as the --%s and --%s are deprecated",
			"spec", flagkit.Builds, flagkit.ReleaseBundles,
			"spec", flagkit.Builds, flagkit.ReleaseBundles,
		)
	}
	return nil
}

func validateCreationValuesPresence(c *components.Context, methodCount int) error {
	if methodCount == 0 {
		if !areBuildFlagsSet(c) && !areBuildEnvVarsSet() {
			return errorutils.CheckErrorf("Either --build-name or JFROG_CLI_BUILD_NAME, and --build-number or JFROG_CLI_BUILD_NUMBER must be defined")
		}
	}
	return nil
}

// areBuildFlagsSet checks if build-name or build-number flags are set.
func areBuildFlagsSet(c *components.Context) bool {
	return c.IsFlagSet(flagkit.BuildName) || c.IsFlagSet(flagkit.BuildNumber)
}

// areBuildEnvVarsSet checks if build environment variables are set.
func areBuildEnvVarsSet() bool {
	return os.Getenv("JFROG_CLI_BUILD_NUMBER") != "" && os.Getenv("JFROG_CLI_BUILD_NAME") != ""
}

func create(c *components.Context) (err error) {
	if err = validateCreateReleaseBundleContext(c); err != nil {
		return err
	}
	creationSpec, err := getReleaseBundleCreationSpec(c)
	if err != nil {
		return
	}
	lcDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return
	}
	createCmd := lifecycle.NewReleaseBundleCreateCommand().SetServerDetails(lcDetails).SetReleaseBundleName(c.GetArgumentAt(0)).
		SetReleaseBundleVersion(c.GetArgumentAt(1)).SetSigningKeyName(c.GetStringFlagValue(flagkit.SigningKey)).
		SetSync(c.GetBoolFlagValue(flagkit.Sync)).
		SetReleaseBundleProject(pluginsCommon.GetProject(c)).SetSpec(creationSpec).
		SetBuildsSpecPath(c.GetStringFlagValue(flagkit.Builds)).SetReleaseBundlesSpecPath(c.GetStringFlagValue(flagkit.ReleaseBundles))

	err = lifecycle.ValidateFeatureSupportedVersion(lcDetails, minArtifactoryVersionForMultiSourceSupport)
	// err == nil means new flags are supported and may be added to createCmd
	if err == nil {
		createCmd.SetReleaseBundlesSources(c.GetStringFlagValue(flagkit.SourceTypeReleaseBundles)).
			SetBuildsSources(c.GetStringFlagValue(flagkit.SourceTypeBuilds))
	}

	return commands.Exec(createCmd)
}

// the function validates that the current artifactory version supports multiple source feature
func multipleSourcesSupported(c *components.Context) error {
	lcDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return err
	}

	return lifecycle.ValidateFeatureSupportedVersion(lcDetails, minArtifactoryVersionForMultiSourceSupport)
}

func getReleaseBundleCreationSpec(c *components.Context) (*spec.SpecFiles, error) {
	// Checking if the "builds" or "release-bundles" flags are set - if so, the spec flag should be ignored
	if c.IsFlagSet(flagkit.Builds) || c.IsFlagSet(flagkit.ReleaseBundles) {
		return nil, nil
	}

	if c.IsFlagSet(flagkit.SourceTypeReleaseBundles) || c.IsFlagSet(flagkit.SourceTypeBuilds) {
		return nil, nil
	}

	// Check if the "spec" flag is set - if so, return the spec
	if c.IsFlagSet("spec") {
		return cliutils.GetSpec(c, true, false)
	}

	// Else - create a spec from the buildName and buildnumber flags or env vars
	buildName := getStringFlagOrEnv(c, flagkit.BuildName, coreutils.BuildName)
	buildNumber := getStringFlagOrEnv(c, flagkit.BuildNumber, coreutils.BuildNumber)

	if buildName != "" && buildNumber != "" {
		return speccore.CreateSpecFromBuildNameNumberAndProject(buildName, buildNumber, pluginsCommon.GetProject(c))
	}

	return nil, fmt.Errorf("either the --spec flag must be provided, " +
		"or both --build-name and --build-number flags (or their corresponding environment variables " +
		"JFROG_CLI_BUILD_NAME and JFROG_CLI_BUILD_NUMBER) must be set")
}

func getStringFlagOrEnv(c *components.Context, flag string, envVar string) string {
	if c.IsFlagSet(flag) {
		return c.GetStringFlagValue(flag)
	}
	return os.Getenv(envVar)
}

func promote(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) != 3 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	lcDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return err
	}

	promoteCmd := lifecycle.NewReleaseBundlePromoteCommand().SetServerDetails(lcDetails).SetReleaseBundleName(c.GetArgumentAt(0)).
		SetReleaseBundleVersion(c.GetArgumentAt(1)).SetEnvironment(c.GetArgumentAt(2)).SetSigningKeyName(c.GetStringFlagValue(flagkit.SigningKey)).
		SetSync(c.GetBoolFlagValue(flagkit.Sync)).SetReleaseBundleProject(pluginsCommon.GetProject(c)).
		SetIncludeReposPatterns(splitRepos(c, flagkit.IncludeRepos)).SetExcludeReposPatterns(splitRepos(c, flagkit.ExcludeRepos)).
		SetPromotionType(c.GetStringFlagValue(flagkit.PromotionType))
	return commands.Exec(promoteCmd)
}

func distribute(c *components.Context) error {
	if err := validateDistributeCommand(c); err != nil {
		return err
	}

	lcDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return err
	}
	distributionRules, maxWaitMinutes, _, err := distribution.InitReleaseBundleDistributeCmd(c)
	if err != nil {
		return err
	}

	distributeCmd := lifecycle.NewReleaseBundleDistributeCommand()
	distributeCmd.SetServerDetails(lcDetails).
		SetReleaseBundleName(c.GetArgumentAt(0)).
		SetReleaseBundleVersion(c.GetArgumentAt(1)).
		SetReleaseBundleProject(pluginsCommon.GetProject(c)).
		SetDistributionRules(distributionRules).
		SetDryRun(c.GetBoolFlagValue("dry-run")).
		SetAutoCreateRepo(c.GetBoolFlagValue(flagkit.CreateRepo)).
		SetPathMappingPattern(c.GetStringFlagValue(flagkit.PathMappingPattern)).
		SetPathMappingTarget(c.GetStringFlagValue(flagkit.PathMappingTarget)).
		SetSync(c.GetBoolFlagValue(flagkit.Sync)).
		SetMaxWaitMinutes(maxWaitMinutes)
	return commands.Exec(distributeCmd)
}

func deleteLocal(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) != 2 && len(c.Arguments) != 3 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	lcDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return err
	}

	environment := ""
	if len(c.Arguments) == 3 {
		environment = c.GetArgumentAt(2)
	}

	deleteCmd := lifecycle.NewReleaseBundleDeleteCommand().
		SetServerDetails(lcDetails).
		SetReleaseBundleName(c.GetArgumentAt(0)).
		SetReleaseBundleVersion(c.GetArgumentAt(1)).
		SetEnvironment(environment).
		SetQuiet(pluginsCommon.GetQuietValue(c)).
		SetReleaseBundleProject(pluginsCommon.GetProject(c)).
		SetSync(c.GetBoolFlagValue(flagkit.Sync))
	return commands.Exec(deleteCmd)
}

func deleteRemote(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) != 2 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	lcDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return err
	}

	distributionRules, maxWaitMinutes, _, err := distribution.InitReleaseBundleDistributeCmd(c)
	if err != nil {
		return err
	}

	deleteCmd := lifecycle.NewReleaseBundleRemoteDeleteCommand().
		SetServerDetails(lcDetails).
		SetReleaseBundleName(c.GetArgumentAt(0)).
		SetReleaseBundleVersion(c.GetArgumentAt(1)).
		SetDistributionRules(distributionRules).
		SetDryRun(c.GetBoolFlagValue("dry-run")).
		SetMaxWaitMinutes(maxWaitMinutes).
		SetQuiet(pluginsCommon.GetQuietValue(c)).
		SetReleaseBundleProject(pluginsCommon.GetProject(c)).
		SetSync(c.GetBoolFlagValue(flagkit.Sync))
	return commands.Exec(deleteCmd)
}

func export(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) < 2 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}
	lcDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return err
	}
	exportCmd, modifications := initReleaseBundleExportCmd(c)
	downloadConfig, err := CreateDownloadConfiguration(c)
	if err != nil {
		return err
	}
	exportCmd.
		SetServerDetails(lcDetails).
		SetReleaseBundleExportModifications(modifications).
		SetDownloadConfiguration(*downloadConfig)

	return commands.Exec(exportCmd)
}

func releaseBundleImport(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) != 1 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	rtDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return err
	}
	importCmd := lifecycle.NewReleaseBundleImportCommand()
	if err != nil {
		return err
	}
	importCmd.
		SetServerDetails(rtDetails).
		SetFilepath(c.GetArgumentAt(0))

	return commands.Exec(importCmd)
}

func annotate(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if c.GetNumberOfArgs() < 2 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	rtDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return err
	}
	annotateCmd := lifecycle.NewReleaseBundleAnnotateCommand()

	project := pluginsCommon.GetProject(c)
	if project == "" {
		project = "default"
	}

	tagExist := c.IsFlagSet(flagkit.Tag)
	propsExist := c.IsFlagSet(flagkit.Properties)
	deleteProps := c.IsFlagSet(flagkit.DeleteProperty)

	if !tagExist && !propsExist && !deleteProps {
		return errors.New("action is not specified. One of tag/properties/del-prop should be specified")
	}

	annotateCmd.
		SetServerDetails(rtDetails).
		SetReleaseBundleProject(project).
		SetReleaseBundleName(c.GetArgumentAt(0)).
		SetReleaseBundleVersion(c.GetArgumentAt(1)).
		SetTag(c.GetStringFlagValue(flagkit.Tag), tagExist).
		SetProps(c.GetStringFlagValue(flagkit.Properties)).
		DeleteProps(c.GetStringFlagValue(flagkit.DeleteProperty)).
		SetRecursive(c.GetBoolFlagValue(flagkit.Recursive), c.IsFlagSet(flagkit.Recursive))
	return commands.Exec(annotateCmd)
}

func validateDistributeCommand(c *components.Context) error {
	if err := distribution.ValidateReleaseBundleDistributeCmd(c); err != nil {
		return err
	}

	mappingPatternProvided := c.IsFlagSet(flagkit.PathMappingPattern)
	mappingTargetProvided := c.IsFlagSet(flagkit.PathMappingTarget)
	if (mappingPatternProvided && !mappingTargetProvided) ||
		(!mappingPatternProvided && mappingTargetProvided) {
		return errorutils.CheckErrorf("the options --%s and --%s must be provided together", flagkit.PathMappingPattern, flagkit.PathMappingTarget)
	}
	return nil
}

func createLifecycleDetailsByFlags(c *components.Context) (*coreConfig.ServerDetails, error) {
	lcDetails, err := pluginsCommon.CreateServerDetailsWithConfigOffer(c, true, commonCliUtils.Platform)
	if err != nil {
		return nil, err
	}
	if lcDetails.Url == "" {
		return nil, errors.New("platform URL is mandatory for lifecycle commands")
	}
	PlatformToLifecycleUrls(lcDetails)
	return lcDetails, nil
}

func splitRepos(c *components.Context, reposOptionKey string) []string {
	if c.IsFlagSet(reposOptionKey) {
		return strings.Split(c.GetStringFlagValue(reposOptionKey), ";")
	}
	return nil
}

func initReleaseBundleExportCmd(c *components.Context) (command *lifecycle.ReleaseBundleExportCommand, modifications services.Modifications) {
	command = lifecycle.NewReleaseBundleExportCommand().
		SetReleaseBundleName(c.GetArgumentAt(0)).
		SetReleaseBundleVersion(c.GetArgumentAt(1)).
		SetTargetPath(c.GetArgumentAt(2)).
		SetProject(c.GetStringFlagValue(flagkit.Project))

	modifications = services.Modifications{
		PathMappings: []artClientUtils.PathMapping{
			{
				Input:  c.GetStringFlagValue(flagkit.PathMappingPattern),
				Output: c.GetStringFlagValue(flagkit.PathMappingTarget),
			},
		},
	}
	return
}

func CreateDownloadConfiguration(c *components.Context) (downloadConfiguration *artifactoryUtils.DownloadConfiguration, err error) {
	downloadConfiguration = new(artifactoryUtils.DownloadConfiguration)
	downloadConfiguration.MinSplitSize, err = getMinSplit(c, flagkit.DownloadMinSplitKb)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.SplitCount, err = getSplitCount(c, flagkit.DownloadSplitCount, flagkit.DownloadMaxSplitCount)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.Threads, err = pluginsCommon.GetThreadsCount(c)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.SkipChecksum = c.GetBoolFlagValue("skip-checksum")
	downloadConfiguration.Symlink = true
	return
}

func getMinSplit(c *components.Context, defaultMinSplit int64) (minSplitSize int64, err error) {
	minSplitSize = defaultMinSplit
	if c.GetStringFlagValue(flagkit.MinSplit) != "" {
		minSplitSize, err = strconv.ParseInt(c.GetStringFlagValue(flagkit.MinSplit), 10, 64)
		if err != nil {
			err = errors.New("The '--min-split' option should have a numeric value. " + getDocumentationMessage())
			return 0, err
		}
	}

	return minSplitSize, nil
}

func getSplitCount(c *components.Context, defaultSplitCount, maxSplitCount int) (splitCount int, err error) {
	splitCount = defaultSplitCount
	err = nil
	if c.GetStringFlagValue("split-count") != "" {
		splitCount, err = strconv.Atoi(c.GetStringFlagValue("split-count"))
		if err != nil {
			err = errors.New("The '--split-count' option should have a numeric value. " + getDocumentationMessage())
		}
		if splitCount > maxSplitCount {
			err = errors.New("The '--split-count' option value is limited to a maximum of " + strconv.Itoa(maxSplitCount) + ".")
		}
		if splitCount < 0 {
			err = errors.New("the '--split-count' option cannot have a negative value")
		}
	}
	return
}

func getDocumentationMessage() string {
	return "You can read the documentation at " + coreutils.JFrogHelpUrl + "jfrog-cli"
}

func PlatformToLifecycleUrls(lcDetails *coreConfig.ServerDetails) {
	lcDetails.ArtifactoryUrl = utils.AddTrailingSlashIfNeeded(lcDetails.Url) + "artifactory/"
	lcDetails.LifecycleUrl = utils.AddTrailingSlashIfNeeded(lcDetails.Url) + "lifecycle/"
	lcDetails.Url = ""
}
