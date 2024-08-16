package evidence

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/metadata"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const leadArtifactQueryTemplate = `{
	"query": "{versions(filter: {packageId: \"%s\", name: \"%s\", repositoriesIn: [{name: \"%s\"}]}) { edges { node { repos { name leadFilePath } } } } }"
}`

type createEvidencePackage struct {
	createEvidenceBase
	packageName     string
	packageVersion  string
	packageRepoName string
}

func NewCreateEvidencePackage(serverDetails *coreConfig.ServerDetails, predicateFilePath, predicateType, key, keyId, packageName,
	packageVersion, packageRepoName string) Command {
	return &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			key:               key,
			keyId:             keyId,
		},
		packageName:     packageName,
		packageVersion:  packageVersion,
		packageRepoName: packageRepoName,
	}
}

func (c *createEvidencePackage) CommandName() string {
	return "create-package-evidence"
}

func (c *createEvidencePackage) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createEvidencePackage) Run() error {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		log.Error("failed to create Artifactory client", err)
		return err
	}
	metadataClient, err := utils.CreateMetadataServiceManager(c.serverDetails, false)
	if err != nil {
		return err
	}

	packageType, err := c.getPackageType(artifactoryClient)
	if err != nil {
		return err
	}

	leadArtifact, err := c.getPackageVersionLeadArtifact(packageType, metadataClient)
	if err != nil {
		return err
	}
	leadArtifactPath := c.buildLeadArtifactPath(leadArtifact)
	leadArtifactChecksum, err := c.getFileChecksum(leadArtifactPath, artifactoryClient)
	if err != nil {
		return err
	}
	envelope, err := c.createEnvelope(leadArtifactPath, leadArtifactChecksum)
	if err != nil {
		return err
	}
	err = c.uploadEvidence(envelope, leadArtifactPath)
	if err != nil {
		return err
	}

	return nil
}

func (c *createEvidencePackage) getPackageType(artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	var request services.RepositoryDetails
	err := artifactoryClient.GetRepository(c.packageRepoName, &request)
	if err != nil {
		return "", errorutils.CheckErrorf("No such package: %s/%s", c.packageRepoName, c.packageVersion)
	}
	return request.PackageType, nil
}

func (c *createEvidencePackage) getPackageVersionLeadArtifact(packageType string, metadataClient metadata.Manager) (string, error) {
	body, err := metadataClient.GraphqlQuery(c.createQuery(packageType))
	if err != nil {
		return "", err
	}

	res := &model.MetadataResponse{}
	err = json.Unmarshal(body, res)
	if err != nil {
		return "", err
	}
	if len(res.Data.Versions.Edges) == 0 {
		return "", errorutils.CheckErrorf("No such package: %s/%s", c.packageRepoName, c.packageVersion)
	}

	// Fetch the leadFilePath based on repoName
	for _, repo := range res.Data.Versions.Edges[0].Node.Repos {
		if repo.Name == c.packageRepoName {
			return repo.LeadFilePath, nil
		}
	}
	return "", errorutils.CheckErrorf("Can't find lead artifact of pacakge: %s/%s", c.packageRepoName, c.packageVersion)
}

func (c *createEvidencePackage) createQuery(packageType string) []byte {
	packageId := packageType + "://" + c.packageName
	return []byte(fmt.Sprintf(leadArtifactQueryTemplate, packageId, c.packageVersion, c.packageRepoName))
}

func (c *createEvidencePackage) buildLeadArtifactPath(leadArtifact string) string {
	return fmt.Sprintf("%s/%s", c.packageRepoName, leadArtifact)
}
