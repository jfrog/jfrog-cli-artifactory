package cli

import (
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cli/docs/create"
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
			Aliases:     []string{"create"},
			Flags:       GetCommandFlags(CreateEvidence),
			Description: create.GetDescription(),
			Arguments:   create.GetArguments(),
			Action:      createEvidence,
		},
	}
}

func platformToEvidenceUrls(rtDetails *coreConfig.ServerDetails) {
	rtDetails.ArtifactoryUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "artifactory/"
	rtDetails.EvidenceUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "evidence/"
}

func createEvidence(c *components.Context) error {
	if err := validateCreateEvidenceContext(c); err != nil {
		return err
	}

	artifactoryClient, err := evidenceDetailsByFlags(c)
	if err != nil {
		return err
	}

	createCmd := evidence.NewEvidenceCreateCommand().
		SetServerDetails(artifactoryClient).
		SetPredicateFilePath(c.GetStringFlagValue(EvdPredicate)).
		SetPredicateType(c.GetStringFlagValue(EvdPredicateType)).
		SetRepoPath(c.GetStringFlagValue(EvdRepoPath)).
		SetKey(c.GetStringFlagValue(EvdKey)).
		SetKeyId(c.GetStringFlagValue(EvdKeyId))
	return commands.Exec(createCmd)
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
	if !c.IsFlagSet(EvdRepoPath) || assertValueProvided(c, EvdRepoPath) != nil {
		return errorutils.CheckErrorf("'repo-path' is a mandatory field for creating a custom evidence: --%s", EvdRepoPath)
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
