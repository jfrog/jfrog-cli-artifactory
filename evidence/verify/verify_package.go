package verify

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"

	cliUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const aqlPackageQueryTemplate = "items.find({\"repo\": \"%s\",\"path\": \"%s\",\"name\": \"%s\"}).include(\"sha256\")"

// verifyEvidencePackage verifies evidence for a package.
type verifyEvidencePackage struct {
	verifyEvidenceBase
	packageService evidence.PackageService
}

// NewVerifyEvidencePackage creates a new command for verifying evidence for a package.
func NewVerifyEvidencePackage(serverDetails *config.ServerDetails, format, packageName, packageVersion, packageRepoName string, keys []string, useArtifactoryKeys bool) evidence.Command {
	return &verifyEvidencePackage{
		verifyEvidenceBase: newVerifyEvidenceBase(serverDetails, format, keys, useArtifactoryKeys),
		packageService:     evidence.NewPackageService(packageName, packageVersion, packageRepoName),
	}
}

// CommandName returns the command name for package evidence verification.
func (c *verifyEvidencePackage) CommandName() string {
	return "verify-package-evidence"
}

// ServerDetails returns the server details for the command.
func (c *verifyEvidencePackage) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// Run executes the package evidence verification command.
func (c *verifyEvidencePackage) Run() error {
	defer c.quitProgress()

	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		return fmt.Errorf("failed to create Artifactory client: %w", err)
	}
	c.setHeadline("Searching package")
	packageType, err := c.packageService.GetPackageType(*artifactoryClient)
	if err != nil {
		return fmt.Errorf("failed to get package type: %w", err)
	}

	metadataClient, err := cliUtils.CreateMetadataServiceManager(c.serverDetails, false)
	if err != nil {
		return fmt.Errorf("failed to create metadata service manager: %w", err)
	}

	leadArtifactPath, err := c.packageService.GetPackageVersionLeadArtifact(packageType, metadataClient, *artifactoryClient)
	if err != nil {
		return fmt.Errorf("failed to get package version lead artifact: %w", err)
	}
	split := strings.Split(leadArtifactPath, "/")
	if len(split) == 0 {
		return fmt.Errorf("invalid lead artifact path: %s", leadArtifactPath)
	}
	fileName := split[len(split)-1]

	path := fmt.Sprintf("%s/%s", c.packageService.GetPackageName(), c.packageService.GetPackageVersion())
	aqlQuery := fmt.Sprintf(aqlPackageQueryTemplate, c.packageService.GetPackageRepoName(), path, fileName)
	result, err := utils.ExecuteAqlQuery(aqlQuery, artifactoryClient)
	if err != nil {
		return fmt.Errorf("failed to execute AQL query: %w", err)
	}
	if len(result.Results) == 0 {
		return errors.New("no package lead file found for the given package name and version")
	}

	packageSha256 := result.Results[0].Sha256
	metadata, err := c.queryEvidenceMetadata(c.packageService.GetPackageRepoName(), path, fileName)
	if err != nil {
		return err
	}
	subjectPath := fmt.Sprintf("%s/%s/%s", c.packageService.GetPackageRepoName(), path, fileName)
	return c.verifyEvidence(artifactoryClient, metadata, packageSha256, subjectPath)
}
