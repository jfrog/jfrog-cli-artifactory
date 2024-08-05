package cli

import (
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cli/docs/create"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"strings"
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

var execFunc = func(command commands.Command) error {
	return commands.Exec(command)
}

func createEvidence(c *components.Context) error {
	if err := validateCreateEvidenceCommonContext(c); err != nil {
		return err
	}
	subject, err := getAndValidateSubject(c)
	if err != nil {
		return err
	}
	serverDetails, err := evidenceDetailsByFlags(c)
	if err != nil {
		return err
	}

	var command EvidenceCommands
	switch subject {
	case subjectRepoPath:
		command = NewEvidenceCustomCommand(c, execFunc)
	case releaseBundle:
		command = NewEvidenceReleaseBundleCommand(c, execFunc)
	case buildName:
		command = NewEvidenceBuildCommand(c, execFunc)
	default:
		return errors.New("unsupported subject")
	}

	return command.CreateEvidence(c, serverDetails)
}

func validateCreateEvidenceCommonContext(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}

	if len(c.Arguments) > 1 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}

	if !c.IsFlagSet(predicate) || assertValueProvided(c, predicate) != nil {
		return errorutils.CheckErrorf("'predicate' is a mandatory field for creating evidence: --%s", predicate)
	}
	if !c.IsFlagSet(predicateType) || assertValueProvided(c, predicateType) != nil {
		return errorutils.CheckErrorf("'predicate-type' is a mandatory field for creating evidence: --%s", predicateType)
	}
	if !c.IsFlagSet(key) || assertValueProvided(c, key) != nil {
		return errorutils.CheckErrorf("'key' is a mandatory field for creating evidence: --%s", key)
	}
	return nil
}

func getAndValidateSubject(c *components.Context) (string, error) {
	var foundSubjects []string
	for _, key := range subjectTypes {
		if c.GetStringFlagValue(key) != "" {
			foundSubjects = append(foundSubjects, key)
		}
	}

	if len(foundSubjects) == 0 {
		return "", errorutils.CheckErrorf("subject must be one of the fields: [%s]", strings.Join(subjectTypes, ", "))
	}
	if len(foundSubjects) > 1 {
		return "", errorutils.CheckErrorf("multiple subjects found: [%s]", strings.Join(foundSubjects, ", "))
	}
	return foundSubjects[0], nil
}

func evidenceDetailsByFlags(c *components.Context) (*coreConfig.ServerDetails, error) {
	serverDetails, err := pluginsCommon.CreateServerDetailsWithConfigOffer(c, true, commonCliUtils.Platform)
	if err != nil {
		return nil, err
	}
	if serverDetails.Url == "" {
		return nil, errors.New("platform URL is mandatory for evidence commands")
	}
	platformToEvidenceUrls(serverDetails)
	return serverDetails, nil
}

func platformToEvidenceUrls(rtDetails *coreConfig.ServerDetails) {
	rtDetails.ArtifactoryUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "artifactory/"
	rtDetails.EvidenceUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "evidence/"
}

func assertValueProvided(c *components.Context, fieldName string) error {
	if c.GetStringFlagValue(fieldName) == "" {
		return errorutils.CheckErrorf("the --%s option is mandatory", fieldName)
	}
	return nil
}
