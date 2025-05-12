package evidence

import (
	"fmt"
	"os"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const getEvidenceWithoutPredicateGraphqlQuery = `{ "query": "{\n  releaseBundleVersion {\n    getVersion(repositoryKey: \"%s\", name: \"%s\", version: \"%s\") {\n      createdBy\n      createdAt\n      evidenceConnection {\n        edges {\n          cursor\n          node {\n            path\n            name\n            predicateSlug\n          }\n        }\n      }\n      artifactsConnection(first: 13, after:\"YXJ0aWZhY3Q6Mw==\") {\n        totalCount\n        pageInfo {\n          hasNextPage\n          hasPreviousPage\n          startCursor\n          endCursor\n        }\n        edges {\n          cursor\n          node {\n            path\n            name\n            packageType\n            sourceRepositoryPath\n            evidenceConnection(first: 0) {\n              totalCount\n              pageInfo {\n                hasNextPage\n                hasPreviousPage\n                startCursor\n                endCursor\n              }\n              edges {\n                cursor\n                node {\n                  path\n                  name\n                  predicateSlug\n                }\n              }\n            }\n          }\n        }\n      }\n      fromBuilds {\n        name\n        number\n        startedAt\n        evidenceConnection {\n          edges {\n            node {\n              path\n              name\n              predicateSlug\n            }\n          }\n        }\n      }\n    }\n  }\n}" }`
const getEvidenceWithPredicateGraphqlQuery = `{ "query": "{\n  releaseBundleVersion {\n    getVersion(repositoryKey: \"%s\", name: \"%s\", version: \"%s\") {\n      createdBy\n      createdAt\n      evidenceConnection {\n        edges {\n          cursor\n          node {\n            path\n            name\n            predicateSlug\n          predicate\n		}\n        }\n      }\n      artifactsConnection(first: 13, after:\"YXJ0aWZhY3Q6Mw==\") {\n        totalCount\n        pageInfo {\n          hasNextPage\n          hasPreviousPage\n          startCursor\n          endCursor\n        }\n        edges {\n          cursor\n          node {\n            path\n            name\n            packageType\n            sourceRepositoryPath\n            evidenceConnection(first: 0) {\n              totalCount\n              pageInfo {\n                hasNextPage\n                hasPreviousPage\n                startCursor\n                endCursor\n              }\n              edges {\n                cursor\n                node {\n                  path\n                  name\n                  predicateSlug\n                predicate\n		}\n              }\n            }\n          }\n        }\n      }\n      fromBuilds {\n        name\n        number\n        startedAt\n        evidenceConnection {\n          edges {\n            node {\n              path\n              name\n              predicateSlug\n            predicate\n		}\n          }\n        }\n      }\n    }\n  }\n}" }`

type getEvidenceReleaseBundle struct {
	serverDetails        *coreConfig.ServerDetails
	project              string
	releaseBundle        string
	releaseBundleVersion string
	outputFileName       string
	format               string
	includePredicate     bool
}

func NewGetEvidenceReleaseBundle(serverDetails *coreConfig.ServerDetails, project, releaseBundle,
	releaseBundleVersion string) Command {
	return &getEvidenceReleaseBundle{
		serverDetails:        serverDetails,
		project:              project,
		releaseBundle:        releaseBundle,
		releaseBundleVersion: releaseBundleVersion,
	}
}

func (g *getEvidenceReleaseBundle) CommandName() string {
	return "get-release-bundle-evidence"
}

func (g *getEvidenceReleaseBundle) ServerDetails() (*coreConfig.ServerDetails, error) {
	return g.serverDetails, nil
}

func (g *getEvidenceReleaseBundle) Run() error {
	onemodelClient, err := utils.CreateOnemodelServiceManager(g.serverDetails, false)
	if err != nil {
		log.Error("failed to create onemodel client", err)
		return err
	}

	evidences, err := g.getEvidence(onemodelClient)
	if err != nil {
		return err
	}

	err = exportEvidenceToFile(evidences, g.outputFileName, g.format)
	if err != nil {
		return err
	}

	return nil
}

func (g *getEvidenceReleaseBundle) getEvidence(onemodelClient onemodel.Manager) ([]byte, error) {
	repoKey := getRepoKey(g.project)
	graphqlQuery := getGraphqlQuery(g.includePredicate)
	query := createGetEvidenceQuery(repoKey, g.releaseBundle, g.releaseBundleVersion, graphqlQuery)
	evidence, err := onemodelClient.GraphqlQuery(query)
	if err != nil {
		return nil, err
	}

	if len(evidence) == 0 {
		return nil, fmt.Errorf("no evidence found for release bundle %s", g.releaseBundle)
	}

	return evidence, nil
}

func getRepoKey(project string) string {
	defaultReleaseBundleRepoKey := "release-bundles-v2"
	if project == "" || project == "default" {
		return defaultReleaseBundleRepoKey
	}
	return fmt.Sprintf("%s-%s", project, defaultReleaseBundleRepoKey)
}

func getGraphqlQuery(includePredicate bool) string {
	if includePredicate {
		return getEvidenceWithPredicateGraphqlQuery
	}
	return getEvidenceWithoutPredicateGraphqlQuery
}

func createGetEvidenceQuery(repoKey, releaseBundle, releaseBundleVersion, query string) []byte {
	graphqlQuery := fmt.Sprintf(getEvidenceWithoutPredicateGraphqlQuery, repoKey, releaseBundle, releaseBundleVersion)
	log.Debug("GraphQL query: ", graphqlQuery)
	return []byte(graphqlQuery)
}

func exportEvidenceToFile(evidence []byte, outputFileName, format string) error {
	if outputFileName == "" {
		outputFileName = "evidences"
	}

	if format == "" {
		format = "json"
	}

	switch format {
	case "json":
		return exportEvidenceToJsonFile(evidence, outputFileName)
	default:
		log.Error("Unsupported format. Supported formats are: json")
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func exportEvidenceToJsonFile(evidence []byte, outputFileName string) error {
	file, err := os.Create(outputFileName)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = file.Write(evidence)
	if err != nil {
		return err
	}

	log.Info("Evidence successfully exported to", outputFileName)
	return nil
}
