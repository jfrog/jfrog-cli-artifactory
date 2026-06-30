package cargo

import (
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type servicesVirtualParams = services.VirtualRepositoryBaseParams

// repositoryGetter is the subset of ArtifactoryServicesManager we need (for testability).
type repositoryGetter interface {
	GetRepository(repoKey string, target interface{}) error
}

// extractRepoNameFromURL extracts the Artifactory repo key from a cargo registry index URL.
// Handles ".../api/cargo/<repo>[/index/]" and ".../artifactory/<repo>".
func extractRepoNameFromURL(indexURL string) string {
	indexURL = strings.TrimPrefix(strings.TrimSpace(indexURL), "sparse+")
	u, err := url.Parse(indexURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(segments); i++ {
		if segments[i] == "cargo" && segments[i-1] == "api" {
			return segments[i+1]
		}
	}
	// Fallback: ".../artifactory/<repo>"
	for i, s := range segments {
		if s == "artifactory" && i+1 < len(segments) {
			return segments[i+1]
		}
	}
	if len(segments) > 0 {
		return segments[len(segments)-1]
	}
	return ""
}

// resolveDeploymentRepo resolves a virtual repo to its default deployment local repo.
// Non-virtual repos pass through unchanged. Returns "" if a virtual repo has no default.
func resolveDeploymentRepo(repo string, rg repositoryGetter) string {
	if repo == "" {
		return ""
	}
	params := &servicesVirtualParams{}
	if err := rg.GetRepository(repo, params); err != nil {
		log.Debug("cargo: could not get repo details for '" + repo + "', using as-is: " + err.Error())
		return repo
	}
	if params.Rclass == services.VirtualRepositoryRepoType {
		if params.DefaultDeploymentRepo == "" {
			log.Warn("cargo: virtual repository '" + repo + "' has no default deployment repository; cannot set build properties")
			return ""
		}
		log.Info("cargo: resolved virtual repository '" + repo + "' to '" + params.DefaultDeploymentRepo + "'")
		return params.DefaultDeploymentRepo
	}
	return repo
}
