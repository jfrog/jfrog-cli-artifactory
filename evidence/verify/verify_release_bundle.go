package verify

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const aqlReleaseBundleQueryTemplate = "items.find({\"repo\": \"%s\",\"path\": \"%s\",\"name\": \"%s\"}).include(\"sha256\")"

// verifyEvidenceReleaseBundle verifies evidence for a release bundle.
type verifyEvidenceReleaseBundle struct {
	verifyEvidenceBase
	project              string
	releaseBundle        string
	releaseBundleVersion string
}

// NewVerifyEvidenceReleaseBundle creates a new command for verifying evidence for a release bundle.
func NewVerifyEvidenceReleaseBundle(serverDetails *config.ServerDetails, format, project, releaseBundle, releaseBundleVersion string, keys []string, useArtifactoryKeys bool) evidence.Command {
	return &verifyEvidenceReleaseBundle{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails:      serverDetails,
			format:             format,
			keys:               keys,
			useArtifactoryKeys: useArtifactoryKeys,
		},
		project:              project,
		releaseBundle:        releaseBundle,
		releaseBundleVersion: releaseBundleVersion,
	}
}

// CommandName returns the command name for release bundle evidence verification.
func (c *verifyEvidenceReleaseBundle) CommandName() string {
	return "create-release-bundle-evidence"
}

// ServerDetails returns the server details for the command.
func (c *verifyEvidenceReleaseBundle) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// Run executes the release bundle evidence verification command.
func (c *verifyEvidenceReleaseBundle) Run() error {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		return fmt.Errorf("failed to create Artifactory client: %w", err)
	}

	repoKey := utils.BuildReleaseBundleRepoKey(c.project)

	path := fmt.Sprintf("%s/%s", c.releaseBundle, c.releaseBundleVersion)
	result, err := utils.ExecuteAqlQuery(fmt.Sprintf(aqlReleaseBundleQueryTemplate, repoKey, path, "release-bundle.json.evd"), artifactoryClient)
	if err != nil {
		return fmt.Errorf("failed to execute AQL query: %w", err)
	}
	if len(result.Results) == 0 {
		return fmt.Errorf("no release bundle manifest found for the given release bundle and version")
	}

	metadata, err := c.queryEvidenceMetadata(repoKey, path, "release-bundle.json.evd")
	if err != nil {
		return err
	}
	subjectPath := fmt.Sprintf("%s/%s/%s", repoKey, path, "release-bundle.json.evd")
	releaseBundleSha256 := result.Results[0].Sha256
	return c.verifyEvidences(artifactoryClient, metadata, releaseBundleSha256, subjectPath)
}
