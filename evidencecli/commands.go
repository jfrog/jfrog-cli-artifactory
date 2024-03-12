package evidencecli

import (
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecli/docs/createevidence"
	verify "github.com/jfrog/jfrog-cli-artifactory/evidencecli/docs/verifyevidence"
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
			Flags:       GetCommandFlags(CreateEvidence),
			Description: attest.GetDescription(),
			Arguments:   attest.GetArguments(),
			Action:      createEvidence,
		},
		{
			Name:        "verify-evidence",
			Aliases:     []string{"verify", "v"},
			Flags:       GetCommandFlags(VerifyEvidence),
			Description: verify.GetDescription(),
			Arguments:   verify.GetArguments(),
			Action:      verifyEvidence,
		},
	}
}

func platformToEvidenceUrls(evdDetails *coreConfig.ServerDetails) {
	evdDetails.ArtifactoryUrl = utils.AddTrailingSlashIfNeeded(evdDetails.Url) + "artifactory/"
	evdDetails.LifecycleUrl = utils.AddTrailingSlashIfNeeded(evdDetails.Url) + "lifecycle/"
	evdDetails.Url = ""
}

func createEvidence(c *components.Context) error {
	if err := validateCreateEvidenceContext(c); err != nil {
		return err
	}

	artifactoryClient, err := evidenceDetailsByFlags(c)
	if err != nil {
		return err
	}

	createCmd := evidencecore.NewEvidenceCreateCommand().SetServerDetails(artifactoryClient).SetPredicateFilePath(c.GetStringFlagValue(EvdPredicate)).
		SetPredicateType(c.GetStringFlagValue(EvdPredicateType)).SetSubjects(c.GetStringFlagValue(EvdSubjects)).SetKey(c.GetStringFlagValue(EvdKey)).
		SetKeyId(c.GetStringFlagValue(EvdKeyId)).SetEvidenceName(c.GetStringFlagValue(EvdName)).SetOverride(c.GetBoolFlagValue(EvdOverride))
	return commands.Exec(createCmd)
}

func verifyEvidence(c *components.Context) error {
	if err := validateVerifyEvidenceContext(c); err != nil {
		return err
	}

	artifactoryClient, err := evidenceDetailsByFlags(c)
	if err != nil {
		return err
	}

	verifyCmd := evidencecore.NewEvidenceVerifyCommand().SetServerDetails(artifactoryClient).SetKey(c.GetStringFlagValue(EvdKey)).SetEvidenceName(c.GetStringFlagValue(EvdName))
	return commands.Exec(verifyCmd)
}

func validateVerifyEvidenceContext(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}
	if !c.IsFlagSet(EvdKey) || assertValueProvided(c, EvdKey) != nil {
		return errorutils.CheckErrorf("'key' is a mandatory field for creating a custom evidence: --%s", EvdKey)
	}
	if !c.IsFlagSet(EvdName) || assertValueProvided(c, EvdName) != nil {
		return errorutils.CheckErrorf("'key' is a mandatory field for creating a custom evidence: --%s", EvdName)
	}

	return nil
}

func evidenceDetailsByFlags(c *components.Context) (*coreConfig.ServerDetails, error) {
	artifactoryClient, err := pluginsCommon.CreateServerDetailsWithConfigOffer(c, true, commonCliUtils.Platform)
	if err != nil {
		return nil, err
	}
	if artifactoryClient.Url == "" {
		return nil, errors.New("platform URL is mandatory for evidence commands")
	}
	platformToEvidenceUrls(artifactoryClient)
	return artifactoryClient, nil
}

func validateCreateEvidenceContext(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) > 1 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	if !c.IsFlagSet(EvdPredicate) || assertValueProvided(c, EvdPredicate) != nil {
		return errorutils.CheckErrorf("'predicate' is a mandatory field for creating a custom evidence: --%s", EvdPredicate)
	}
	if !c.IsFlagSet(EvdPredicateType) || assertValueProvided(c, EvdPredicateType) != nil {
		return errorutils.CheckErrorf("'predicate' is a mandatory field for creating a custom evidence: --%s", EvdPredicateType)
	}
	if !c.IsFlagSet(EvdSubjects) || assertValueProvided(c, EvdSubjects) != nil {
		return errorutils.CheckErrorf("'subjects' is a mandatory field for creating a custom evidence: --%s", EvdSubjects)
	}
	if !c.IsFlagSet(EvdKey) || assertValueProvided(c, EvdKey) != nil {
		return errorutils.CheckErrorf("'key' is a mandatory field for creating a custom evidence: --%s", EvdKey)
	}

	return nil
}

func assertValueProvided(c *components.Context, fieldName string) error {
	if c.GetStringFlagValue(fieldName) == "" {
		return errorutils.CheckErrorf("the --%s option is mandatory", fieldName)
	}
	return nil
}
