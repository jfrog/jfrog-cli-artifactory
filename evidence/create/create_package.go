package create

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type createEvidencePackage struct {
	createEvidenceBase
	packageService evidence.PackageService
}

func NewCreateEvidencePackage(serverDetails *config.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, packageName,
	packageVersion, packageRepoName string) evidence.Command {
	return &createEvidencePackage{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
		},
		packageService: evidence.NewPackageService(packageName, packageVersion, packageRepoName),
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

	packageType, err := c.packageService.GetPackageType(artifactoryClient)
	if err != nil {
		return err
	}

	leadArtifactPath, err := c.packageService.GetPackageVersionLeadArtifact(packageType, metadataClient, artifactoryClient)
	if err != nil {
		return err
	}

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
