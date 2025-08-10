package resolver

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	artifactoryUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
)

const aqlBuildQueryTemplate = "items.find({\"repo\":\"%s\",\"path\":\"%s\",\"name\":{\"$match\":\"%s*\"}}).include(\"sha256\",\"name\").sort({\"$desc\":[\"name\"]}).limit(1)"

type BuildPathResolver struct {
	project           string
	buildName         string
	buildNumber       string
	artifactoryClient artifactory.ArtifactoryServicesManager
}

func NewBuildPathResolver(project, buildName, buildNumber string, serverDetails *config.ServerDetails) (*BuildPathResolver, error) {
	client, err := artifactoryUtils.CreateUploadServiceManager(serverDetails, 1, 0, 0, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Artifactory client: %w", err)
	}
	return &BuildPathResolver{
		project:           project,
		buildName:         buildName,
		buildNumber:       buildNumber,
		artifactoryClient: client,
	}, nil
}

func (b *BuildPathResolver) ResolveSubjectRepoPath() (string, error) {
	repoKey := utils.BuildBuildInfoRepoKey(b.project)
	result, err := utils.ExecuteAqlQuery(fmt.Sprintf(aqlBuildQueryTemplate, repoKey, b.buildName, b.buildNumber), &b.artifactoryClient)
	if err != nil {
		return "", fmt.Errorf("failed to execute AQL query: %w", err)
	}
	if len(result.Results) == 0 {
		return "", fmt.Errorf("no build found for the given build name and number")
	}
	subjectFileName := result.Results[0].Name

	subjectPath := fmt.Sprintf("%s/%s/%s", repoKey, b.buildName, subjectFileName)
	return subjectPath, nil
}
