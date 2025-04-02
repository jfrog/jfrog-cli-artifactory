package evidence

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type createEvidenceReleaseBundle struct {
	createEvidenceBase
	project              string
	releaseBundle        string
	releaseBundleVersion string
}

const (
	releaseBundleEvdManifestName       = "release-bundle.json"
	releaseBundleEvdManifestNameLegacy = "release-bundle.json.evd"
)

func NewCreateEvidenceReleaseBundle(serverDetails *coreConfig.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, project, releaseBundle,
	releaseBundleVersion string) Command {
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
	repoKey := buildRepoKey(c.project)
	manifestPath, manifestChecksum, err := c.buildReleaseBundleSubjectPathWithManifestName(artifactoryClient, releaseBundleEvdManifestName, repoKey)
	if err == nil {
		return manifestPath, manifestChecksum, nil
	}
	log.Info(fmt.Sprintf("Failed to get manifest name %s in %s", releaseBundleEvdManifestName, manifestPath), err)
	log.Info(fmt.Sprintf("Attempt to get manifest name %s", releaseBundleEvdManifestNameLegacy))
	// fallback to legacy name
	manifestPath, manifestChecksum, err = c.buildReleaseBundleSubjectPathWithManifestName(artifactoryClient, releaseBundleEvdManifestNameLegacy, repoKey)
	if err != nil {
		return "", "", err
	}

	return manifestPath, manifestChecksum, nil
}

func (c *createEvidenceReleaseBundle) buildReleaseBundleSubjectPathWithManifestName(artifactoryClient artifactory.ArtifactoryServicesManager, manifestName, repoKey string) (string, string, error) {
	manifestPath := buildManifestPath(repoKey, c.releaseBundle, c.releaseBundleVersion, manifestName)
	manifestChecksum, err := c.getFileChecksum(manifestPath, artifactoryClient)

	return manifestPath, manifestChecksum, err
}

func buildRepoKey(project string) string {
	if project == "" || project == "default" {
		return "release-bundles-v2"
	}

	return fmt.Sprintf("%s-release-bundles-v2", project)
}

func buildManifestPath(repoKey, name, version, manifestName string) string {
	return fmt.Sprintf("%s/%s/%s/%s", repoKey, name, version, manifestName)
}
