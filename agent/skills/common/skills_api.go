package common

import (
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
)

func ListSkills(serverDetails *config.ServerDetails, repoKey string, limit int, sortBy string) ([]services.SkillListItem, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	var allItems []services.SkillListItem
	cursor := ""
	pageSize := 100
	for {
		items, nextCursor, err := serviceManager.ListSkills(repoKey, pageSize, cursor, sortBy)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, items...)
		if limit > 0 && len(allItems) >= limit {
			return allItems[:limit], nil
		}
		if nextCursor == "" || len(items) < pageSize {
			break
		}
		cursor = nextCursor
	}
	return allItems, nil
}

// ListVersions returns the version folders published under <repoKey>/<slug>/ using
// the generic Artifactory storage API, bypassing any Skills API filtering.
// This queries raw storage instead of the filtered Skills API endpoint.
// On 404, it disambiguates between missing repo and missing skill to provide
// users with actionable error messages.
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

	// Query raw storage layer instead of filtered Skills API
	info, err := serviceManager.FolderInfo(fmt.Sprintf("%s/%s", repoKey, slug))
	if err != nil {
		// On 404, determine whether the repo or skill is missing by checking repo existence.
		// This gives users clearer error messages for troubleshooting.
		if agentcommon.IsHTTPNotFound(err) {
			// Attempt to fetch the repo to distinguish repo-missing from skill-missing errors.
			_, repoErr := serviceManager.FolderInfo(repoKey)
			if repoErr != nil && agentcommon.IsHTTPNotFound(repoErr) {
				return nil, fmt.Errorf("repository '%s' not found: %w", repoKey, repoErr)
			}
			if repoErr != nil {
				// Non-404 errors from repo probe should be propagated (e.g., auth, network).
				return nil, fmt.Errorf("repository '%s': %w", repoKey, repoErr)
			}
			// Repo exists, so it's the skill that's missing.
			return nil, fmt.Errorf("skill '%s' not found in repository '%s': %w", slug, repoKey, err)
		}
		return nil, fmt.Errorf("list skill versions: %w", err)
	}

	versions := make([]services.SkillVersion, 0, len(info.Children))
	for _, child := range info.Children {
		if !child.Folder {
			continue
		}
		name := child.Uri
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}
		if name == "" {
			continue
		}
		versions = append(versions, services.SkillVersion{Version: name})
	}
	return versions, nil
}

func SearchSkills(serverDetails *config.ServerDetails, repoKey, query string, limit int) ([]services.SkillSearchResult, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return serviceManager.SearchSkills(repoKey, query, limit)
}

func VersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	versions, err := ListVersions(serverDetails, repoKey, slug)
	if err != nil {
		return false, err
	}
	for _, v := range versions {
		if v.Version == version {
			return true, nil
		}
	}
	return false, nil
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
