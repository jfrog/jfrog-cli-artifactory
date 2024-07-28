package cli

import (
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cli/docs/create"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
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

func createEvidence(c *components.Context) error {
	if err := validateContext(c); err != nil {
		return err
	}
	subject, err := getAndValidateSubject(c)
	if err != nil {
		return err
	}
	artifactoryClient, err := evidenceDetailsByFlags(c)
	if err != nil {
		return err
	}
	var command EvidenceCommands
	if subject == EvdRepoPath {
		command = NewEvidenceCustomCommand(c)
	}
	if subject == releaseBundle {
		command = NewEvidenceReleaseBundleCommand(c)
	}
	return command.CreateEvidence(artifactoryClient)
}

func validateContext(c *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}
	if len(c.Arguments) > 1 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}
	return nil
}

func getAndValidateSubject(c *components.Context) (string, error) {
	subjects := []string{
		EvdRepoPath,
		releaseBundle,
	}
	var foundSubjects []string
	for _, key := range subjects {
		if c.GetStringFlagValue(key) != "" {
			foundSubjects = append(foundSubjects, key)
		}
	}

	if len(foundSubjects) == 0 {
		return "", errorutils.CheckErrorf("Subject must be one of the fields: repo-path, release-bundle")
	}
	if len(foundSubjects) > 1 {
		return "", errorutils.CheckErrorf("multiple subjects found: [%s]. Please specify only one", strings.Join(foundSubjects, ", "))
	}
	return foundSubjects[0], nil
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
