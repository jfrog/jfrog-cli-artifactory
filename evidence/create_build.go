package evidence

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

type createEvidenceBuild struct {
	createEvidenceBase
	project string
	build   string
}

func NewCreateEvidenceBuild(serverDetails *coreConfig.ServerDetails,
	predicateFilePath, predicateType, key, keyId, project, build string) Command {
	return &createEvidenceBuild{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			key:               key,
			keyId:             keyId,
		},
		project: project,
		build:   build,
	}
}

func (c *createEvidenceBuild) CommandName() string {
	return "create-build-evidence"
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
	subject, err := c.buildBuildInfoSubjectPath(artifactoryClient)
	if err != nil {
		return err
	}
	envelope, err := c.createEnvelope(subject)
	if err != nil {
		return err
	}
	err = c.uploadEvidence(envelope, subject)
	if err != nil {
		return err
	}

	return nil
}

func (c *createEvidenceBuild) buildBuildInfoSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	build := strings.Split(c.build, ":")
	if len(build) != 2 {
		return "", fmt.Errorf("invalid build format. expected format is <name>:<number>")
	}
	name := build[0]
	number := build[1]

	timestamp, err := getBuildLatestTimestamp(name, number, c.project, artifactoryClient)
	if err != nil {
		return "", err
	}

	repoKey := buildBuildInfoRepoKey(c.project)
	buildInfoPath := buildBuildInfoPath(repoKey, name, number, timestamp)
	buildInfoChecksum, err := getBuildInfoPathChecksum(buildInfoPath, artifactoryClient)
	if err != nil {
		return "", err
	}
	return buildInfoPath + "@" + buildInfoChecksum, nil
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
		errorMessage := fmt.Sprintf("failed to find build, name:%s, number:%s, project: %s", name, number, project)
		return "", errorutils.CheckErrorf(errorMessage)
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
		log.Warn(fmt.Sprintf("build info json path '%s' does not exist.", buildInfoPath))
		return "", err
	}
	return res.Checksums.Sha256, nil
}
