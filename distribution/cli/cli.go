package cli

import (
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/cmddefs"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	distributionCommands "github.com/jfrog/jfrog-cli-artifactory/distribution/commands"
	"github.com/jfrog/jfrog-cli-artifactory/distribution/docs/releasebundlecreate"
	"github.com/jfrog/jfrog-cli-artifactory/distribution/docs/releasebundledelete"
	"github.com/jfrog/jfrog-cli-artifactory/distribution/docs/releasebundledistribute"
	"github.com/jfrog/jfrog-cli-artifactory/distribution/docs/releasebundlesign"
	"github.com/jfrog/jfrog-cli-artifactory/distribution/docs/releasebundleupdate"
	"github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	buildInfoSummary "github.com/jfrog/jfrog-cli-core/v2/common/cliutils/summary"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	distributionServices "github.com/jfrog/jfrog-client-go/distribution/services"
	distributionServicesUtils "github.com/jfrog/jfrog-client-go/distribution/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"os"
	"path/filepath"
	"strings"
)

const distributionCategory = "distribution"

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "release-bundle-create",
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleV1Create),
			Aliases:     []string{"rbc"},
			Description: releasebundlecreate.GetDescription(),
			Arguments:   releasebundlecreate.GetArguments(),
			Category:    distributionCategory,
			Action:      releaseBundleCreateCmd,
		},
		{
			Name:        "release-bundle-update",
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleV1Update),
			Aliases:     []string{"rbu"},
			Description: releasebundleupdate.GetDescription(),
			Arguments:   releasebundleupdate.GetArguments(),
			Category:    distributionCategory,
			Action:      releaseBundleUpdateCmd,
		},
		{
			Name:        "release-bundle-sign",
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleV1Sign),
			Aliases:     []string{"rbs"},
			Description: releasebundlesign.GetDescription(),
			Arguments:   releasebundlesign.GetArguments(),
			Category:    distributionCategory,
			Action:      releaseBundleSignCmd,
		},
		{
			Name:        "release-bundle-distribute",
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleV1Distribute),
			Aliases:     []string{"rbd"},
			Description: releasebundledistribute.GetDescription(),
			Arguments:   releasebundledistribute.GetArguments(),
			Category:    distributionCategory,
			Action:      releaseBundleDistributeCmd,
		},
		{
			Name:        "release-bundle-delete",
			Flags:       flagkit.GetCommandFlags(cmddefs.ReleaseBundleV1Delete),
			Aliases:     []string{"rbdel"},
			Description: releasebundledelete.GetDescription(),
			Arguments:   releasebundledelete.GetArguments(),
			Category:    distributionCategory,
			Action:      releaseBundleDeleteCmd,
		},
	}
}

func releaseBundleCreateCmd(c *components.Context) error {
	if !(len(c.Arguments) == 2 && c.IsFlagSet("spec") || (len(c.Arguments) == 3 && !c.IsFlagSet("spec"))) {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}
	if c.GetBoolFlagValue("detailed-summary") && !c.GetBoolFlagValue("sign") {
		return pluginsCommon.PrintHelpAndReturnError("The --detailed-summary option can't be used without --sign", c)
	}
	var releaseBundleCreateSpec *spec.SpecFiles
	var err error
	if c.IsFlagSet("spec") {
		releaseBundleCreateSpec, err = cliutils.GetSpec(c, true, true)
	} else {
		releaseBundleCreateSpec = createDefaultReleaseBundleSpec(c)
	}
	if err != nil {
		return err
	}
	err = spec.ValidateSpec(releaseBundleCreateSpec.Files, false, true)
	if err != nil {
		return err
	}

	params, err := createReleaseBundleCreateUpdateParams(c, c.Arguments[0], c.Arguments[1])
	if err != nil {
		return err
	}
	releaseBundleCreateCmd := distributionCommands.NewReleaseBundleCreateCommand()
	dsDetails, err := createDistributionDetailsByFlags(c)
	if err != nil {
		return err
	}
	releaseBundleCreateCmd.SetServerDetails(dsDetails).SetReleaseBundleCreateParams(params).SetSpec(releaseBundleCreateSpec).SetDryRun(c.GetBoolFlagValue("dry-run")).SetDetailedSummary(c.GetBoolFlagValue("detailed-summary"))

	err = commands.Exec(releaseBundleCreateCmd)
	if releaseBundleCreateCmd.IsDetailedSummary() {
		if summary := releaseBundleCreateCmd.GetSummary(); summary != nil {
			return buildInfoSummary.PrintBuildInfoSummaryReport(summary.IsSucceeded(), summary.GetSha256(), err)
		}
	}
	return err
}

func releaseBundleUpdateCmd(c *components.Context) error {
	if !(len(c.Arguments) == 2 && c.IsFlagSet("spec") || (len(c.Arguments) == 3 && !c.IsFlagSet("spec"))) {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}
	if c.GetBoolFlagValue("detailed-summary") && !c.GetBoolFlagValue("sign") {
		return pluginsCommon.PrintHelpAndReturnError("The --detailed-summary option can't be used without --sign", c)
	}
	var releaseBundleUpdateSpec *spec.SpecFiles
	var err error
	if c.IsFlagSet("spec") {
		releaseBundleUpdateSpec, err = cliutils.GetSpec(c, true, true)
	} else {
		releaseBundleUpdateSpec = createDefaultReleaseBundleSpec(c)
	}
	if err != nil {
		return err
	}
	err = spec.ValidateSpec(releaseBundleUpdateSpec.Files, false, true)
	if err != nil {
		return err
	}

	params, err := createReleaseBundleCreateUpdateParams(c, c.Arguments[0], c.Arguments[1])
	if err != nil {
		return err
	}
	releaseBundleUpdateCmd := distributionCommands.NewReleaseBundleUpdateCommand()
	dsDetails, err := createDistributionDetailsByFlags(c)
	if err != nil {
		return err
	}
	releaseBundleUpdateCmd.SetServerDetails(dsDetails).SetReleaseBundleUpdateParams(params).SetSpec(releaseBundleUpdateSpec).SetDryRun(c.GetBoolFlagValue("dry-run")).SetDetailedSummary(c.GetBoolFlagValue("detailed-summary"))

	err = commands.Exec(releaseBundleUpdateCmd)
	if releaseBundleUpdateCmd.IsDetailedSummary() {
		if updateBundleCmdSummary := releaseBundleUpdateCmd.GetSummary(); updateBundleCmdSummary != nil {
			return buildInfoSummary.PrintBuildInfoSummaryReport(updateBundleCmdSummary.IsSucceeded(), updateBundleCmdSummary.GetSha256(), err)
		}
	}
	return err
}

func releaseBundleSignCmd(c *components.Context) error {
	if len(c.Arguments) != 2 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	params := distributionServices.NewSignBundleParams(c.Arguments[0], c.Arguments[1])
	params.StoringRepository = c.GetStringFlagValue("repo")
	params.GpgPassphrase = c.GetStringFlagValue("passphrase")
	releaseBundleSignCmd := distributionCommands.NewReleaseBundleSignCommand()
	dsDetails, err := createDistributionDetailsByFlags(c)
	if err != nil {
		return err
	}
	releaseBundleSignCmd.SetServerDetails(dsDetails).SetReleaseBundleSignParams(params).SetDetailedSummary(c.GetBoolFlagValue("detailed-summary"))
	err = commands.Exec(releaseBundleSignCmd)
	if releaseBundleSignCmd.IsDetailedSummary() {
		if summary := releaseBundleSignCmd.GetSummary(); summary != nil {
			return buildInfoSummary.PrintBuildInfoSummaryReport(summary.IsSucceeded(), summary.GetSha256(), err)
		}
	}
	return err
}

func releaseBundleDistributeCmd(c *components.Context) error {
	if err := ValidateReleaseBundleDistributeCmd(c); err != nil {
		return err
	}

	dsDetails, err := createDistributionDetailsByFlags(c)
	if err != nil {
		return err
	}
	distributionRules, maxWaitMinutes, params, err := InitReleaseBundleDistributeCmd(c)
	if err != nil {
		return err
	}

	distributeCmd := distributionCommands.NewReleaseBundleDistributeV1Command()
	distributeCmd.SetServerDetails(dsDetails).
		SetDistributeBundleParams(params).
		SetDistributionRules(distributionRules).
		SetDryRun(c.GetBoolFlagValue("dry-run")).
		SetSync(c.GetBoolFlagValue("sync")).
		SetMaxWaitMinutes(maxWaitMinutes).
		SetAutoCreateRepo(c.GetBoolFlagValue("create-repo"))

	return commands.Exec(distributeCmd)
}

func releaseBundleDeleteCmd(c *components.Context) error {
	if len(c.Arguments) != 2 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}
	var distributionRules *spec.DistributionRules
	if c.IsFlagSet("dist-rules") {
		if c.IsFlagSet("site") || c.IsFlagSet("city") || c.IsFlagSet("country-code") {
			return pluginsCommon.PrintHelpAndReturnError("flag --dist-rules can't be used with --site, --city or --country-code", c)
		}
		var err error
		distributionRules, err = spec.CreateDistributionRulesFromFile(c.GetStringFlagValue("dist-rules"))
		if err != nil {
			return err
		}
	} else {
		distributionRules = CreateDefaultDistributionRules(c)
	}

	params := distributionServices.NewDeleteReleaseBundleParams(c.Arguments[0], c.Arguments[1])
	params.DeleteFromDistribution = c.GetBoolFlagValue("delete-from-dist")
	params.Sync = c.GetBoolFlagValue("sync")
	maxWaitMinutes, err := c.WithDefaultIntFlagValue("max-wait-minutes", 60)
	if err != nil {
		return err
	}
	params.MaxWaitMinutes = maxWaitMinutes
	distributeBundleCmd := distributionCommands.NewReleaseBundleDeleteParams()
	dsDetails, err := createDistributionDetailsByFlags(c)
	if err != nil {
		return err
	}
	distributeBundleCmd.SetQuiet(pluginsCommon.GetQuietValue(c)).SetServerDetails(dsDetails).SetDistributeBundleParams(params).SetDistributionRules(distributionRules).SetDryRun(c.GetBoolFlagValue("dry-run"))

	return commands.Exec(distributeBundleCmd)
}

func createDefaultReleaseBundleSpec(c *components.Context) *spec.SpecFiles {
	return spec.NewBuilder().
		Pattern(c.Arguments[2]).
		Target(c.GetStringFlagValue("target")).
		Props(c.GetStringFlagValue("props")).
		Build(c.GetStringFlagValue("build")).
		Bundle(c.GetStringFlagValue("bundle")).
		Exclusions(pluginsCommon.GetStringsArrFlagValue(c, "exclusions")).
		Regexp(c.GetBoolFlagValue("regexp")).
		TargetProps(c.GetStringFlagValue("target-props")).
		Ant(c.GetBoolFlagValue("ant")).
		BuildSpec()
}

func createReleaseBundleCreateUpdateParams(c *components.Context, bundleName, bundleVersion string) (distributionServicesUtils.ReleaseBundleParams, error) {
	releaseBundleParams := distributionServicesUtils.NewReleaseBundleParams(bundleName, bundleVersion)
	releaseBundleParams.SignImmediately = c.GetBoolFlagValue("sign")
	releaseBundleParams.StoringRepository = c.GetStringFlagValue("repo")
	releaseBundleParams.GpgPassphrase = c.GetStringFlagValue("passphrase")
	releaseBundleParams.Description = c.GetStringFlagValue("desc")
	if c.IsFlagSet("release-notes-path") {
		bytes, err := os.ReadFile(c.GetStringFlagValue("release-notes-path"))
		if err != nil {
			return releaseBundleParams, errorutils.CheckError(err)
		}
		releaseBundleParams.ReleaseNotes = string(bytes)
		releaseBundleParams.ReleaseNotesSyntax, err = populateReleaseNotesSyntax(c)
		if err != nil {
			return releaseBundleParams, err
		}
	}
	return releaseBundleParams, nil
}

func createDistributionDetailsByFlags(c *components.Context) (*coreConfig.ServerDetails, error) {
	dsDetails, err := pluginsCommon.CreateServerDetailsWithConfigOffer(c, true, cliutils.Ds)
	if err != nil {
		return nil, err
	}
	if dsDetails.DistributionUrl == "" {
		return nil, errors.New("no JFrog Distribution URL specified, either via the --url flag or as part of the server configuration")
	}
	return dsDetails, nil
}

func populateReleaseNotesSyntax(c *components.Context) (distributionServicesUtils.ReleaseNotesSyntax, error) {
	// If release notes syntax is set, use it
	releaseNotesSyntax := c.GetStringFlagValue("release-notes-syntax")
	if releaseNotesSyntax != "" {
		switch releaseNotesSyntax {
		case "markdown":
			return distributionServicesUtils.Markdown, nil
		case "asciidoc":
			return distributionServicesUtils.Asciidoc, nil
		case "plain_text":
			return distributionServicesUtils.PlainText, nil
		default:
			return distributionServicesUtils.PlainText, errorutils.CheckErrorf("--release-notes-syntax must be one of: markdown, asciidoc or plain_text.")
		}
	}
	// If the file extension is ".md" or ".markdown", use the Markdown syntax
	extension := strings.ToLower(filepath.Ext(c.GetStringFlagValue("release-notes-path")))
	if extension == ".md" || extension == ".markdown" {
		return distributionServicesUtils.Markdown, nil
	}
	return distributionServicesUtils.PlainText, nil
}
