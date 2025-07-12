package create

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type createEvidenceReleaseBundle struct {
	createEvidenceBase
	project              string
	releaseBundle        string
	releaseBundleVersion string
}

func NewCreateEvidenceReleaseBundle(serverDetails *config.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, project, releaseBundle,
	releaseBundleVersion string) evidence.Command {
	return &createEvidenceReleaseBundle{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
		},
		project:              project,
		releaseBundle:        releaseBundle,
		releaseBundleVersion: releaseBundleVersion,
	}
}

func (c *createEvidenceReleaseBundle) CommandName() string {
	return "create-release-bundle-evidence"
}

func (c *createEvidenceReleaseBundle) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createEvidenceReleaseBundle) Run() error {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		log.Error("failed to create Artifactory client", err)
		return err
	}
	subject, sha256, err := c.buildReleaseBundleSubjectPath(artifactoryClient)
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

func (c *createEvidenceReleaseBundle) buildReleaseBundleSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager) (string, string, error) {
	repoKey := utils.BuildReleaseBundleRepoKey(c.project)
	manifestPath := buildManifestPath(repoKey, c.releaseBundle, c.releaseBundleVersion)

	manifestChecksum, err := c.getFileChecksum(manifestPath, artifactoryClient)
	if err != nil {
		return "", "", err
	}

	return manifestPath, manifestChecksum, nil
}

func buildManifestPath(repoKey, name, version string) string {
	return fmt.Sprintf("%s/%s/%s/release-bundle.json.evd", repoKey, name, version)
}
