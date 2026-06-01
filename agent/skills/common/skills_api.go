package common

import (
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
)

func ListSkills(serverDetails *config.ServerDetails, repoKey string, limit int, sortBy string) ([]services.SkillListItem, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	var allItems []services.SkillListItem
	cursor := ""
	pageSize := 100
	for {
		items, nextCursor, err := sm.ListSkills(repoKey, pageSize, cursor, sortBy)
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

func ListVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]services.SkillVersion, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.ListSkillVersions(repoKey, slug)
}

func SearchSkills(serverDetails *config.ServerDetails, repoKey, query string, limit int) ([]services.SkillSearchResult, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.SearchSkills(repoKey, query, limit)
}

func VersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return false, err
	}
	return sm.SkillVersionExists(repoKey, slug, version)
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
