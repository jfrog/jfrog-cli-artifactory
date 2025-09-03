package create

import (
	"errors"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type createEvidenceBuild struct {
	createEvidenceBase
	project     string
	buildName   string
	buildNumber string
}

func NewCreateEvidenceBuild(serverDetails *config.ServerDetails,
	predicateFilePath, predicateType, markdownFilePath, key, keyId, project, buildName, buildNumber, providerId, integration string) evidence.Command {
	return &createEvidenceBuild{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
			providerId:        providerId,
			integration:       integration,
		},
		project:     project,
		buildName:   buildName,
		buildNumber: buildNumber,
	}
}

func (c *createEvidenceBuild) CommandName() string {
	return "create-buildName-evidence"
}

func (c *createEvidenceBuild) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createEvidenceBuild) Run() error {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		log.Error("failed to create Artifactory client", err)
		return err
	}

	timestamp, err := getBuildLatestTimestamp(c.buildName, c.buildNumber, c.project, artifactoryClient)
	if err != nil {
		return err
	}

	subject, sha256, err := c.buildBuildInfoSubjectPath(artifactoryClient, timestamp)
	if err != nil {
		return err
	}
	envelope, err := c.createEnvelope(subject, sha256)
	if err != nil {
		return err
	}

	response, err := c.uploadEvidence(envelope, subject)
	if err != nil {
		return err
	}
	c.recordSummary(subject, sha256, response, timestamp)

	return nil
}

func (c *createEvidenceBuild) buildBuildInfoSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager, timestamp string) (string, string, error) {
	repoKey := utils.BuildBuildInfoRepoKey(c.project)
	buildInfoPath := buildBuildInfoPath(repoKey, c.buildName, c.buildNumber, timestamp)
	buildInfoChecksum, err := getBuildInfoPathChecksum(buildInfoPath, artifactoryClient)
	if err != nil {
		return "", "", err
	}
	return buildInfoPath, buildInfoChecksum, nil
}

func (c *createEvidenceBuild) recordSummary(subject string, sha256 string, response *model.CreateResponse, timestamp string) {
	displayName := fmt.Sprintf("%s %s", c.buildName, c.buildNumber)
	commandSummary := commandsummary.EvidenceSummaryData{
		Subject:        subject,
		SubjectSha256:  sha256,
		PredicateType:  c.predicateType,
		PredicateSlug:  response.PredicateSlug,
		Verified:       response.Verified,
		DisplayName:    displayName,
		SubjectType:    commandsummary.SubjectTypeBuild,
		BuildName:      c.buildName,
		BuildNumber:    c.buildNumber,
		BuildTimestamp: timestamp,
		RepoKey:        utils.BuildBuildInfoRepoKey(c.project),
	}

	err := c.recordEvidenceSummary(commandSummary)
	if err != nil {
		log.Warn("Failed to record evidence summary:", err.Error())
	}
}

func getBuildLatestTimestamp(name string, number string, project string, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	buildInfo := services.BuildInfoParams{
		BuildName:   name,
		BuildNumber: number,
		ProjectKey:  project,
	}
	log.Debug("Getting build info for buildName:", name, "buildNumber:", number, "project:", project)
	res, ok, err := artifactoryClient.GetBuildInfo(buildInfo)
	if err != nil {
		return "", fmt.Errorf("failed to get build info for buildName: %s, buildNumber: %s, project: %s, error: %w", name, number, project, err)
	}
	if !ok {
		errorMessage := fmt.Sprintf("failed to find buildName, name:%s, number:%s, project: %s", name, number, project)
		return "", errorutils.CheckError(errors.New(errorMessage))
	}
	timestamp, err := utils.ParseIsoTimestamp(res.BuildInfo.Started)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", timestamp.UnixMilli()), nil
}

func buildBuildInfoPath(repoKey string, name string, number string, timestamp string) string {
	jsonFile := fmt.Sprintf("%s-%s.json", number, timestamp)
	return fmt.Sprintf("%s/%s/%s", repoKey, name, jsonFile)
}

func getBuildInfoPathChecksum(buildInfoPath string, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	res, err := artifactoryClient.FileInfo(buildInfoPath)
	if err != nil {
		log.Warn(fmt.Sprintf("buildName info json path '%s' does not exist.", buildInfoPath))
		return "", err
	}
	return res.Checksums.Sha256, nil
}
