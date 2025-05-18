package evidence

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jfrog/gofrog/log"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
)

const getCustomEvidenceWithoutPredicateGraphqlQuery = `query SearchEvidence {\n evidence {\n searchEvidence( where: {hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\" }} ) {\n totalCount\n pageInfo {\n hasNextPage\n hasPreviousPage\n startCursor\n endCursor\n }\n edges {\n cursor\n node {\n path\n name\n predicateSlug\n}\n }\n }\n }\n }`
const getCustomEvidenceWithPredicateGraphqlQuery = `query SearchEvidence {\n evidence {\n searchEvidence( where: {hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\" }} ) {\n totalCount\n pageInfo {\n hasNextPage\n hasPreviousPage\n startCursor\n endCursor\n }\n edges {\n cursor\n node {\n path\n name\n predicateSlug\n predicate\n }\n }\n }\n }\n }`

type getEvidenceCustom struct {
	getEvidenceBase
	subjectRepoPath string
}

func NewGetEvidenceCustom(serverDetails *coreConfig.ServerDetails, subjectRepoPath, format, outputFileName string, includePredicate bool) Command {
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

func (g *getEvidenceCustom) getEvidence(onemodelClient onemodel.Manager) ([]byte, error) {
	query, err := g.createGetEvidenceQuery(g.subjectRepoPath)
	evidence, err := onemodelClient.GraphqlQuery(query)
	if err != nil {
		return nil, err
	}
	return evidence, nil
}

func (g *getEvidenceCustom) createGetEvidenceQuery(subjectRepoPath string) ([]byte, error) {
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
	// Split the input to 2 parts using the "/" delimiter
	parts := strings.SplitN(subjectRepoPath, "/", 2)

	// Check if we have at least 2 parts
	if len(parts) < 2 {
		return "", "", errors.New("invalid input format: must be in 'repo/path' format")
	}

	repo := parts[0]
	path := parts[1]

	return repo, path, nil
}
