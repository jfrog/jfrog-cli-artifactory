package resolver

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	cliUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/metadata"
)

type PackagePathResolver struct {
	packageService    evidence.PackageService
	artifactoryClient artifactory.ArtifactoryServicesManager
	metadataClient    metadata.Manager
}

func NewPackagePathResolver(packageName, packageVersion, repoName string, serverDetails *config.ServerDetails) (*PackagePathResolver, error) {
	artifactoryClient, err := cliUtils.CreateUploadServiceManager(serverDetails, 1, 0, 0, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Artifactory client: %w", err)
	}
	metadataClient, err := cliUtils.CreateMetadataServiceManager(serverDetails, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata service manager: %w", err)
	}
	return &PackagePathResolver{
		packageService:    evidence.NewPackageService(packageName, packageVersion, repoName),
		artifactoryClient: artifactoryClient,
		metadataClient:    metadataClient,
	}, nil
}

func (p *PackagePathResolver) ResolveSubjectRepoPath() (string, error) {
	packageType, err := p.packageService.GetPackageType(p.artifactoryClient)
	if err != nil {
		return "", fmt.Errorf("failed to get package type: %w", err)
	}

	leadArtifactPath, err := p.packageService.GetPackageVersionLeadArtifact(packageType, p.metadataClient, p.artifactoryClient)
	if err != nil {
		return "", fmt.Errorf("failed to get package version lead artifact: %w", err)
	}
	return leadArtifactPath, nil
}
