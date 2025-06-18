package evidence

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/metadata"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	clientLog "github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

const leadArtifactQueryTemplate = `{
	"query": "{versions(filter: {packageId: \"%s\", name: \"%s\", repositoriesIn: [{name: \"%s\"}]}) { edges { node { repos { name leadFilePath } } } } }"
}`

// basePackage provides shared logic for package evidence commands (create/verify)
type basePackage struct {
	PackageName     string
	PackageVersion  string
	PackageRepoName string
}

func (b *basePackage) getPackageType(artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	var request services.RepositoryDetails
	err := artifactoryClient.GetRepository(b.PackageRepoName, &request)
	if err != nil {
		return "", errorutils.CheckErrorf("No such package: %s/%s", b.PackageRepoName, b.PackageVersion)
	}
	return request.PackageType, nil
}

func (b *basePackage) getPackageVersionLeadArtifact(packageType string, metadataClient metadata.Manager, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	leadFileRequest := services.LeadFileParams{
		PackageType:     strings.ToUpper(packageType),
		PackageRepoName: b.PackageRepoName,
		PackageName:     b.PackageName,
		PackageVersion:  b.PackageVersion,
	}

	leadArtifact, err := artifactoryClient.GetPackageLeadFile(leadFileRequest)
	if err != nil {
		leadArtifactPath, err := b.getPackageVersionLeadArtifactFromMetaData(packageType, metadataClient)
		if err != nil {
			return "", err
		}
		return b.buildLeadArtifactPath(leadArtifactPath), nil
	}

	leadArtifactPath := strings.Replace(string(leadArtifact), ":", "/", 1)
	return leadArtifactPath, nil
}

func (b *basePackage) getPackageVersionLeadArtifactFromMetaData(packageType string, metadataClient metadata.Manager) (string, error) {
	body, err := metadataClient.GraphqlQuery(b.createQuery(packageType))
	if err != nil {
		return "", err
	}

	res := &model.MetadataResponse{}
	err = json.Unmarshal(body, res)
	if err != nil {
		return "", err
	}
	if len(res.Data.Versions.Edges) == 0 {
		return "", errorutils.CheckErrorf("No such package: %s/%s", b.PackageRepoName, b.PackageVersion)
	}

	// Fetch the leadFilePath based on repoName
	for _, repo := range res.Data.Versions.Edges[0].Node.Repos {
		if repo.Name == b.PackageRepoName {
			return repo.LeadFilePath, nil
		}
	}
	return "", errorutils.CheckErrorf("Can't find lead artifact of pacakge: %s/%s", b.PackageRepoName, b.PackageVersion)
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
