package evidence

import (
	"errors"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"strings"

	cliUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const aqlPackageQueryTemplate = "items.find({\"repo\": \"%s\",\"path\": \"%s\",\"name\": \"%s\"}).include(\"sha256\")"

// verifyEvidencePackage verifies evidence for a package.
type verifyEvidencePackage struct {
	verifyEvidenceBase
	basePackage
}

// NewVerifyEvidencePackage creates a new command for verifying evidence for a package.
func NewVerifyEvidencePackage(serverDetails *coreConfig.ServerDetails, format, packageName, packageVersion, packageRepoName string, keys []string, useArtifactoryKeys bool) Command {
	return &verifyEvidencePackage{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails:      serverDetails,
			format:             format,
			keys:               keys,
			useArtifactoryKeys: useArtifactoryKeys,
		},
		basePackage: basePackage{
			PackageName:     packageName,
			PackageVersion:  packageVersion,
			PackageRepoName: packageRepoName,
		},
	}
}

// CommandName returns the command name for package evidence verification.
func (c *verifyEvidencePackage) CommandName() string {
	return "verify-package-evidence"
}

// ServerDetails returns the server details for the command.
func (c *verifyEvidencePackage) ServerDetails() (*coreConfig.ServerDetails, error) {
	return c.serverDetails, nil
}

// Run executes the package evidence verification command.
func (c *verifyEvidencePackage) Run() error {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		return fmt.Errorf("failed to create Artifactory client: %w", err)
	}
	packageType, err := c.basePackage.getPackageType(*artifactoryClient)
	if err != nil {
		return fmt.Errorf("failed to get package type: %w", err)
	}
	metadataClient, err := cliUtils.CreateMetadataServiceManager(c.serverDetails, false)
	if err != nil {
		return fmt.Errorf("failed to create metadata service manager: %w", err)
	}
	leadArtifactPath, err := c.basePackage.getPackageVersionLeadArtifact(packageType, metadataClient, *artifactoryClient)
	if err != nil {
		return fmt.Errorf("failed to get package version lead artifact: %w", err)
	}
	split := strings.Split(leadArtifactPath, "/")
	fileName := split[len(split)-1]

	path := fmt.Sprintf("%s/%s", c.basePackage.PackageName, c.basePackage.PackageVersion)

	query := fmt.Sprintf(aqlPackageQueryTemplate, c.basePackage.PackageRepoName, path, fileName)
	result, err := utils.ExecuteAqlQuery(query, artifactoryClient)
	if err != nil {
		return fmt.Errorf("failed to execute AQL query: %w", err)
	}
	if len(result.Results) == 0 {
		return errors.New("no package lead file found for the given package name and version")
	}
	packageSha256 := result.Results[0].Sha256
	metadata, err := c.queryEvidenceMetadata(c.basePackage.PackageRepoName, path, fileName)
	if err != nil {
		return err
	}
	subjectPath := fmt.Sprintf("%s/%s/%s", c.basePackage.PackageRepoName, path, fileName)
	return c.verifyEvidences(artifactoryClient, metadata, packageSha256, subjectPath)
}
