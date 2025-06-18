package evidence

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/metadata"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	clientLog "github.com/jfrog/jfrog-client-go/utils/log"
)

const leadArtifactQueryTemplate = `{
	"query": "{versions(filter: {packageId: \"%s\", name: \"%s\", repositoriesIn: [{name: \"%s\"}]}) { edges { node { repos { name leadFilePath } } } } }"
}`

// PackageService defines the interface for package-related operations
type PackageService interface {
	// GetPackageType retrieves the package type for the given package
	GetPackageType(artifactoryClient artifactory.ArtifactoryServicesManager) (string, error)

	// GetPackageVersionLeadArtifact retrieves the lead artifact path for a package version
	// with fallback logic from Artifactory to Metadata service
	GetPackageVersionLeadArtifact(packageType string, metadataClient metadata.Manager, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error)

	// GetPackageName returns the package name
	GetPackageName() string

	// GetPackageVersion returns the package version
	GetPackageVersion() string

	// GetPackageRepoName returns the package repository name
	GetPackageRepoName() string
}

// NewPackageService creates a new PackageService instance
// This factory function allows for easy creation and potential future extension
func NewPackageService(name, version, repoName string) PackageService {
	return &basePackage{
		PackageName:     name,
		PackageVersion:  version,
		PackageRepoName: repoName,
	}
}

// basePackage provides shared logic for package evidence commands (create/verify)
// It implements the PackageService interface
type basePackage struct {
	PackageName     string
	PackageVersion  string
	PackageRepoName string
}

// Ensure basePackage implements PackageService interface
var _ PackageService = (*basePackage)(nil)

func (b *basePackage) GetPackageType(artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	if artifactoryClient == nil {
		return "", errorutils.CheckErrorf("Artifactory client is required")
	}

	var request services.RepositoryDetails
	err := artifactoryClient.GetRepository(b.PackageRepoName, &request)
	if err != nil {
		return "", errorutils.CheckErrorf("failed to get repository '%s': %w", b.PackageRepoName, err)
	}
	return request.PackageType, nil
}

func (b *basePackage) GetPackageVersionLeadArtifact(packageType string, metadataClient metadata.Manager, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	if artifactoryClient == nil {
		return "", errorutils.CheckErrorf("Artifactory client is required")
	}
	if metadataClient == nil {
		return "", errorutils.CheckErrorf("Metadata client is required")
	}

	leadFileRequest := services.LeadFileParams{
		PackageType:     strings.ToUpper(packageType),
		PackageRepoName: b.PackageRepoName,
		PackageName:     b.PackageName,
		PackageVersion:  b.PackageVersion,
	}

	leadArtifact, err := artifactoryClient.GetPackageLeadFile(leadFileRequest)
	if err != nil {
		// Fallback to metadata service
		leadArtifactPath, err := b.getPackageVersionLeadArtifactFromMetaData(packageType, metadataClient)
		if err != nil {
			return "", fmt.Errorf("failed to get lead artifact from both Artifactory and Metadata services: %w", err)
		}
		return b.buildLeadArtifactPath(leadArtifactPath), nil
	}

	leadArtifactPath := strings.Replace(string(leadArtifact), ":", "/", 1)
	return leadArtifactPath, nil
}

func (b *basePackage) getPackageVersionLeadArtifactFromMetaData(packageType string, metadataClient metadata.Manager) (string, error) {
	body, err := metadataClient.GraphqlQuery(b.createQuery(packageType))
	if err != nil {
		return "", fmt.Errorf("failed to query metadata service: %w", err)
	}

	res := &model.MetadataResponse{}
	err = json.Unmarshal(body, res)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal metadata response: %w", err)
	}
	if len(res.Data.Versions.Edges) == 0 {
		return "", errorutils.CheckErrorf("no package found: %s/%s", b.PackageRepoName, b.PackageVersion)
	}

	// Fetch the leadFilePath based on repoName
	for _, repo := range res.Data.Versions.Edges[0].Node.Repos {
		if repo.Name == b.PackageRepoName {
			return repo.LeadFilePath, nil
		}
	}
	return "", errorutils.CheckErrorf("lead artifact not found for package: %s/%s", b.PackageRepoName, b.PackageVersion)
}

func (c *basePackage) createQuery(packageType string) []byte {
	packageId := packageType + "://" + c.PackageName
	query := fmt.Sprintf(leadArtifactQueryTemplate, packageId, c.PackageVersion, c.PackageRepoName)
	clientLog.Debug("Fetch lead artifact using graphql query:", query)
	return []byte(query)
}

func (c *basePackage) buildLeadArtifactPath(leadArtifact string) string {
	return fmt.Sprintf("%s/%s", c.PackageRepoName, leadArtifact)
}

// GetPackageName returns the package name
func (b *basePackage) GetPackageName() string {
	return b.PackageName
}

// GetPackageVersion returns the package version
func (b *basePackage) GetPackageVersion() string {
	return b.PackageVersion
}

// GetPackageRepoName returns the package repository name
func (b *basePackage) GetPackageRepoName() string {
	return b.PackageRepoName
}
