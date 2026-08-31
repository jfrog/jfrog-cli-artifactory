package common

import (
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// skillVersionsPageSize is the limit we request per Skills API versions call. Kept as our own constant
// (rather than relying on services.DefaultSkillVersionsLimit) so this repo controls its own page size independently.
const skillVersionsPageSize = 200

func ListSkills(serverDetails *config.ServerDetails, repoKey string, limit int, sortBy string) ([]services.SkillListItem, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	var allItems []services.SkillListItem
	cursor := ""
	for {
		items, nextCursor, err := serviceManager.ListSkills(repoKey, skillVersionsPageSize, cursor, sortBy)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, items...)
		if limit > 0 && len(allItems) >= limit {
			return allItems[:limit], nil
		}
		if nextCursor == "" || len(items) < skillVersionsPageSize {
			break
		}
		cursor = nextCursor
	}
	return allItems, nil
}

// ListVersions returns the versions published for <repoKey>/<slug> via the Skills API
// (api/skills/{repoKey}/api/v1/skills/{slug}/versions).
//
// It requests skillVersionsPageSize versions per call and follows nextCursor for as many additional calls as needed
// (verified against live instance: nextCursor is omitted entirely when last page is served), ensuring a skill with
// more versions than one page is listed in full. On 404, it disambiguates between missing repo and missing skill.
func ListVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]services.SkillVersion, error) {
	if serverDetails == nil {
		return nil, fmt.Errorf("server details are required to list skill versions")
	}
	repoKey = strings.TrimSpace(repoKey)
	if repoKey == "" {
		return nil, fmt.Errorf("repository is required to list skill versions")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, fmt.Errorf("skill name is required to list skill versions")
	}

	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return listVersionsFromManager(serviceManager, repoKey, slug)
}

// listVersionsFromManager holds the actual pagination/error-disambiguation logic,
// taking the ArtifactoryServicesManager interface directly so it can be unit tested
// with a mock instead of a live server.
func listVersionsFromManager(serviceManager artifactory.ArtifactoryServicesManager, repoKey, slug string) ([]services.SkillVersion, error) {
	var allVersions []services.SkillVersion
	cursor := ""
	for {
		log.Debug(fmt.Sprintf("list skill versions: calling ListSkillVersions for skill '%s' in repo '%s' with cursor '%s'", slug, repoKey, cursor))
		versions, nextCursor, err := serviceManager.ListSkillVersions(repoKey, slug, skillVersionsPageSize, cursor)
		if err != nil {
			// Only disambiguate on the first page: a 404 mid-pagination means the skill
			// was deleted concurrently, not that the repo/skill never existed.
			if cursor == "" && agentcommon.IsHTTPNotFound(err) {
				return nil, agentcommon.DisambiguateFolderError(serviceManager, repoKey, err, fmt.Errorf("skill '%s' not found in repository '%s': %w", slug, repoKey, err))
			}
			return nil, fmt.Errorf("list skill versions: %w", err)
		}
		allVersions = append(allVersions, versions...)
		log.Debug(fmt.Sprintf("list skill versions: received %d versions, next cursor: '%s'", len(versions), nextCursor))
		if nextCursor == "" {
			log.Debug(fmt.Sprintf("list skill versions: no more pages for skill '%s', %d version(s) total", slug, len(allVersions)))
			break
		}
		if nextCursor == cursor {
			// Guard against a server bug returning a non-advancing cursor: without this,
			// a stuck cursor spins forever, growing allVersions unbounded.
			return nil, fmt.Errorf("list skill versions: server returned a non-advancing cursor for skill '%s'", slug)
		}
		cursor = nextCursor
	}
	return allVersions, nil
}

func SearchSkills(serverDetails *config.ServerDetails, repoKey, query string, limit int) ([]services.SkillSearchResult, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return serviceManager.SearchSkills(repoKey, query, limit)
}

// VersionExists reports whether version is published for repoKey/slug, via a single
// version-detail request instead of paginating ListVersions. A "false" result doesn't
// say why (version missing vs skill/repo missing) - callers that need that, like
// `skills delete --dry-run`, should follow up with ListVersions instead.
func VersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	if serverDetails == nil {
		return false, fmt.Errorf("server details are required to check skill version existence")
	}
	repoKey = strings.TrimSpace(repoKey)
	if repoKey == "" {
		return false, fmt.Errorf("repository is required to check skill version existence")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return false, fmt.Errorf("skill name is required to check skill version existence")
	}

	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return false, err
	}
	return versionExistsFromManager(serviceManager, repoKey, slug, version)
}

// versionExistsFromManager takes the ArtifactoryServicesManager interface directly so it can be unit tested with a mock.
func versionExistsFromManager(serviceManager artifactory.ArtifactoryServicesManager, repoKey, slug, version string) (bool, error) {
	exists, err := serviceManager.SkillVersionExists(repoKey, slug, version)
	if err != nil {
		return false, fmt.Errorf("check skill version existence: %w", err)
	}
	return exists, nil
}

func SearchSkillsByProperty(serverDetails *config.ServerDetails, query, repoKey string) ([]services.SkillPropertySearchResult, error) {
	results, err := agentcommon.SearchByProperty(serverDetails, agentcommon.PropertySearchOptions{
		NamePropertyKey: SearchNamePropertyKey,
		Query:           query,
		RepoKey:         repoKey,
	})
	if err != nil {
		return nil, err
	}
	out := make([]services.SkillPropertySearchResult, len(results))
	for i, r := range results {
		out[i] = services.SkillPropertySearchResult{
			Repo:    r.Repo,
			Name:    r.Name,
			Version: r.Version,
			URI:     r.URI,
		}
	}
	return out, nil
}

// GetSkillDescription fetches the skill.description property for a given artifact path.
func GetSkillDescription(serverDetails *config.ServerDetails, repoPath string) (string, error) {
	return agentcommon.GetItemPropertyDescription(serverDetails, repoPath, SearchDescriptionPropertyKeys)
}
