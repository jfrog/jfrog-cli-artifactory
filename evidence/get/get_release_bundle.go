package get

import (
	"encoding/json"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const getReleaseBundleEvidenceWithoutPredicateGraphqlQuery = "{\"query\":\"{ releaseBundleVersion { getVersion(repositoryKey: \\\"%s\\\", name: \\\"%s\\\", version: \\\"%s\\\") { createdBy createdAt evidenceConnection { edges { cursor node { createdBy createdAt path name predicateSlug } } } artifactsConnection(first: %s, after: \\\"YXJ0aWZhY3Q6MA==\\\", where: { hasEvidence: true }) { totalCount pageInfo { hasNextPage hasPreviousPage startCursor endCursor } edges { cursor node { path name packageType sourceRepositoryPath evidenceConnection(first: 0) { totalCount edges { cursor node { createdBy createdAt path name predicateSlug } } } } } } fromBuilds { name number startedAt evidenceConnection { edges { node { createdBy createdAt path name predicateSlug } } } } } } }\"}"

const getReleaseBundleEvidenceWithPredicateGraphqlQuery = "{\"query\":\"{ releaseBundleVersion { getVersion(repositoryKey: \\\"%s\\\", name: \\\"%s\\\", version: \\\"%s\\\") { createdBy createdAt evidenceConnection { edges { cursor node { createdBy createdAt path name predicateSlug predicate } } } artifactsConnection(first: %s, after: \\\"YXJ0aWZhY3Q6MA==\\\", where: { hasEvidence: true }) { totalCount pageInfo { hasNextPage hasPreviousPage startCursor endCursor } edges { cursor node { path name packageType sourceRepositoryPath evidenceConnection(first: 0) { totalCount pageInfo { hasNextPage hasPreviousPage startCursor endCursor } edges { cursor node { createdBy createdAt path name predicateSlug predicate } } } } } } fromBuilds { name number startedAt evidenceConnection { edges { node { createdBy createdAt path name predicateSlug predicate } } } } } } }\"}"

const defaultArtifactsLimit = "1000" // Default limit for the number of artifacts to show in the evidence response.
type getEvidenceReleaseBundle struct {
	getEvidenceBase
	project              string
	releaseBundle        string
	releaseBundleVersion string
	artifactsLimit       string
}

func NewGetEvidenceReleaseBundle(serverDetails *config.ServerDetails,
	releaseBundle, releaseBundleVersion, project, format, outputFileName, artifactsLimit string, includePredicate bool) evidence.Command {
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
		artifactsLimit:       artifactsLimit,
	}
}

func (g *getEvidenceReleaseBundle) CommandName() string {
	return "get-release-bundle-evidence"
}

func (g *getEvidenceReleaseBundle) ServerDetails() (*config.ServerDetails, error) {
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
	query := g.buildGraphqlQuery(g.releaseBundle, g.releaseBundleVersion)
	evidence, err := onemodelClient.GraphqlQuery(query)
	if err != nil {
		return nil, err
	}

	if len(evidence) == 0 {
		return nil, fmt.Errorf("no evidence found for release bundle %s:%s", g.releaseBundle, g.releaseBundleVersion)
	}

	// Prettify the output by removing cursors, errors, and null fields
	prettifiedEvidence, err := g.prettifyGraphQLOutput(evidence)
	if err != nil {
		log.Error("Failed to prettify GraphQL output:", err)
		// Return original evidence if prettification fails
		return evidence, nil
	}

	return prettifiedEvidence, nil
}

// prettifyGraphQLOutput removes cursor fields, errors, and null fields from the GraphQL response
func (g *getEvidenceReleaseBundle) prettifyGraphQLOutput(rawEvidence []byte) ([]byte, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(rawEvidence, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GraphQL response: %w", err)
	}

	// Remove errors field if present
	delete(data, "errors")

	// Process the data recursively to remove cursors and null fields
	if dataObj, ok := data["data"].(map[string]interface{}); ok {
		g.removeCursorsAndNulls(dataObj)
	}

	// Marshal back to JSON with proper indentation
	prettified, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal prettified response: %w", err)
	}

	return prettified, nil
}

// removeCursorsAndNulls recursively removes cursor fields and null fields from the data structure
func (g *getEvidenceReleaseBundle) removeCursorsAndNulls(data interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Remove cursor fields
		delete(v, "cursor")
		delete(v, "startCursor")
		delete(v, "endCursor")

		// Remove null fields
		for key, value := range v {
			if value == nil {
				delete(v, key)
			} else {
				// Recursively process nested structures
				g.removeCursorsAndNulls(value)
			}
		}

	case []interface{}:
		// Process each element in the slice
		for _, item := range v {
			g.removeCursorsAndNulls(item)
		}
	}
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

func (g *getEvidenceReleaseBundle) buildGraphqlQuery(releaseBundle, releaseBundleVersion string) []byte {
	numberOfArtifactsToShow := g.getArtifactLimit(g.artifactsLimit)
	graphqlQuery := fmt.Sprintf(g.getGraphqlQuery(g.includePredicate), g.getRepoKey(g.project), releaseBundle, releaseBundleVersion, numberOfArtifactsToShow)
	log.Debug("GraphQL query: ", graphqlQuery)
	return []byte(graphqlQuery)
}

func (g *getEvidenceReleaseBundle) getArtifactLimit(artifactsLimit string) string {
	if artifactsLimit == "" {
		return defaultArtifactsLimit
	}
	return artifactsLimit
}
