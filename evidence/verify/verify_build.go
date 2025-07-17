package verify

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const aqlBuildQueryTemplate = "items.find({\"repo\":\"%s\",\"path\":\"%s\",\"name\":{\"$match\":\"%s*\"}}).include(\"sha256\",\"name\").sort({\"$desc\":[\"name\"]}).limit(1)"

// verifyEvidenceBuild verifies evidence for a build.
type verifyEvidenceBuild struct {
	verifyEvidenceBase
	project     string
	buildName   string
	buildNumber string
}

// NewVerifyEvidencesBuild creates a new command for verifying evidence for a build.
func NewVerifyEvidencesBuild(serverDetails *config.ServerDetails, project, buildName, buildNumber, format string, keys []string, useArtifactoryKeys bool) evidence.Command {
	return &verifyEvidenceBuild{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails:      serverDetails,
			format:             format,
			keys:               keys,
			useArtifactoryKeys: useArtifactoryKeys,
		},
		project:     project,
		buildName:   buildName,
		buildNumber: buildNumber,
	}
}

// Run executes the build evidence verification command.
func (v *verifyEvidenceBuild) Run() error {
	client, err := v.createArtifactoryClient()
	if err != nil {
		return fmt.Errorf("failed to create Artifactory client: %w", err)
	}

	repoKey := utils.BuildBuildInfoRepoKey(v.project)

	result, err := utils.ExecuteAqlQuery(fmt.Sprintf(aqlBuildQueryTemplate, repoKey, v.buildName, v.buildNumber), client)
	if err != nil {
		return fmt.Errorf("failed to execute AQL query: %w", err)
	}
	if len(result.Results) == 0 {
		return fmt.Errorf("no build found for the given build name and number")
	}
	buildInfoSha256 := result.Results[0].Sha256
	subjectFileName := result.Results[0].Name

	metadata, err := v.queryEvidenceMetadata(repoKey, v.buildName, subjectFileName)
	if err != nil {
		return err
	}

	subjectPath := fmt.Sprintf("%s/%s/%s", repoKey, v.buildName, subjectFileName)
	return v.verifyEvidences(client, metadata, buildInfoSha256, subjectPath)
}

// ServerDetails returns the server details for the command.
func (v *verifyEvidenceBuild) ServerDetails() (*config.ServerDetails, error) {
	return v.serverDetails, nil
}

// CommandName returns the command name for build evidence verification.
func (v *verifyEvidenceBuild) CommandName() string {
	return "verify-evidence-build"
}
