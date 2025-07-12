package get

import (
	"fmt"
	"path"
	"strings"

	"github.com/jfrog/gofrog/log"
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
)

const getCustomEvidenceWithoutPredicateGraphqlQuery = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\"}} ) { totalCount edges { node { createdAt createdBy path name predicateSlug subject { sha256 } signingKey { alias } } } } } }"}`
const getCustomEvidenceWithPredicateGraphqlQuery = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\"}} ) { totalCount edges { node {createdAt createdBy path name predicateSlug predicate subject { sha256 } signingKey { alias } } } } } }"}`

type getEvidenceCustom struct {
	getEvidenceBase
	subjectRepoPath string
}

func NewGetEvidenceCustom(serverDetails *config.ServerDetails, subjectRepoPath, format, outputFileName string, includePredicate bool) evidence.Command {
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

func (g *getEvidenceCustom) ServerDetails() (*config.ServerDetails, error) {
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
	repoKey, path, name, err := g.getRepoKeyAndPath(subjectRepoPath)
	if err != nil {
		return nil, err
	}
	graphqlQuery := fmt.Sprintf(g.getGraphqlQuery(g.includePredicate), repoKey, path, name)
	log.Debug("GraphQL query: ", graphqlQuery)
	return []byte(graphqlQuery), nil
}

func (g *getEvidenceCustom) getGraphqlQuery(includePredicate bool) string {
	if includePredicate {
		return getCustomEvidenceWithPredicateGraphqlQuery
	}
	return getCustomEvidenceWithoutPredicateGraphqlQuery
}

func (g *getEvidenceCustom) getRepoKeyAndPath(subjectRepoPath string) (string, string, string, error) {
	firstSlashIndex := strings.Index(subjectRepoPath, "/")
	if firstSlashIndex <= 0 || firstSlashIndex == len(subjectRepoPath)-1 {
		return "", "", "", fmt.Errorf("invalid input: expected format 'repo/path', got '%s'", subjectRepoPath)
	}
	repo := subjectRepoPath[:firstSlashIndex]
	pathAndName := subjectRepoPath[firstSlashIndex+1:]

	pathVal := path.Dir(pathAndName)
	name := path.Base(pathAndName)
	if pathVal == "." {
		pathVal = ""
	}

	return repo, pathVal, name, nil
}
