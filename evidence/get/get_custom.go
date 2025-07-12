package get

import (
	"fmt"
	"strings"

	"github.com/jfrog/gofrog/log"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
)

const getCustomEvidenceWithoutPredicateGraphqlQuery = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\"}} ) { totalCount edges { cursor node { path name predicateSlug createdAt createdBy subject { sha256 } signingKey { alias } } } } } }"}`
const getCustomEvidenceWithPredicateGraphqlQuery = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\"}} ) { totalCount edges { cursor node { path name predicateSlug predicate createdAt createdBy subject { sha256 } signingKey { alias } } } } } }"}`

type getEvidenceCustom struct {
	getEvidenceBase
	subjectRepoPath string
}

func NewGetEvidenceCustom(serverDetails *coreConfig.ServerDetails, subjectRepoPath, format, outputFileName string, includePredicate bool) evidence.Command {
	return &getEvidenceCustom{
		getEvidenceBase: getEvidenceBase{
			serverDetails:    serverDetails,
			format:           format,
			outputFileName:   outputFileName,
			includePredicate: includePredicate,
		},
		subjectRepoPath: subjectRepoPath,
	}
}

func (g *getEvidenceCustom) CommandName() string {
	return "get-custom-evidence"
}

func (g *getEvidenceCustom) ServerDetails() (*coreConfig.ServerDetails, error) {
	return g.serverDetails, nil
}

func (g *getEvidenceCustom) Run() error {
	onemodelClient, err := utils.CreateOnemodelServiceManager(g.serverDetails, false)
	if err != nil {
		log.Error("failed to create onemodel client", err)
		return fmt.Errorf("onemodel client init failed: %w", err)

	}

	evidences, err := g.getEvidence(onemodelClient)
	if err != nil {
		log.Error("Failed to get evidence:", err)
		return fmt.Errorf("evidence retrieval failed: %w", err)
	}

	return g.exportEvidenceToFile(evidences, g.outputFileName, g.format)
}

func (g *getEvidenceCustom) getEvidence(onemodelClient onemodel.Manager) ([]byte, error) {
	query, err := g.buildGraphqlQuery(g.subjectRepoPath)
	if err != nil {
		return nil, err
	}
	evidence, err := onemodelClient.GraphqlQuery(query)
	if err != nil {
		return nil, err
	}
	return evidence, nil
}

func (g *getEvidenceCustom) buildGraphqlQuery(subjectRepoPath string) ([]byte, error) {
	repoKey, path, err := g.getRepoKeyAndPath(subjectRepoPath)
	if err != nil {
		return nil, err
	}
	graphqlQuery := fmt.Sprintf(g.getGraphqlQuery(g.includePredicate), repoKey, path)
	log.Debug("GraphQL query: ", graphqlQuery)
	return []byte(graphqlQuery), nil
}

func (g *getEvidenceCustom) getGraphqlQuery(includePredicate bool) string {
	if includePredicate {
		return getCustomEvidenceWithPredicateGraphqlQuery
	}
	return getCustomEvidenceWithoutPredicateGraphqlQuery
}

func (g *getEvidenceCustom) getRepoKeyAndPath(subjectRepoPath string) (string, string, error) {
	idx := strings.Index(subjectRepoPath, "/")
	if idx <= 0 || idx == len(subjectRepoPath)-1 {
		return "", "", fmt.Errorf("invalid input: expected format 'repo/path', got '%s'", subjectRepoPath)
	}

	return subjectRepoPath[:idx], subjectRepoPath[idx+1:], nil
}
