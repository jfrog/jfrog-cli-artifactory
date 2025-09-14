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

// A graphql query to get evidence for a release bundle including evidence on artifacts and builds inside the release bundle.
// Artifacts in release bundle will be given from the first artifact YXJ0aWZhY3Q6MA== which is base64 of artifact:0
const getReleaseBundleEvidenceWithoutPredicateGraphqlQuery = "{\"query\":\"{ releaseBundleVersion { getVersion(repositoryKey: \\\"%s\\\", name: \\\"%s\\\", version: \\\"%s\\\") { evidenceConnection { edges { node { predicateSlug predicateType downloadPath verified signingKey { alias } createdBy createdAt subject { sha256 } } } } artifactsConnection(first: %s, after: \\\"YXJ0aWZhY3Q6MA==\\\", where: { hasEvidence: true }) { totalCount edges { node { sourceRepositoryPath packageType evidenceConnection(first: 0) { totalCount edges { node { createdBy createdAt downloadPath predicateSlug verified signingKey { alias } subject { sha256 } } } } } } } fromBuilds { name number startedAt evidenceConnection { edges { node { predicateSlug predicateType downloadPath verified signingKey { alias } createdBy createdAt subject { sha256 } } } } } } } }\"}"

const getReleaseBundleEvidenceWithPredicateGraphqlQuery = "{\"query\":\"{ releaseBundleVersion { getVersion(repositoryKey: \\\"%s\\\", name: \\\"%s\\\", version: \\\"%s\\\") { evidenceConnection { edges { node { predicateSlug predicateType downloadPath verified signingKey { alias } createdBy createdAt subject { sha256 } predicate } } } artifactsConnection(first: %s, after: \\\"YXJ0aWZhY3Q6MA==\\\", where: { hasEvidence: true }) { totalCount edges { node { sourceRepositoryPath packageType evidenceConnection(first: 0) { totalCount edges { node { createdBy createdAt downloadPath predicateSlug verified signingKey { alias } subject { sha256 } predicate } } } } } } fromBuilds { name number startedAt evidenceConnection { edges { node { predicateSlug predicateType downloadPath verified signingKey { alias } createdBy createdAt subject { sha256 } predicate } } } } } } }\"}"

const defaultArtifactsLimit = "1000" // Default limit for the number of artifacts to show in the evidence response.
type getEvidenceReleaseBundle struct {
	getEvidenceBase
	project              string
	releaseBundle        string
	releaseBundleVersion string
	artifactsLimit       string
}

type ReleaseBundleOutput struct {
	SchemaVersion string              `json:"schemaVersion"`
	Type          SubjectType         `json:"type"`
	Result        ReleaseBundleResult `json:"result"`
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

	evidenceRecords, err := g.getEvidence(onemodelClient)
	if err != nil {
		return err
	}

	err = g.exportEvidenceToFile(evidenceRecords, g.outputFileName, g.format)
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

	transformedEvidence, err := g.transformReleaseBundleGraphQLOutput(evidence)
	if err != nil {
		log.Error("Failed to transform GraphQL output:", err)
		return evidence, nil
	}

	return transformedEvidence, nil
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

func (g *getEvidenceReleaseBundle) transformReleaseBundleGraphQLOutput(rawEvidence []byte) ([]byte, error) {
	var graphqlResponse map[string]any
	if err := json.Unmarshal(rawEvidence, &graphqlResponse); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	data, ok := graphqlResponse["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing data field in GraphQL response")
	}

	releaseBundleVersion, ok := data["releaseBundleVersion"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing releaseBundleVersion field in GraphQL response")
	}

	getVersion, ok := releaseBundleVersion["getVersion"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing getVersion field in GraphQL response")
	}

	output := ReleaseBundleOutput{
		SchemaVersion: SchemaVersion,
		Type:          ReleaseBundleType,
		Result: ReleaseBundleResult{
			ReleaseBundle:        g.releaseBundle,
			ReleaseBundleVersion: g.releaseBundleVersion,
		},
	}

	releaseBundleEvidence := g.extractEvidenceFromConnection(getVersion, "evidenceConnection")
	output.Result.Evidence = releaseBundleEvidence

	artifactsEvidence := g.extractArtifactsEvidence(getVersion)
	if len(artifactsEvidence) > 0 {
		output.Result.Artifacts = artifactsEvidence
	}

	buildsEvidence := g.extractBuildsEvidence(getVersion)
	if len(buildsEvidence) > 0 {
		output.Result.Builds = buildsEvidence
	}

	transformed, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transformed output: %w", err)
	}

	return transformed, nil
}

func (g *getEvidenceReleaseBundle) extractEvidenceFromConnection(data map[string]any, connectionName string) []EvidenceEntry {
	connection, ok := data[connectionName].(map[string]any)
	if !ok {
		return []EvidenceEntry{}
	}

	edges, ok := connection["edges"].([]any)
	if !ok {
		return []EvidenceEntry{}
	}

	evidence := make([]EvidenceEntry, 0, len(edges))
	for _, edge := range edges {
		edgeMap, ok := edge.(map[string]any)
		if !ok {
			continue
		}

		node, ok := edgeMap["node"].(map[string]any)
		if !ok {
			continue
		}

		evidenceEntry := createOrderedEvidenceEntry(node, g.includePredicate)
		evidence = append(evidence, evidenceEntry)
	}

	return evidence
}

func (g *getEvidenceReleaseBundle) extractArtifactsEvidence(data map[string]any) []ArtifactEvidence {
	artifactsConnection, ok := data["artifactsConnection"].(map[string]any)
	if !ok {
		return []ArtifactEvidence{}
	}

	edges, ok := artifactsConnection["edges"].([]any)
	if !ok {
		return []ArtifactEvidence{}
	}

	artifacts := make([]ArtifactEvidence, 0, len(edges))
	for _, edge := range edges {
		edgeMap, ok := edge.(map[string]any)
		if !ok {
			continue
		}

		node, ok := edgeMap["node"].(map[string]any)
		if !ok {
			continue
		}

		repoPath, _ := node["sourceRepositoryPath"].(string)
		packageType, _ := node["packageType"].(string)
		evidence := g.extractEvidenceFromConnection(node, "evidenceConnection")

		// Create ArtifactEvidence for each evidence entry
		for _, evidenceEntry := range evidence {
			artifact := ArtifactEvidence{
				Evidence:    evidenceEntry,
				PackageType: packageType,
				RepoPath:    repoPath,
			}
			artifacts = append(artifacts, artifact)
		}
	}

	return artifacts
}

func (g *getEvidenceReleaseBundle) extractBuildsEvidence(data map[string]any) []BuildEvidence {
	fromBuilds, ok := data["fromBuilds"].([]any)
	if !ok {
		return []BuildEvidence{}
	}

	builds := make([]BuildEvidence, 0, len(fromBuilds))
	for _, build := range fromBuilds {
		buildMap, ok := build.(map[string]any)
		if !ok {
			continue
		}

		buildName, _ := buildMap["name"].(string)
		buildNumber, _ := buildMap["number"].(string)
		startedAt, _ := buildMap["startedAt"].(string)
		evidence := g.extractEvidenceFromConnection(buildMap, "evidenceConnection")

		// Create BuildEvidence for each evidence entry
		for _, evidenceEntry := range evidence {
			buildEvidence := BuildEvidence{
				Evidence:    evidenceEntry,
				BuildName:   buildName,
				BuildNumber: buildNumber,
				StartedAt:   startedAt,
			}
			builds = append(builds, buildEvidence)
		}
	}

	return builds
}
