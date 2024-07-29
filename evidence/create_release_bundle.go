package evidence

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

type CreateEvidenceReleaseBundle struct {
	CreateEvidenceBase
	project       string
	releaseBundle string
}

func NewCreateEvidenceReleaseBundle() *CreateEvidenceReleaseBundle {
	return &CreateEvidenceReleaseBundle{}
}

func (c *CreateEvidenceReleaseBundle) SetServerDetails(serverDetails *config.ServerDetails) *CreateEvidenceReleaseBundle {
	c.serverDetails = serverDetails
	return c
}

func (c *CreateEvidenceReleaseBundle) SetPredicateFilePath(predicateFilePath string) *CreateEvidenceReleaseBundle {
	c.predicateFilePath = predicateFilePath
	return c
}

func (c *CreateEvidenceReleaseBundle) SetPredicateType(predicateType string) *CreateEvidenceReleaseBundle {
	c.predicateType = predicateType
	return c
}

func (c *CreateEvidenceReleaseBundle) SetProject(project string) *CreateEvidenceReleaseBundle {
	c.project = project
	return c
}

func (c *CreateEvidenceReleaseBundle) SetReleaseBundle(releaseBundle string) *CreateEvidenceReleaseBundle {
	c.releaseBundle = releaseBundle
	return c
}

func (c *CreateEvidenceReleaseBundle) SetKey(key string) *CreateEvidenceReleaseBundle {
	c.key = key
	return c
}

func (c *CreateEvidenceReleaseBundle) SetKeyId(keyId string) *CreateEvidenceReleaseBundle {
	c.keyId = keyId
	return c
}

func (c *CreateEvidenceReleaseBundle) CommandName() string {
	return "create-release-bundle-evidence"
}

func (c *CreateEvidenceReleaseBundle) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *CreateEvidenceReleaseBundle) Run() error {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		log.Error("failed to create Artifactory client", err)
		return err
	}

	subject, err := c.buildReleaseBundleSubjectPath(artifactoryClient)
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

func (c *CreateEvidenceReleaseBundle) buildReleaseBundleSubjectPath(client artifactory.ArtifactoryServicesManager) (string, error) {
	releaseBundle := strings.Split(c.releaseBundle, ":")
	name := releaseBundle[0]
	version := releaseBundle[1]
	repoKey := buildRepoKey(c.project)
	manifestPath := buildManifestPath(repoKey, name, version)

	manifestChecksum, err := getManifestPathChecksum(manifestPath, client)
	if err != nil {
		return "", err
	}

	return manifestPath + "@" + manifestChecksum, nil
}

func buildRepoKey(project string) string {
	if project == "" || project == "default" {
		return "release-bundles-v2"
	}
	return fmt.Sprintf("%s-release-bundles-v2", project)
}

func buildManifestPath(repoKey, name, version string) string {
	return fmt.Sprintf("%s/%s/%s/release-bundle.json.evd", repoKey, name, version)
}

func getManifestPathChecksum(manifestPath string, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	res, err := artifactoryClient.FileInfo(manifestPath)
	if err != nil {
		log.Warn(fmt.Sprintf("release bundle manifest path '%s' does not exist.", manifestPath))
		return "", err
	}
	return res.Checksums.Sha256, nil
}
