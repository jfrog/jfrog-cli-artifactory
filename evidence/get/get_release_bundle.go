package get

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const getReleaseBundleEvidenceWithoutPredicateGraphqlQuery = `{
    "query": "{
        releaseBundleVersion {
            getVersion(repositoryKey: \"%s\", name: \"%s\", version: \"%s\") {
                createdBy
                createdAt
                evidenceConnection {
                    edges {
                        cursor
                        node {
                            path
                            name
                            predicateSlug
                        }
                    }
                }
                artifactsConnection(first: %s, after: \"YXJ0aWZhY3Q6MA==\", where: {
                    hasEvidence: true
                }) {
                    totalCount
                    pageInfo {
                        hasNextPage
                        hasPreviousPage
                        startCursor
                        endCursor
                    }
                    edges {
                        cursor
                        node {
                            path
                            name
                            packageType
                            sourceRepositoryPath
                            evidenceConnection(first: 0) {
                                totalCount
                                pageInfo {
                                    hasNextPage
                                    hasPreviousPage
                                    startCursor
                                    endCursor
                                }
                                edges {
                                    cursor
                                    node {
                                        path
                                        name
                                        predicateSlug
                                    }
                                }
                            }
                        }
                    }
                }
                fromBuilds {
                    name
                    number
                    startedAt
                    evidenceConnection {
                        edges {
                            node {
                                path
                                name
                                predicateSlug
                            }
                        }
                    }
                }
            }
        }
    }"
}`

const getReleaseBundleEvidenceWithPredicateGraphqlQuery = `{
    "query": "{
        releaseBundleVersion {
            getVersion(repositoryKey: \"%s\", name: \"%s\", version: \"%s\") {
                createdBy
                createdAt
                evidenceConnection {
                    edges {
                        cursor
                        node {
                            path
                            name
                            predicateSlug
                            predicate
                        }
                    }
                }
                artifactsConnection(first: %s, after: \"YXJ0aWZhY3Q6MA==\", where: {
                    hasEvidence: true
                }) {
                    totalCount
                    pageInfo {
                        hasNextPage
                        hasPreviousPage
                        startCursor
                        endCursor
                    }
                    edges {
                        cursor
                        node {
                            path
                            name
                            packageType
                            sourceRepositoryPath
                            evidenceConnection(first: 0) {
                                totalCount
                                pageInfo {
                                    hasNextPage
                                    hasPreviousPage
                                    startCursor
                                    endCursor
                                }
                                edges {
                                    cursor
                                    node {
                                        path
                                        name
                                        predicateSlug
                                        predicate
                                    }
                                }
                            }
                        }
                    }
                }
                fromBuilds {
                    name
                    number
                    startedAt
                    evidenceConnection {
                        edges {
                            node {
                                path
                                name
                                predicateSlug
                                predicate
                            }
                        }
                    }
                }
            }
        }
    }"
}`

const defaultArtifactsLimit = "1000"

type getEvidenceReleaseBundle struct {
	getEvidenceBase
	project              string
	releaseBundle        string
	releaseBundleVersion string
	artifactsLimit       string
}

func NewGetEvidenceReleaseBundle(serverDetails *coreConfig.ServerDetails,
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
	query := g.buildGraphqlQuery(g.releaseBundle, g.releaseBundleVersion)
	evidence, err := onemodelClient.GraphqlQuery(query)
	if err != nil {
		return nil, err
	}

	if len(evidence) == 0 {
		return nil, fmt.Errorf("no evidence found for release bundle %s:%s", g.releaseBundle, g.releaseBundleVersion)
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
