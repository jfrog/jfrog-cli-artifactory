package evidence

import (
	"errors"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
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

func NewCreateEvidenceBuild(serverDetails *coreConfig.ServerDetails,
	predicateFilePath, predicateType, markdownFilePath, key, keyId, project, buildName, buildNumber string) Command {
	return &createEvidenceBuild{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
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
	subject, sha256, err := c.buildBuildInfoSubjectPath(artifactoryClient)
	if err != nil {
		return err
	}
	envelope, err := c.createEnvelope(subject, sha256)
	if err != nil {
		return err
	}
	err = c.uploadEvidence(envelope, subject)
	if err != nil {
		return err
	}

	return nil
}

func (c *createEvidenceBuild) buildBuildInfoSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager) (string, string, error) {
	timestamp, err := getBuildLatestTimestamp(c.buildName, c.buildNumber, c.project, artifactoryClient)
	if err != nil {
		return "", "", err
	}

	repoKey := buildBuildInfoRepoKey(c.project)
	buildInfoPath := buildBuildInfoPath(repoKey, c.buildName, c.buildNumber, timestamp)
	buildInfoChecksum, err := getBuildInfoPathChecksum(buildInfoPath, artifactoryClient)
	if err != nil {
		return "", "", err
	}
	return buildInfoPath, buildInfoChecksum, nil
}

func getBuildLatestTimestamp(name string, number string, project string, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	buildInfo := services.BuildInfoParams{
		BuildName:   name,
		BuildNumber: number,
		ProjectKey:  project,
	}
	res, ok, err := artifactoryClient.GetBuildInfo(buildInfo)
	if err != nil {
		return "", err
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

func buildBuildInfoRepoKey(project string) string {
	if project == "" || project == "default" {
		return "artifactory-build-info"
	}
	return fmt.Sprintf("%s-build-info", project)
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
