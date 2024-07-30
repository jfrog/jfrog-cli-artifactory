package evidence

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

type createEvidenceReleaseBundle struct {
	createEvidenceBase
	project       string
	releaseBundle string
}

func NewCreateEvidenceReleaseBundle(serverDetails *coreConfig.ServerDetails, predicateFilePath string, predicateType string, key string, keyId string,
	project string, releaseBundle string) Command {
	return &createEvidenceReleaseBundle{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			key:               key,
			keyId:             keyId,
		},
		project:       project,
		releaseBundle: releaseBundle,
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

func (c *createEvidenceReleaseBundle) buildReleaseBundleSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	releaseBundle := strings.Split(c.releaseBundle, ":")
	if len(releaseBundle) != 2 {
		return "", fmt.Errorf("invalid release bundle format. expected format is <name>:<version>")
	}
	name := releaseBundle[0]
	version := releaseBundle[1]
	repoKey := buildRepoKey(c.project)
	manifestPath := buildManifestPath(repoKey, name, version)

	manifestChecksum, err := getManifestPathChecksum(manifestPath, artifactoryClient)
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
