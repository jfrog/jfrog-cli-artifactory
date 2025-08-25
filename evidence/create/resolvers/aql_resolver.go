package resolvers

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type AqlResolver interface {
	Resolve(repoName, path, checksum string) ([]string, error)
}

type AqlSubjectResolver struct {
	client artifactory.ArtifactoryServicesManager
}

const aqlEmptyPathQueryTemplate = "items.find({\"repo\": \"%s\",\"sha256\": \"%s\"})"
const aqlWithPathQueryTemplate = "items.find({\"repo\": \"%s\", \"path\": {\"$match\" : \"%s*\"},\"sha256\": \"%s\"})"
const subjectRepoPath = "%s/%s/%s"

func (r *AqlSubjectResolver) Resolve(repoName, path, checksum string) ([]string, error) {
	if repoName == "" || checksum == "" {
		return nil, fmt.Errorf("repository name and checksum must be provided")
	}
	var aqlQuery string
	if path == "" {
		log.Info("Resolving subject by repository "+repoName+" and checksum", checksum)
		aqlQuery = fmt.Sprintf(aqlEmptyPathQueryTemplate, repoName, checksum)
	} else {
		log.Info("Resolving subject by repository "+repoName+", path", path, "and checksum", checksum)
		normalizedPath := strings.TrimPrefix(path, repoName+"/")
		if len(normalizedPath) < len(path) {
			pathWildcard := "*" + normalizedPath
			// repoKey could potentially be part of the path, so we add a wildcard to match any prefix
			// e.g., repoKey could be "myapp" and path could contain a folder with same name "myapp/some/path/file.txt",
			// so the full repoPath would be "myapp/myapp/some/path/file.txt", but the repoKey is hidden under the sub-domain: "myapp.docker.io/myapp/myimg:tag"
			// In this case, we want to match "*some/path/file.txt"
			log.Debug("AQL path contains repository name, adding wildcard to match any prefix:", pathWildcard)
			aqlQuery = fmt.Sprintf(aqlWithPathQueryTemplate, repoName, pathWildcard, checksum)
		} else {
			aqlQuery = fmt.Sprintf(aqlWithPathQueryTemplate, repoName, normalizedPath, checksum)
		}
	}
	log.Debug("Executing aql query", aqlQuery)
	results, err := utils.ExecuteAqlQuery(aqlQuery, &r.client)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve subject, aql error: %w", err)
	}

	var subjects []string
	for _, item := range results.Results {
		subjects = append(subjects, fmt.Sprintf(subjectRepoPath, item.Repo, item.Path, item.Name))
	}
	if len(subjects) == 0 {
		return nil, fmt.Errorf("no subject found for repository %s and checksum %s and path %s", repoName, checksum, path)
	}

	return subjects, nil
}

func NewAqlSubjectResolver(client artifactory.ArtifactoryServicesManager) *AqlSubjectResolver {
	return &AqlSubjectResolver{
		client: client,
	}
}
