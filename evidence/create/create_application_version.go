package create

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type createEvidenceApplication struct {
	createEvidenceBase
	project            string
	application        string
	applicationVersion string
}

func NewCreateEvidenceApplication(serverDetails *config.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, project, applicationName,
	applicationVersion string) evidence.Command {
	return &createEvidenceApplication{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
		},
		project:            project,
		application:        applicationName,
		applicationVersion: applicationVersion,
	}
}

func (c *createEvidenceApplication) CommandName() string {
	return "create-application-evidence"
}

func (c *createEvidenceApplication) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createEvidenceApplication) Run() error {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		log.Error("failed to create Artifactory client", err)
		return err
	}
	subject, sha256, err := c.buildApplicationSubjectPath(artifactoryClient)
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

func (c *createEvidenceApplication) buildApplicationSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager) (string, string, error) {
	repoKey := utils.BuildReleaseBundleRepoKey(c.project) // currently same as release bundle repo key
	manifestPath := buildManifestPathApplication(repoKey, c.application, c.applicationVersion)

	manifestChecksum, err := c.getFileChecksum(manifestPath, artifactoryClient)
	if err != nil {
		return "", "", err
	}

	return manifestPath, manifestChecksum, nil
}

func buildManifestPathApplication(repoKey, name, version string) string {
	return fmt.Sprintf("%s/%s/%s/release-bundle.json.evd", repoKey, name, version)
}
