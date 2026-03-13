package pnpm

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const cacheRepositorySuffix = "-cache"

type registryMap struct {
	defaultRepo string
	scoped      map[string]string // @scope -> repo name
}

func groupByRepo(deps []parsedDep, workingDir string) map[string][]parsedDep {
	registryRepos := getRegistryRepos(workingDir)
	groups := make(map[string][]parsedDep)
	for _, dp := range deps {
		repo := dp.parts.repo
		if repo == "" {
			log.Debug(fmt.Sprintf("No repo in resolved URL for '%s', falling back to pnpm registry config.", dp.dep.name))
			repo = resolveRepoFromRegistry(dp.dep.name, registryRepos)
		}
		groups[repo] = append(groups[repo], dp)
	}
	return groups
}

// resolvePublishRepo determines the target Artifactory repo for a published package.
// Priority: publishConfig.registry (from package.json) > pnpm config (scoped/default registry).
func resolvePublishRepo(pkgName string, publishRepos map[string]string, fallback registryMap) string {
	if repo := publishRepos[pkgName]; repo != "" {
		return repo
	}
	return resolveRepoFromRegistry(pkgName, fallback)
}

// resolveRepoFromRegistry finds the repo name for a dependency by matching its scope
// against registries from pnpm config. Falls back to the default registry.
func resolveRepoFromRegistry(depName string, registryRepos registryMap) string {
	if strings.HasPrefix(depName, "@") {
		slashIdx := strings.Index(depName, "/")
		if slashIdx == -1 {
			return registryRepos.defaultRepo
		}
		scope := depName[:slashIdx]
		if repo, ok := registryRepos.scoped[scope]; ok {
			return repo
		}
	}
	return registryRepos.defaultRepo
}

// getRegistryRepos runs `pnpm config list --json` and extracts repo names from registry URLs.
func getRegistryRepos(workingDir string) registryMap {
	result := registryMap{scoped: make(map[string]string)}

	cmd := exec.Command("pnpm", "config", "list", "--json")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		log.Debug("Could not read pnpm config, repo detection may be incomplete:", err.Error())
		return result
	}

	var configMap map[string]interface{}
	if err = json.Unmarshal(out, &configMap); err != nil {
		log.Debug("Could not parse pnpm config JSON:", err.Error())
		return result
	}

	if registry, ok := configMap["registry"].(string); ok {
		if repo := extractRepoFromRegistryURL(registry); repo != "" {
			result.defaultRepo = repo
			log.Debug("Default registry repo detected:", repo)
		}
	}

	for key, val := range configMap {
		if !strings.HasPrefix(key, "@") || !strings.HasSuffix(key, ":registry") {
			continue
		}
		scope := strings.TrimSuffix(key, ":registry")
		if registryURL, ok := val.(string); ok {
			if repo := extractRepoFromRegistryURL(registryURL); repo != "" {
				result.scoped[scope] = repo
				log.Debug(fmt.Sprintf("Scoped registry repo detected: %s -> %s", scope, repo))
			}
		}
	}

	return result
}

// extractRepoFromRegistryURL extracts the Artifactory repo name from a registry URL
// like "https://mycompany.jfrog.io/artifactory/api/npm/my-npm-repo/"
func extractRepoFromRegistryURL(registryURL string) string {
	const apiNpmPrefix = "api/npm/"
	idx := strings.Index(registryURL, apiNpmPrefix)
	if idx == -1 {
		return ""
	}
	rest := registryURL[idx+len(apiNpmPrefix):]
	rest = strings.TrimSuffix(rest, "/")
	if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
		return rest[:slashIdx]
	}
	return rest
}

// resolveAqlRepo determines the correct repo to use for AQL queries.
// Remote repos store files in a "<repo>-cache" local repo, so AQL must target that.
// Virtual and local repos are used as-is.
func resolveAqlRepo(repo string, servicesManager artifactory.ArtifactoryServicesManager) string {
	if repo == "" {
		return ""
	}
	repoDetails := &services.RepositoryDetails{}
	if err := servicesManager.GetRepository(repo, repoDetails); err != nil {
		log.Debug(fmt.Sprintf("Could not determine type for repo '%s', using as-is: %s", repo, err.Error()))
		return repo
	}
	repoType := repoDetails.GetRepoType()
	log.Debug(fmt.Sprintf("Repo '%s' type: %s", repo, repoType))
	if repoType == services.RemoteRepositoryRepoType {
		return repo + cacheRepositorySuffix
	}
	return repo
}
