package evidencecli

import (
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecli/docs"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecli/docs/createevidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecli/docs/verifyevidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecore"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "create-evidence",
			Aliases:     []string{"attest", "att"},
			Flags:       docs.GetCommandFlags(docs.CreateEvidence),
			Description: createevidence.GetDescription(),
			Arguments:   createevidence.GetArguments(),
			Action:      createEvidenceCmd,
		},
		{
			Name:        "verify-evidence",
			Aliases:     []string{"verify", "v"},
			Flags:       docs.GetCommandFlags(docs.VerifyEvidence),
			Description: verifyevidence.GetDescription(),
			Arguments:   verifyevidence.GetArguments(),
			Action:      verifyEvidenceCmd,
		},
	}
}

func PlatformToEvidenceUrls(evdDetails *coreConfig.ServerDetails) {
	evdDetails.ArtifactoryUrl = utils.AddTrailingSlashIfNeeded(evdDetails.Url) + "artifactory/"
	evdDetails.LifecycleUrl = utils.AddTrailingSlashIfNeeded(evdDetails.Url) + "lifecycle/"
	evdDetails.Url = ""
}

func createEvidenceCmd(c *components.Context) error {
	if err := validateCreateEvidenceContext(c); err != nil {
		return err
	}

	evdDetails, err := evidenceDetailsByFlags(c)
	if err != nil {
		return err
	}

	createCmd := evidencecore.NewEvidenceCreateCommand().SetServerDetails(evdDetails).SetPredicateFilePath(c.GetStringFlagValue(docs.EvdPredicate)).
		SetPredicateType(c.GetStringFlagValue(docs.EvdPredicateType)).SetSubjects(c.GetStringFlagValue(docs.EvdSubjects)).SetKey(c.GetStringFlagValue(docs.EvdKey)).
		SetKeyId(c.GetStringFlagValue(docs.EvdKeyId)).SetEvidenceName(c.GetStringFlagValue(docs.EvdName)).SetOverride(c.GetBoolFlagValue(docs.EvdOverride))
	return commands.Exec(createCmd)
}

func verifyEvidenceCmd(c *components.Context) error {
	if err := validateVerifyEvidenceContext(c); err != nil {
		return err
	}

	evdDetails, err := evidenceDetailsByFlags(c)
	if err != nil {
		return err
	}

	verifyCmd := evidencecore.NewEvidenceVerifyCommand().SetServerDetails(evdDetails).SetKey(c.GetStringFlagValue(docs.EvdKey)).SetEvidenceName(c.GetStringFlagValue(docs.EvdName))
	return commands.Exec(verifyCmd)
}

func validateVerifyEvidenceContext(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}
	if !c.IsFlagSet(docs.EvdKey) || assertValueProvided(c, docs.EvdKey) != nil {
		return errorutils.CheckErrorf("'key' is a mandatory fiels for creating a custom evidence: --%s", docs.EvdKey)
	}
	if !c.IsFlagSet(docs.EvdName) || assertValueProvided(c, docs.EvdName) != nil {
		return errorutils.CheckErrorf("'key' is a mandatory fiels for creating a custom evidence: --%s", docs.EvdName)
	}

	return nil
}

func evidenceDetailsByFlags(c *components.Context) (*coreConfig.ServerDetails, error) {
	evdDetails, err := createServerDetailsWithConfigOffer(c)
	if err != nil {
		return nil, err
	}
	if evdDetails.Url == "" {
		return nil, errors.New("platform URL is mandatory for evidence commands")
	}
	PlatformToEvidenceUrls(evdDetails)
	return evdDetails, nil
}

func validateCreateEvidenceContext(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) > 1 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	if !c.IsFlagSet(docs.EvdPredicate) || assertValueProvided(c, docs.EvdPredicate) != nil {
		return errorutils.CheckErrorf("'predicate' is a mandatory field for creating a custom evidence: --%s", docs.EvdPredicate)
	}
	if !c.IsFlagSet(docs.EvdPredicateType) || assertValueProvided(c, docs.EvdPredicateType) != nil {
		return errorutils.CheckErrorf("'predicate-type' is a mandatory field for creating a custom evidence: --%s", docs.EvdPredicateType)
	}
	if !c.IsFlagSet(docs.EvdSubjects) || assertValueProvided(c, docs.EvdSubjects) != nil {
		return errorutils.CheckErrorf("'subject' is a mandatory fiels for creating a custom evidence: --%s", docs.EvdSubjects)
	}
	if !c.IsFlagSet(docs.EvdKey) || assertValueProvided(c, docs.EvdKey) != nil {
		return errorutils.CheckErrorf("'key' is a mandatory fiels for creating a custom evidence: --%s", docs.EvdKey)
	}

	return nil
}

func assertValueProvided(c *components.Context, fieldName string) error {
	if c.GetStringFlagValue(fieldName) == "" {
		return errorutils.CheckErrorf("the --%s option is mandatory", fieldName)
	}
	return nil
}

func createServerDetailsWithConfigOffer(c *components.Context) (*coreConfig.ServerDetails, error) {
	return pluginsCommon.CreateServerDetailsWithConfigOffer(c, true, commonCliUtils.Platform)
}
