package evidence

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const getReleaseBundleEvidenceWithoutPredicateGraphqlQuery = `{ "query": "{\n  releaseBundleVersion {\n    getVersion(repositoryKey: \"%s\", name: \"%s\", version: \"%s\") {\n      createdBy\n      createdAt\n      evidenceConnection {\n        edges {\n          cursor\n          node {\n            path\n            name\n            predicateSlug\n          }\n        }\n      }\n      artifactsConnection(first: 1000, after:\"YXJ0aWZhY3Q6MA==\", where:{\n  hasEvidence: true\n}) {\n        totalCount\n        pageInfo {\n          hasNextPage\n          hasPreviousPage\n          startCursor\n          endCursor\n        }\n        edges {\n          cursor\n          node {\n            path\n            name\n            packageType\n            sourceRepositoryPath\n            evidenceConnection(first: 0) {\n              totalCount\n              pageInfo {\n                hasNextPage\n                hasPreviousPage\n                startCursor\n                endCursor\n              }\n              edges {\n                cursor\n                node {\n                  path\n                  name\n                  predicateSlug\n                }\n              }\n            }\n          }\n        }\n      }\n      fromBuilds {\n        name\n        number\n        startedAt\n        evidenceConnection {\n          edges {\n            node {\n              path\n              name\n              predicateSlug\n            }\n          }\n        }\n      }\n    }\n  }\n}" }`
const getReleaseBundleEvidenceWithPredicateGraphqlQuery = `{ "query": "{\n  releaseBundleVersion {\n    getVersion(repositoryKey: \"%s\", name: \"%s\", version: \"%s\") {\n      createdBy\n      createdAt\n      evidenceConnection {\n        edges {\n          cursor\n          node {\n            path\n            name\n            predicateSlug\n            predicate\n          }\n        }\n      }\n      artifactsConnection(first: 1000, after:\"YXJ0aWZhY3Q6MA==\", where:{\n  hasEvidence: true\n}) {\n        totalCount\n        pageInfo {\n          hasNextPage\n          hasPreviousPage\n          startCursor\n          endCursor\n        }\n        edges {\n          cursor\n          node {\n            path\n            name\n            packageType\n            sourceRepositoryPath\n            evidenceConnection(first: 0) {\n              totalCount\n              pageInfo {\n                hasNextPage\n                hasPreviousPage\n                startCursor\n                endCursor\n              }\n              edges {\n                cursor\n                node {\n                  path\n                  name\n                  predicateSlug\n                predicate\n		}\n              }\n            }\n          }\n        }\n      }\n      fromBuilds {\n        name\n        number\n        startedAt\n        evidenceConnection {\n          edges {\n            node {\n              path\n              name\n              predicateSlug\n            predicate\n		}\n          }\n        }\n      }\n    }\n  }\n}" }`

type getEvidenceReleaseBundle struct {
	getEvidenceBase
	project              string
	releaseBundle        string
	releaseBundleVersion string
}

func NewGetEvidenceReleaseBundle(serverDetails *coreConfig.ServerDetails,
	releaseBundle, releaseBundleVersion, project, format, outputFileName string, includePredicate bool) Command {
	return &getEvidenceReleaseBundle{
		getEvidenceBase: getEvidenceBase{
			serverDetails:    serverDetails,
			outputFileName:   outputFileName,
			format:           format,
			includePredicate: includePredicate,
		},
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

	err = g.exportEvidenceToFile(evidences, g.outputFileName, g.format)
	if err != nil {
		return err
	}

	return nil
}

func (g *getEvidenceReleaseBundle) getEvidence(onemodelClient onemodel.Manager) ([]byte, error) {
	query := g.createGetEvidenceQuery(g.releaseBundle, g.releaseBundleVersion)
	evidence, err := onemodelClient.GraphqlQuery(query)
	if err != nil {
		return nil, err
	}

	if len(evidence) == 0 {
		return nil, fmt.Errorf("no evidence found for release bundle %s", g.releaseBundle)
	}

	return evidence, nil
}

func (g *getEvidenceReleaseBundle) getRepoKey(project string) string {
	defaultReleaseBundleRepoKey := "release-bundles-v2"
	if project == "" || project == "default" {
		return defaultReleaseBundleRepoKey
	}
	return fmt.Sprintf("%s-%s", project, defaultReleaseBundleRepoKey)
}

func (g *getEvidenceReleaseBundle) getGraphqlQuery(includePredicate bool) string {
	if includePredicate {
		return getReleaseBundleEvidenceWithPredicateGraphqlQuery
	}
	return getReleaseBundleEvidenceWithoutPredicateGraphqlQuery
}

func (g *getEvidenceReleaseBundle) createGetEvidenceQuery(releaseBundle, releaseBundleVersion string) []byte {
	graphqlQuery := fmt.Sprintf(g.getGraphqlQuery(g.includePredicate), g.getRepoKey(g.project), releaseBundle, releaseBundleVersion)
	log.Debug("GraphQL query: ", graphqlQuery)
	return []byte(graphqlQuery)
}
